package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// The HttpSession for the runner
type RunnerHttpSession struct {
	BaseUrl *url.URL

	// The http client can be used by multiple threads
	client *http.Client
}

const ContentTypeJson = "application/json"

const (
	PathApi        = "/api/v4"
	PathJobMailBox = PathApi + "/jobs/request"
	PathJobState   = PathApi + "/jobs/%d"
	PathJobTrace   = PathJobState + "/trace"
)

type JobState string

const (
	Pending JobState = "pending"
	Running JobState = "running"
	Failed  JobState = "failed"
	Success JobState = "success"
)

type FeaturesInfo struct {
	Variables               bool `json:"variables"`
	Image                   bool `json:"image"`
	Services                bool `json:"services"`
	Artifacts               bool `json:"artifacts"`
	Cache                   bool `json:"cache"`
	Shared                  bool `json:"shared"`
	UploadMultipleArtifacts bool `json:"upload_multiple_artifacts"`
	UploadRawArtifacts      bool `json:"upload_raw_artifacts"`
	Session                 bool `json:"session"`
	Terminal                bool `json:"terminal"`
	Refspecs                bool `json:"refspecs"`
	Masking                 bool `json:"masking"`
	Proxy                   bool `json:"proxy"`
}

type VersionInfo struct {
	Name         string       `json:"name,omitempty"`
	Version      string       `json:"version,omitempty"`
	Revision     string       `json:"revision,omitempty"`
	Platform     string       `json:"platform,omitempty"`
	Architecture string       `json:"architecture,omitempty"`
	Executor     string       `json:"executor,omitempty"`
	Shell        string       `json:"shell,omitempty"`
	Features     FeaturesInfo `json:"features"`
}

type JobRequest struct {
	Info  VersionInfo `json:"info,omitempty"`
	Token string      `json:"token"`
}

// Poll next job from the queue
func (s *RunnerHttpSession) PollNextJob(runnerToken string) (*JobSpec, error) {
	reqUrl, err := s.formatRequestUrl(PathJobMailBox)
	if err != nil {
		return nil, err
	}

	jr := newJobRequest(runnerToken)
	req, err := jsonRequest(http.MethodPost, reqUrl, jr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		debugResponse(resp)
		return nil, err
	}
	defer resp.Body.Close()

	// No new jobs
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	// Gitlab answers with 201 Created for new jobs
	if resp.StatusCode == http.StatusCreated {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		spec, err := ParseJobSpec(body)
		if err != nil {
			return nil, err
		}

		return spec, nil
	}

	return nil, errors.New(fmt.Sprintf("Unknown response code %v", resp.StatusCode))
}

func newJobRequest(runnerToken string) *JobRequest {
	return &JobRequest{
		Token: runnerToken,
		Info: VersionInfo{
			Features: FeaturesInfo{
				Cache:                   true,
				Variables:               true,
				Artifacts:               true,
				Image:                   true,
				Refspecs:                true,
				Shared:                  true,
				UploadMultipleArtifacts: true,

				// TODO: add support for services
				Services: false,
			},
		},
	}
}

// Generic JSON request
func jsonRequest(method string, requestUrl *url.URL, requestBody interface{}) (*http.Request, error) {
	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	reqRdr := bytes.NewReader(reqBody)
	req, err := http.NewRequest(method, requestUrl.String(), reqRdr)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", ContentTypeJson)
	req.Header.Add("Content-Type", ContentTypeJson)

	return req, nil
}

type UpdateJobStateRequest struct {
	//Info          VersionInfo      `json:"info,omitempty"`
	Token string   `json:"token,omitempty"`
	State JobState `json:"state,omitempty"`
	//FailureReason JobFailureReason `json:"failure_reason,omitempty"`
}

type RemoteJobState struct {
	StatusCode  int
	RemoteState string
}

// Synchronize local and remote status of the job
func (s *RunnerHttpSession) UpdateJobStatus(jobId int, jobToken string, state JobState) (*RemoteJobState, error) {
	request := UpdateJobStateRequest{
		Token: jobToken,
		State: state,
	}

	path := fmt.Sprintf(PathJobState, jobId)
	reqUrl, err := s.formatRequestUrl(path)
	if err != nil {
		return nil, err
	}

	req, err := jsonRequest(http.MethodPut, reqUrl, request)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rstate := RemoteJobState{
		StatusCode:  resp.StatusCode,
		RemoteState: resp.Header.Get("Job-Status"),
	}

	return &rstate, nil
}

// Update Job logs
func (s *RunnerHttpSession) PatchJobLog(jobId int, jobToken string, content []byte, startOffset int) error {
	endOffset := startOffset + len(content)
	path := fmt.Sprintf(PathJobTrace, jobId)
	reqUrl, err := s.formatRequestUrl(path)

	if err != nil {
		return err
	}

	reqBody := bytes.NewReader(content)
	req, err := http.NewRequest(http.MethodPatch, reqUrl.String(), reqBody)
	if err != nil {
		return err
	}

	contentRange := fmt.Sprintf("%d-%d", startOffset, endOffset-1)
	req.Header.Add("Content-Type", "text/plain")
	req.Header.Add("Content-Range", contentRange)
	req.Header.Add("Job-Token", jobToken)

	_, err = s.client.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func (s *RunnerHttpSession) formatRequestUrl(refPath string) (*url.URL, error) {
	refUrl, err := url.Parse(refPath)

	if err != nil {
		return nil, err
	}

	return s.BaseUrl.ResolveReference(refUrl), nil
}

// Create new http session
func NewHttpSession(baseUrl string) (*RunnerHttpSession, error) {
	v, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}

	newClient := http.Client{}
	return &RunnerHttpSession{
		BaseUrl: v,
		client:  &newClient,
	}, nil
}

func debugRequest(req *http.Request) {
	if log.GetLevel() < log.DebugLevel {
		return
	}

	b, err := httputil.DumpRequestOut(req, true)

	if err != nil {
		log.Warn(err)
	}

	log.Debugf("---REQUEST---\n%v\n---EOF REQUEST---", string(b))
}

func debugResponse(resp *http.Response) {
	if log.GetLevel() < log.DebugLevel {
		return
	}

	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Warn(err)
	}

	log.Debugf("---RESPONSE---\n%v\n---EOF RESPONSE---", string(b))
}
