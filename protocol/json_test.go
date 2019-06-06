package protocol

import (
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
	//		parse()
	//	})
	//}

	jsonData, err := ioutil.ReadFile("testdata/job_spec.json")
	if err != nil {
		t.Error(err)
	}

	_, err = parse(jsonData)
	if err != nil {
		t.Error(err)
	}
}
