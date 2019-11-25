package jobmon

import (
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	v12 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"net/http"
	k "sisyphus/kubernetes"
	"sisyphus/protocol"
	"strings"
	"time"
)

// Create job from descriptor and monitor loop
func RunJob(spec *protocol.JobSpec,
	k8sSession *k.Session,
	k8sJobParams *k.K8SJobParameters,
	httpSession *protocol.RunnerHttpSession,
	cacheBucket string,
	stopChan <-chan bool,
	tickGitLabLog *time.Ticker) {

	jobPrefix := fmt.Sprintf("sphs-%v-%v-", spec.JobInfo.ProjectId, spec.Id)

	rrq, err := protocol.ToFlatJson(k8sJobParams)
	if err != nil {
		logrus.Error(err)
		return
	}

	logrus.WithFields(map[string]interface{}{
		"project": spec.JobInfo.ProjectName,
		"job":     spec.JobInfo.Name,
		"jobId":   spec.Id,
	}).Infof("Starting new job with parameters %s", rrq)

	job, err := k8sSession.CreateGitLabJob(jobPrefix, spec, k8sJobParams, cacheBucket)
	if err != nil {
		msg := fmt.Sprintf("Failed to create K8S job for project=%v, job=%v, job_id=%v",
			spec.JobInfo.ProjectName,
			spec.JobInfo.Name,
			spec.Id)

		logrus.Error(msg)
		logrus.Error(err)
		return
	} else {
		monitorJob(job, httpSession, spec.Id, spec.Token, stopChan, tickGitLabLog)
	}
}

// Monitor job loop
func monitorJob(job *k.Job, httpSession *protocol.RunnerHttpSession, jobId int, gitlabJobToken string, stopChan <-chan bool, tickGitLabLog *time.Ticker) {
	ctxLogger := logrus.WithFields(
		logrus.Fields{
			"k8sjob":    job.Name,
			"gitlabjob": jobId,
		})

	loggingState := newLogState(ctxLogger)

	// Logger for gitlab trace
	// Writes log messages directly to gitlab console
	labLog := logrus.New()
	labLog.SetLevel(logrus.DebugLevel)
	labLog.SetFormatter(&logrus.TextFormatter{
		ForceColors:            true,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})
	labLog.SetOutput(loggingState.logBuffer)

	backChannel := gitLabBackChannel{
		httpSession:    httpSession,
		jobId:          jobId,
		gitlabJobToken: gitlabJobToken,
		localLogger:    ctxLogger,
	}

	defer func() {
		ctxLogger.Infof("Deleting job %v", job.Name)
		err := job.Delete()
		if err != nil {
			ctxLogger.Error(err)
		}
	}()

	logPush := func() {
		<-tickGitLabLog.C
		err := pushLogsToGitlab(loggingState, &backChannel)
		if err != nil {
			ctxLogger.Warn(err)
		}
	}

	// The error can be ignored for pending status,
	_, _ = backChannel.syncJobStatus(protocol.Pending)

	// Rate limiter for this routine
	tickJobState := time.NewTicker(1 * time.Second)
	defer tickJobState.Stop()

	logPushTimer := time.NewTicker(1 * time.Second)
	defer logPushTimer.Stop()

	for {
		select {

		case <-tickJobState.C:
			status, err := job.GetK8SJobStatus()
			if err != nil {
				ctxLogger.Warn(err)
				labLog.Warnf("%s %s", err, podsInfoMessage(status.Pods))
				continue
			}

			// Handle jobs canceled by gitlab
			gitlabStatus, err := backChannel.syncJobStatus(protocol.Running)
			switch {
			case gitlabStatus == nil:
				ctxLogger.Warn("gitlab job status is nil")
				continue
			case gitlabStatus.StatusCode == http.StatusForbidden:
				ctxLogger.Info("job canceled")
				return
			case gitlabStatus.StatusCode != http.StatusOK:
				ctxLogger.Warnf("unknown gitlab status response code '%d', msg '%s'", gitlabStatus.StatusCode, gitlabStatus.RemoteState)
				continue
			}

			js := status.Job.Status

			// The pod must be not in pending or unknown state to have logs
			builderPhase := status.PodPhases[k.ContainerNameBuilder]
			if builderPhase == v1.PodRunning || builderPhase == v1.PodSucceeded || builderPhase == v1.PodFailed {
				// find pod for builder
				//noinspection GoShadowedVar
				podName, err := findPodOfContainer(status.Pods, k.ContainerNameBuilder)
				if err != nil {
					ctxLogger.Warn(err)
					labLog.Warnf("%s %s", err, podsInfoMessage(status.Pods))
					continue
				}

				// Fetch logs from K8S
				err = loggingState.bufferLogs(job, podName)
				if err != nil {
					ctxLogger.Warn(err)
					labLog.Warnf("%s %s", err, podsInfoMessage(status.Pods))
					continue
				}
			} else if builderPhase == v1.PodPending {
				podInfo := podsInfoMessage(status.Pods)
				labLog.Infof("PENDING %s", podInfo)
			}

			switch {
			case js.Failed > 0:
				duration := renderJobDuration(&js)
				msg := fmt.Sprintf("Job Failed %s. %s", duration, podsInfoMessage(status.Pods))
				ctxLogger.Warn(msg)
				labLog.Error(msg)

				logPush()
				syncJobStateLoop(&backChannel, protocol.Failed, ctxLogger)
				return

			case js.Succeeded > 0 && js.Active == 0:

				duration := renderJobDuration(&js)
				msg := fmt.Sprintf("OK: duration %s. %s", duration, podsInfoMessage(status.Pods))

				ctxLogger.Info(msg)
				labLog.Info(msg)

				logPush()
				syncJobStateLoop(&backChannel, protocol.Success, ctxLogger)
				return
			}

			// push logs to gitlab
		case <-logPushTimer.C:
			logPush()

		case <-stopChan:
			// the runner is killed
			labLog.Error("The runner was killed")
			logPush()
			syncJobStateLoop(&backChannel, protocol.Failed, ctxLogger)
			return
		}
	}

}

