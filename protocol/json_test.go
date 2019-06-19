package protocol

import (
	"fmt"
	"io/ioutil"
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
