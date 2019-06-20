package kubernetes

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"sisyphus/protocol"

	// GCP Auth provider
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Client session
type Session struct {
	// Kubernetes namespace where all objects will be created
	Namespace                 string
	k8sClient                 *kubernetes.Clientset
	k8sDefaultResourceRequest v1.ResourceList
}

// Start new kubernetes session with configuration from home directory
func CreateK8SSession(namespace string, k8sDefaultResourceRequest v1.ResourceList) (*Session, error) {
	home := homeDir()
	kubeconfig := filepath.Join(home, ".kube", "config")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Session{
		Namespace:                 namespace,
		k8sClient:                 clientset,
		k8sDefaultResourceRequest: k8sDefaultResourceRequest,
	}, nil
}

// Create new job template
func (s *Session) CreateGitLabJob(namePrefix string, spec *protocol.JobSpec, cacheBucket string) (*Job, error) {
	job, err := newJobFromGitLab(s, namePrefix, spec, cacheBucket, s.k8sDefaultResourceRequest)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
