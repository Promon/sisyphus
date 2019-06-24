package jobmon

import (
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
func RunJob(spec *protocol.JobSpec, k8sSession *k.Session, httpSession *protocol.RunnerHttpSession, cacheBucket string, workOk <-chan bool) {
	jobPrefix := fmt.Sprintf("sphs-%v-%v-", spec.JobInfo.ProjectId, spec.Id)

	job, err := k8sSession.CreateGitLabJob(jobPrefix, spec, cacheBucket)
	if err != nil {
		msg := fmt.Sprintf("Failed to create K8S job for project=%v, job=%v, job_id=%v",
			spec.JobInfo.ProjectName,
			spec.JobInfo.Name,
			spec.Id)

		logrus.Error(msg)
		logrus.Error(err)
		return
	} else {
		monitorJob(job, httpSession, spec.Id, spec.Token, workOk)
	}
}

// Monitor job loop
func monitorJob(job *k.Job, httpSession *protocol.RunnerHttpSession, jobId int, gitlabJobToken string, workOk <-chan bool) {
	ctxLogger := logrus.WithFields(
		logrus.Fields{
			"k8sjob":    job.Name,
			"gitlabjob": jobId,
		})

	logState := newLogState(ctxLogger)

	// Logger for gitlab trace
	// Writes log messages directly to gitlab console
	labLog := logrus.New()
	labLog.SetLevel(logrus.DebugLevel)
	labLog.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	labLog.SetOutput(logState.logBuffer)

	backChannel := GitlabBackChannel{
		httpSession:    httpSession,
		jobId:          jobId,
		gitlabJobToken: gitlabJobToken,
		localLogger:    ctxLogger,
	}

	defer func() {
		ctxLogger.Debugf("Deleting Job %v", job.Name)
		err := job.Delete()
		if err != nil {
			ctxLogger.Error(err)
		}
	}()

	logPush := func() {
		err := pushLogsToGitlab(logState, &backChannel)
		if err != nil {
			ctxLogger.Warn("Failed to push logs to gitlab")
		}
	}

	backChannel.syncJobStatus(protocol.Pending)

	// Rate limiter for this routine
	tickLimiter := time.NewTicker(1 * time.Second)
	defer tickLimiter.Stop()

	for range workOk {
		<-tickLimiter.C

		status, err := job.GetK8SJobStatus()
		if err != nil {
			ctxLogger.Warn(err)
			labLog.Warn(err)
		}

		js := status.JobStatus
		ctxLogger.Debugf("Status Active %v, failed %v, succeeded %v", js.Active, js.Failed, js.Succeeded)

		// The pod must be not in pending or unknown state to have logs
		builderPhase := status.PodPhases[k.ContainerNameBuilder]
		if builderPhase == v1.PodRunning || builderPhase == v1.PodSucceeded || builderPhase == v1.PodFailed {
			// Job canceled remotely
			status := backChannel.syncJobStatus(protocol.Running)
			if cancelRequested(status) {
				ctxLogger.Info("Job canceled")
				return
			}

			// Fetch logs from K8S
			err := logState.bufferLogs(job)
			if err != nil {
				ctxLogger.Warn(err)
				labLog.Warn(err)
			}
		} else if builderPhase == v1.PodPending {
			gitlabStatus := backChannel.syncJobStatus(protocol.Pending)
			if cancelRequested(gitlabStatus) {
				ctxLogger.Info("Job canceled")
				return
			} else {
				podInfo := podInfoMessage(status.Pods)
				labLog.Infof("PENDING %s", podInfo)
			}
		}

		switch {
		case js.Failed > 0 ||
			(len(js.Conditions) > 0 && js.Conditions[0].Type == v12.JobFailed):
			labLog.Error("Job Failed")
			logPush()
			backChannel.syncJobStatus(protocol.Failed)
			return

		case js.Succeeded > 0 && js.Active == 0 ||
			(len(js.Conditions) > 0 && js.Conditions[0].Type == v12.JobComplete):
			labLog.Info("OK")
			logPush()
			backChannel.syncJobStatus(protocol.Success)
			return
		default:
			// Just push logs to gitlab
			logPush()
		}
	}

	// Out of loop means the runner is killed
	defer backChannel.syncJobStatus(protocol.Failed)
	labLog.Error("Runner was killed")
	logPush()
}

func pushLogsToGitlab(logState *LogState, backChannel *GitlabBackChannel) error {
	if logState.logBuffer.Len() > 0 {
		err := backChannel.writeLogLines(logState.logBuffer.Bytes(), logState.gitlabStartOffset)
		if err != nil {
			return err
		} else {
			// update next offset
			logState.gitlabStartOffset = logState.gitlabStartOffset + logState.logBuffer.Len()
			// reset buffer
			logState.logBuffer.Reset()
		}
	}

	return nil
}

func cancelRequested(state *protocol.RemoteJobState) bool {
	switch {
	case state == nil:
		return false
	case state.StatusCode != http.StatusOK:
		return true
	default:
		return false
	}
}

func podInfoMessage(pods []v1.Pod) string {
	perpod := make([]string, len(pods))

	for i, pod := range pods {
		var podState string
		if len(pod.Status.Conditions) == 0 {
			podState = "Unknown"
		} else {
			podState = string(pod.Status.Conditions[0].Type)
		}

		line := fmt.Sprintf("[%s: %s]", pod.Name, podState)
		perpod[i] = line
	}

	return strings.Join(perpod, ", ")
}
