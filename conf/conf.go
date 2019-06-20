package conf

import "gopkg.in/yaml.v2"

type SisyphusConf struct {
	// The url to gitlab. Not api url
	GitlabUrl string `yaml:"gitlab_url"`
	// The token for registered runner
	RunnerToken string `yaml:"runner_token"`
	// Kubernetes namespace where everything will be created
	K8SNamespace string `yaml:"k8s_namespace"`
	// GCP cache bucket
	GcpCacheBucket string `yaml:"gcp_cache_bucket"`
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
