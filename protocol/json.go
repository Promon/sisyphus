package protocol

import (
	"encoding/json"
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

type CachePolicy string

const (
	CachePolicyUndefined CachePolicy = ""
	CachePolicyPullPush  CachePolicy = "pull-push"
	CachePolicyPull      CachePolicy = "pull"
	//CachePolicyPush      CachePolicy = "push"
)

type JobCache struct {
	Key    string      `json:"key"`
	Paths  []string    `json:"paths"`
	Policy CachePolicy `json:"policy"`
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
	Variables     []JobVariable   `json:"variables,omitempty"`
	Steps         []JobStep       `json:"steps,omitempty"`
	Artifacts     []JobArtifact   `json:"artifacts,omitempty"`
	Dependencies  []JobDependency `json:"dependencies,omitempty"`
	Cache         []JobCache      `json:"cache,omitempty"`
}

func ParseJobSpec(jsonData []byte) (*JobSpec, error) {
	var spec JobSpec
	err := json.Unmarshal(jsonData, &spec)
	if err != nil {
		return nil, err
	}

	cleanSpec := cleanupJobSpec(spec)
	return cleanSpec, nil
}

// Cleanup potential junk values sent by gitlab
func cleanupJobSpec(origSpec JobSpec) *JobSpec {
	newSpec := origSpec

	// Clean cache spec
	cleanCache := make([]JobCache, 0, len(origSpec.Cache))
	for _, c := range origSpec.Cache {
		if len(c.Key) > 0 && len(c.Paths) > 0 {
			cleanCache = append(cleanCache, c)
		}
	}
	newSpec.Cache = cleanCache

	// Clean artifacts
	cleanArtifacts := make([]JobArtifact, 0, len(origSpec.Artifacts))
	for _, a := range origSpec.Artifacts {
		if len(a.Paths) > 0 {
			cleanArtifacts = append(cleanArtifacts, a)
		}
	}
	newSpec.Artifacts = cleanArtifacts

	return &newSpec
}

func GetEnvVars(spec *JobSpec) map[string]string {
	r := make(map[string]string)

	for _, v := range spec.Variables {
		r[v.Key] = v.Value
	}

	return r
}

func ToFlatJson(v interface{}) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
