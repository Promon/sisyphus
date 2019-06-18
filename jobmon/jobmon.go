package jobmon

import (
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"net/http"
	k "sisyphus/kubernetes"
	"sisyphus/protocol"
)

// Monitor job
func MonitorJob(job *k.Job, httpSession *protocol.RunnerHttpSession, jobId int, gitlabJobToken string, workOk <-chan bool) {
	ctxLogger := logrus.WithFields(
		logrus.Fields{
			"jobName": job.Name,
		})

	logState := LogState{
		localLogger:       ctxLogger,
		gitlabStartOffset: 0,
	}

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

	backChannel.syncJobStatus(protocol.Pending)

	for range workOk {
		status, err := job.GetReadinessStatus()
		if err != nil {
			logrus.Warn(err)
		}

		js := status.JobStatus
		ctxLogger.Debugf("Status Active %v, failed %v, succeeded %v", js.Active, js.Failed, js.Succeeded)

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
			}

			// Push logs buffer to gitlab
			if logState.logBuffer.Len() > 0 {
				err = backChannel.writeLogLines(logState.logBuffer.Bytes(), logState.gitlabStartOffset)
				if err != nil {
					ctxLogger.Warn("Failed to send logs to gitlab")
				} else {
					// update next offset
					logState.gitlabStartOffset = logState.gitlabStartOffset + logState.logBuffer.Len()
					// reset buffer
					logState.logBuffer.Reset()
				}
			}

		} else {
			status := backChannel.syncJobStatus(protocol.Pending)
			if cancelRequested(status) {
				ctxLogger.Info("Job canceled")
				return
			}
		}

		switch {
		case status.JobStatus.Failed > 0:
			backChannel.syncJobStatus(protocol.Failed)
			return
		case status.JobStatus.Succeeded > 0 && status.JobStatus.Active == 0:
			backChannel.syncJobStatus(protocol.Success)
			return
		}

	}

	// Out of loop means the runner is killed
	backChannel.syncJobStatus(protocol.Failed)
	ctxLogger.Debugf("EOF")
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
