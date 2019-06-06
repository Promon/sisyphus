package kubernetes

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// List pods belonging to the same controller. For example Job
func getPodsOfController(clientSet *kubernetes.Clientset, namespace string, controllerUid types.UID) ([]v1.Pod, error) {
	labelSelector := fmt.Sprintf("controller-uid=%v", controllerUid)
	pl, err := clientSet.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: labelSelector})

	if err != nil {
		return nil, err
	}

	return pl.Items, nil
}
