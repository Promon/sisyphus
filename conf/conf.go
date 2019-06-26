package conf

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceQuantity struct {
	Type     v1.ResourceName `yaml:"type",json:"type"`
	Quantity string          `yaml:"quantity",json:"quantity"`
}

type SisyphusConf struct {
	// The url to gitlab. Not api url
	GitlabUrl string `yaml:"gitlab_url"`
	// The token for registered runner
	RunnerToken string `yaml:"runner_token"`
	// Kubernetes namespace where everything will be created
	K8SNamespace string `yaml:"k8s_namespace"`
	// GCP cache bucket
	GcpCacheBucket string `yaml:"gcp_cache_bucket"`

	// Optional Default resource requests for new jobs
	DefaultResourceRequest []ResourceQuantity `yaml:"default_resource_request"`
}

func ReadSisyphusConf(yamlRaw []byte) (*SisyphusConf, error) {
	var conf SisyphusConf

	err := yaml.Unmarshal(yamlRaw, &conf)
	if err != nil {
		return nil, err
	}

	return &conf, nil
}

func writeConf(conf *SisyphusConf) ([]byte, error) {
	raw, err := yaml.Marshal(conf)

	if err != nil {
		return nil, err
	}

	return raw, nil
}

// Parse configured resource quantity to K8S types
func ParseResourceQuantity(confResources []ResourceQuantity) (v1.ResourceList, error) {
	result := make(map[v1.ResourceName]resource.Quantity)

	for _, q := range confResources {
		k8sQ, err := resource.ParseQuantity(q.Quantity)
		if err != nil {
			return nil, err
		}

		result[q.Type] = k8sQ
	}

	return result, nil
}

// Validate resource quantity
func ValidateDefaultResourceQuantity(s v1.ResourceList) error {
	// These quantities must be in configuration
	resourceRequiredKeys := []v1.ResourceName{
		v1.ResourceCPU, v1.ResourceStorage, v1.ResourceEphemeralStorage,
	}

	for _, k := range resourceRequiredKeys {
		_, ok := s[k]
		if !ok {
			return errors.New(fmt.Sprintf("default resource quantity for '%s' is missing", k))
		}
	}

	return nil
}
