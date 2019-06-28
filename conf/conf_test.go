package conf

import (
	"fmt"
	"reflect"
	"testing"
)

func Test_Conf(t *testing.T) {
	orig := SisyphusConf{
		RunnerToken:    "abcdef1234567",
		GitlabUrl:      "https://www.gitlab.abc",
		GcpCacheBucket: "test_bucket",
		K8SNamespace:   "builder",

		DefaultNodeSelector: map[string]string{
			"cloud.google.com/gke-preemptible": "true",
			"class":                            "sisyphus",
		},

		DefaultResourceRequest: []ResourceQuantity{
			{Type: "cpu", Quantity: "1000m"},
		},
	}

	r, err := writeConf(&orig)
	if err != nil {
		t.Error(err)
	}

	fmt.Println(string(r))

	deser, err := ReadSisyphusConf(r)
	if err != nil {
		t.Error(err)
	}

	y := *deser
	if !reflect.DeepEqual(orig, y) {
		t.Errorf("YAML parsing failed: got %v, want %v", deser, orig)
	}
}
