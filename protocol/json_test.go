package protocol

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"testing"
)

func Test_parse_X(t *testing.T) {
	//tests := []struct {
	//	name string
	//}{
	//	// TODO: Add test cases.
	//}
	//for _, tt := range tests {
	//	t.Run(tt.name, func(t *testing.T) {
	//		ParseJobSpec()
	//	})
	//}

	jsonData, err := ioutil.ReadFile("testdata/job_spec.json")
	if err != nil {
		t.Error(err)
	}

	spec, err := ParseJobSpec(jsonData)
	if err != nil {
		t.Error(err)
	}

	fmt.Println(spec)
}

func Test_resource(t *testing.T) {
	ex := `{"cpu":"100m"}`
	q := make(v1.ResourceList)
	err := json.Unmarshal([]byte(ex), &q)
	if err != nil {
		t.Error(err)
	}
}
