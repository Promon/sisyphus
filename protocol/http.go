package protocol

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// The HttpSession for the runner
type RunnerHttpSession struct {
	BaseUrl      *url.URL
	PrivateToken string

	// The http client can be used by multiple threads
	client *http.Client
}

const (
	PathJobMailBox = "/api/v4/jobs/request"

	PathGetProjects = "/api/v4/projects?per_page=1"
)

func (s *RunnerHttpSession) MakeRequest() error {
	resp, err := s.client.Get(s.BaseUrl.String())
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	if err != nil {
		return err
	}

	fmt.Println(string(body))

	fmt.Printf("encodings %v", resp.TransferEncoding)
	fmt.Println()
	return nil
}

func (s *RunnerHttpSession) PollProjects() error {
	reqUrl, err := s.formatRequestUrl(PathGetProjects)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodGet, reqUrl.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Private-Token", s.PrivateToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var x interface{}
	err = json.Unmarshal(body, &x)
	if err != nil {
		return err
	}

	//m := x.(map[string]interface{})
	fmt.Printf("%v", x)

	fmt.Println(string(body))
	return nil
}

//
func (s *RunnerHttpSession) formatRequestUrl(refPath string) (*url.URL, error) {
	refUrl, err := url.Parse(refPath)

	if err != nil {
		return nil, err
	}

	return s.BaseUrl.ResolveReference(refUrl), nil
}

func NewHttpSession(baseUrl string, privateToken string) (*RunnerHttpSession, error) {
	v, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}

	newClient := http.Client{}
	return &RunnerHttpSession{
		BaseUrl:      v,
		PrivateToken: privateToken,
		client:       &newClient,
	}, nil
}
