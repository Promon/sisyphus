package protocol

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
)

type JobImage struct {
	Name string `json:"name"`
}

type JobVariable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Public bool   `json:"public"`
	Masked bool   `json:"masked"`
}

type JobStep struct {
	Name           string   `json:"name"`
	Script         []string `json:"script"`
	TimeoutSeconds int      `json:"timeout"`
	When           string   `json:"when"`
	AllowFailure   bool     `json:"allow_failure"`
}

type JobCache struct {
	Key    string   `json:"key"`
	Paths  []string `json:"paths"`
	Policy string   `json:"policy"`
}

type JobArtifact struct {
	Name     string   `json:"name"`
	Paths    []string `json:"paths"`
	When     string   `json:"when"`
	ExpireIn string   `json:"expire_in"`
}

type JobGitInfo struct {
	RepoUrl   string   `json:"repo_url"`
	Ref       string   `json:"ref"`
	Sha       string   `json:"sha"`
	BeforeSha string   `json:"before_sha"`
	RefType   string   `json:"ref_type"`
	Refspecs  []string `json:"refspecs"`
	Depth     int      `json:"depth"`
}

type JobInfo struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	ProjectId   int    `json:"project_id"`
	ProjectName string `json:"project_name"`
}

// Artifact dependency
type JobDependency struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type JobSpec struct {
	Id            int             `json:"id"`
	JobInfo       JobInfo         `json:"job_info"`
	Token         string          `json:"token"`
	AllowGitFetch bool            `json:"allow_git_fetch"`
	Image         JobImage        `json:"image"`
	GitInfo       JobGitInfo      `json:"git_info"`
	Variables     []JobVariable   `json:"variables"`
	Steps         []JobStep       `json:"steps"`
	Artifacts     []JobArtifact   `json:"artifacts"`
	Dependencies  []JobDependency `json:"dependencies"`
	Cache         []JobCache      `json:"cache"`
}

func ParseJobSpec(jsonData []byte) (*JobSpec, error) {
	logrus.Debug(string(jsonData))

	var spec JobSpec
	err := json.Unmarshal(jsonData, &spec)
	if err != nil {
		return nil, err
	}

	return &spec, nil
}
