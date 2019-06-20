package conf

import (
	"reflect"
	"testing"
)

func Test_Conf(t *testing.T) {
	orig := SisyphusConf{
		RunnerToken:    "abcdef1234567",
		GitlabUrl:      "https://www.gitlab.abc",
		GcpCacheBucket: "test_bucket",
		K8SNamespace:   "builder",
	}

	r, err := writeConf(&orig)
	if err != nil {
		t.Error(err)
	}

	deser, err := ReadSisyphusConf(r)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(orig, *deser) {
		t.Errorf("YAML parsing failed: got %v, want %v", deser, orig)
	}
}
