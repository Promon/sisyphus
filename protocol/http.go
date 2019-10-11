package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
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
		return nil, err
	}
	//noinspection GoUnhandledErrorResult
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	// No new jobs
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	// Gitlab answers with 201 Created for new jobs
	if resp.StatusCode == http.StatusCreated {
		//noinspection GoShadowedVar
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if log.GetLevel() >= log.TraceLevel {
			var prettyBuff bytes.Buffer
			err = json.Indent(&prettyBuff, body, "", " ")
			if err != nil {
				log.Warn(err)
			}

			log.Tracef("Received new job `%s`", prettyBuff.String())
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
	//noinspection GoUnhandledErrorResult
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	remoteJobState := RemoteJobState{
		StatusCode:  resp.StatusCode,
		RemoteState: resp.Header.Get("Job-Status"),
	}

	return &remoteJobState, nil
}

// Update Job logs
func (s *RunnerHttpSession) PatchJobLog(jobId int, jobToken string, content []byte, startOffset int) error {
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

	req.Header.Add("Content-Type", "text/plain")
	contentLen := fmt.Sprintf("%d", len(content))
	req.Header.Add("Content-Length", contentLen)
	req.Header.Add("Job-Token", jobToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	//noinspection GoUnhandledErrorResult
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusAccepted {
		return errors.New(fmt.Sprintf("http status is not 2xx '%d' msg '%s'", resp.StatusCode, resp.Status))
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

	newClient := http.Client{
		Timeout: 10 * time.Second,
	}

	return &RunnerHttpSession{
		BaseUrl: v,
		client:  &newClient,
	}, nil
}
