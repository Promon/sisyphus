package jobmon

import (
	"github.com/sirupsen/logrus"
	"sisyphus/protocol"
)

type GitlabBackChannel struct {
	httpSession    *protocol.RunnerHttpSession
	jobId          int
	gitlabJobToken string
	localLogger    *logrus.Entry
}

func (bc *GitlabBackChannel) syncJobStatus(state protocol.JobState) *protocol.RemoteJobState {
	z, err := bc.httpSession.UpdateJobStatus(bc.jobId, bc.gitlabJobToken, state)

	if err != nil {
		bc.localLogger.Warn(err)
		return nil
	}

	return z
}

func (bc *GitlabBackChannel) writeLogLines(content []byte, startOffset int) error {
	return bc.httpSession.PatchJobLog(bc.jobId, bc.gitlabJobToken, content, startOffset)
}
