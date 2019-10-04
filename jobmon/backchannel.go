package jobmon

import (
	"errors"
	"github.com/sirupsen/logrus"
	"sisyphus/protocol"
)

type gitLabBackChannel struct {
	httpSession    *protocol.RunnerHttpSession
	jobId          int
	gitlabJobToken string
	localLogger    *logrus.Entry
}

func (bc *gitLabBackChannel) syncJobStatus(state protocol.JobState) (*protocol.RemoteJobState, error) {
	z, err := bc.httpSession.UpdateJobStatus(bc.jobId, bc.gitlabJobToken, state)

	if err != nil {
		return nil, err
	}

	if z == nil {
		return nil, errors.New("nil response from GitLab server")
	}
	return z, nil
}

func (bc *gitLabBackChannel) writeLogLines(content []byte, startOffset int) error {
	return bc.httpSession.PatchJobLog(bc.jobId, bc.gitlabJobToken, content, startOffset)
}
