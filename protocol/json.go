package protocol

import "encoding/json"

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

type JobSpec struct {
	Id            int64         `json:"id"`
	Token         string        `json:"token"`
	AllowGitFetch bool          `json:"allow_git_fetch"`
	Variables     []JobVariable `json:"variables"`
	Steps         []JobStep     `json:"steps"`
}

func parse(jsonData []byte) (*JobSpec, error) {
	var spec JobSpec
	err := json.Unmarshal(jsonData, &spec)
	if err != nil {
		return nil, err
	}

	return &spec, nil
}
