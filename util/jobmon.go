package util

import (
	"bufio"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	v1 "k8s.io/api/core/v1"
	"net/http"
	k "sisyphus/kubernetes"
	"sisyphus/protocol"
	"strings"
	"time"
)

type LogLine struct {
	timestamp time.Time
	text      string
}

type LogState struct {
	lastSeenLine *LogLine
}

type GitlabBackChannel struct {
	httpSession    *protocol.RunnerHttpSession
	jobId          int
	gitlabJobToken string
	localLogger    *logrus.Entry
}

// Monitor job
func MonitorJob(job *k.Job, httpSession *protocol.RunnerHttpSession, jobId int, gitlabJobToken string, workOk <-chan bool) {
	ctxLogger := logrus.WithFields(
		logrus.Fields{
			"jobName": job.Name,
		})

	logState := LogState{}

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

			err := logState.updateLogs(job)
			if err != nil {
				ctxLogger.Warn(err)
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

func (ls *LogState) printLog(log io.ReadCloser) error {
	sc := bufio.NewScanner(log)
	for sc.Scan() {
		timestamped := sc.Text()
		parsed, err := parseLogLine(timestamped)
		if err != nil {
			return err
		}

		// skip lines older than last seen
		if ls.lastSeenLine == nil || parsed.timestamp.After(ls.lastSeenLine.timestamp) {
			// remember last line we seen
			ls.lastSeenLine = parsed
			fmt.Println(timestamped)
		}
	}

	return nil
}

func (ls *LogState) updateLogs(job *k.Job) error {
	var sinceTime *time.Time = nil

	if ls.lastSeenLine != nil {
		sinceTime = &ls.lastSeenLine.timestamp
	}

	rdr, err := job.GetLog(sinceTime)
	if err != nil {
		return err
	}
	defer rdr.Close()

	return ls.printLog(rdr)
}

func (bc *GitlabBackChannel) syncJobStatus(state protocol.JobState) *protocol.RemoteJobState {
	z, err := bc.httpSession.UpdateJobStatus(bc.jobId, bc.gitlabJobToken, state)

	if err != nil {
		bc.localLogger.Warn(err)
		return nil
	}

	return z
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

// Split log line to timestamp and text
func parseLogLine(logLine string) (*LogLine, error) {
	parts := strings.SplitN(logLine, " ", 2)
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}

	return &LogLine{
		timestamp: ts,
		text:      parts[1],
	}, nil
}
