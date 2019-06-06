package kubernetes

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"

	// GCP Auth provider
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Client session
type Session struct {
	// Kubernetes namespace where all objects will be created
	Namespace string
	k8sClient *kubernetes.Clientset
}

type SessionInterface interface {
	CreateJob(name string) (*Job, error)
}

// Start new kubernetes session with configuration from home directory
func CreateK8SSession(namespace string) (*Session, error) {
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
		Namespace: namespace,
		k8sClient: clientset,
	}, nil
}

// Create new job template
func (s *Session) CreateJob(name string) (*Job, error) {
	job, err := newJob(s, name)
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
