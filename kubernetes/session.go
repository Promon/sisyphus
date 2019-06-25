package kubernetes

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	Namespace string
	k8sClient *kubernetes.Clientset
}

// Start new kubernetes session with configuration from home directory
func CreateK8SSession(inCluster bool, namespace string) (*Session, error) {

	var config *rest.Config
	if inCluster {
		x, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		config = x
	} else {
		home := homeDir()
		kubeconfig := filepath.Join(home, ".kube", "config")

		// use the current context in kubeconfig
		x, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
		config = x
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
func (s *Session) CreateGitLabJob(namePrefix string, spec *protocol.JobSpec, k8sJobParams *K8SJobParameters, cacheBucket string) (*Job, error) {
	job, err := newJobFromGitLab(s, namePrefix, spec, k8sJobParams, cacheBucket)
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