func renderJobDuration(jobStatus *v12.JobStatus) string {
	strDuration := "unknown"

	if jobStatus.StartTime != nil && jobStatus.CompletionTime != nil {
		start := jobStatus.StartTime.Time
		end := jobStatus.CompletionTime.Time

		dur := end.Sub(start)
		strDuration = dur.String()
	}

	return strDuration
}

func syncJobStateLoop(backChannel *gitLabBackChannel, state protocol.JobState, ctxLogger *logrus.Entry) {
	loopTicker := time.NewTicker(time.Second)
	defer loopTicker.Stop()
	var retries = 5

	for retries > 0 {
		retries = retries - 1

		select {
		case <-loopTicker.C:
			_, err := backChannel.syncJobStatus(state)
			if err != nil {
				ctxLogger.Warn(err)
			} else {
				return
			}
		}
	}

}

func pushLogsToGitlab(logState *logState, backChannel *gitLabBackChannel) error {
	logState.logBufferMux.Lock()
	defer logState.logBufferMux.Unlock()

	if logState.logBuffer.Len() > 0 {
		contentRange, err := backChannel.writeLogLines(logState.logBuffer.Bytes(), logState.gitlabStartOffset)

		if err != nil {
			if contentRange != nil {
				logState.gitlabStartOffset = contentRange.End
			}

			return err
		} else {
			// update next offset
			if contentRange == nil {
				logState.gitlabStartOffset = logState.gitlabStartOffset + logState.logBuffer.Len()
			} else {
				logState.gitlabStartOffset = contentRange.End
			}

			// reset buffer
			logState.logBuffer.Reset()
		}
	}

	return nil
}

func podsInfoMessage(pods []v1.Pod) string {
	perpod := make([]string, 0, len(pods))

	for _, pod := range pods {
		//if pod.Status.Phase != nil {
		msg := podStatusMessage(pod)
		perpod = append(perpod, msg)
		//}
	}

	return strings.Join(perpod, ", ")
}

func podStatusMessage(pod v1.Pod) string {
	const check = "\u2714"
	const cross = "\u2718"
	const qMark = "\uFE56"

	status := pod.Status
	activeConditions := make([]string, 0, len(status.Conditions))

	for _, cond := range status.Conditions {
		var mark string

		switch cond.Status {
		case v1.ConditionTrue:
			mark = check
		case v1.ConditionFalse:
			mark = cross
		default:
			mark = qMark
		}
		desc := fmt.Sprintf("%s %s", cond.Type, mark)
		activeConditions = append(activeConditions, desc)
	}

	return fmt.Sprintf("[pod='%s' phase='%s' conditions='%s' reason='%s' msg='%s']",
		pod.Name, status.Phase,
		strings.Join(activeConditions, ", "),
		status.Reason, status.Message)
}

func findPodOfContainer(pods []v1.Pod, containerName string) (string, error) {
	for _, pod := range pods {
		for _, ctr := range pod.Spec.Containers {
			if ctr.Name == containerName {
				return pod.Name, nil
			}
		}
	}

	return "", errors.New(fmt.Sprintf("can not find pod for container '%s'", containerName))
}
