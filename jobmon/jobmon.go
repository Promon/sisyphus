package jobmon

import (
	"fmt"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"net/http"
	k "sisyphus/kubernetes"
	"sisyphus/protocol"
)

// Create job from descriptor and monitor loop
func RunJob(spec *protocol.JobSpec, k8sSession *k.Session, httpSession *protocol.RunnerHttpSession, workOk <-chan bool) {
	jobPrefix := fmt.Sprintf("sphs-%v-%v-", spec.JobInfo.ProjectId, spec.Id)

	job, err := k8sSession.CreateGitLabJob(jobPrefix, spec)
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

// Monitor job
func monitorJob(job *k.Job, httpSession *protocol.RunnerHttpSession, jobId int, gitlabJobToken string, workOk <-chan bool) {
	ctxLogger := logrus.WithFields(
		logrus.Fields{
			"jobName": job.Name,
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

	for range workOk {
		status, err := job.GetReadinessStatus()
		if err != nil {
			ctxLogger.Warn(err)
			labLog.Warn(err)
		}

		js := status.JobStatus
		ctxLogger.Debugf("Status Active %v, failed %v, succeeded %v", js.Active, js.Failed, js.Succeeded)
		ctxLogger.Debugf("Pod Phases %v", status.PodPhases)

		if status.PodPhaseCounts[v1.PodPending] == 0 && status.PodPhaseCounts[v1.PodUnknown] == 0 {

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
		} else {
			status := backChannel.syncJobStatus(protocol.Pending)
			if cancelRequested(status) {
				ctxLogger.Info("Job canceled")
				return
			} else {
				labLog.Info("PENDING")
			}
		}

		switch {
		case status.JobStatus.Failed > 0:
			labLog.Error("Job Failed")
			logPush()
			backChannel.syncJobStatus(protocol.Failed)
			return
		case status.JobStatus.Succeeded > 0 && status.JobStatus.Active == 0:
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
	ctxLogger.Debugf("EOF")
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
