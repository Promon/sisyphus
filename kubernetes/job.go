package kubernetes

import (
	"bytes"
	"errors"
	"io"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

const ContainerNameBuilder = "builder"

type Job struct {
	session *Session

	// The job submitted to the cluster
	k8sJob *batchv1.Job

	// Configmap with entrypoint script(s)
	k8sEntrypointMap *v1.ConfigMap

	// PVC for /build dir
	k8sPvc *v1.PersistentVolumeClaim

	// for faster access these values are copied from session
	k8sClient *kubernetes.Clientset
	namespace string
	Name      string
}

// Additional parameters for K8S job spec
type K8SJobParameters struct {
	NodeSelector      map[string]string `json:"node_selector"`
	ResourceRequest   v1.ResourceList   `json:"resource_request"`
	ActiveDeadlineSec int64             `json:"active_deadline_sec"`
}

// Get job status
func (j *Job) GetStatus() (*batchv1.JobStatus, error) {
	sj, err := j.k8sClient.BatchV1().Jobs(j.namespace).Get(j.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &sj.Status, nil
}

type K8SJobStatus struct {
	Job *batchv1.Job

	// Comprehensive pod status
	Pods      []v1.Pod
	PodPhases map[string]v1.PodPhase
}

func (j *Job) GetK8SJobStatus() (*K8SJobStatus, error) {
	sj, err := j.k8sClient.BatchV1().Jobs(j.namespace).Get(j.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Initialize values to Unknown
	specContainers := sj.Spec.Template.Spec.Containers
	phases := make(map[string]v1.PodPhase)
	for _, c := range specContainers {
		phases[c.Name] = v1.PodUnknown
	}

	// Get current states of container
	pods, err := getPodsOfController(j.k8sClient, j.namespace, j.k8sJob.UID)
	if err != nil {
		return nil, err
	}

	for _, p := range pods {
		for _, c := range p.Spec.Containers {
			phases[c.Name] = p.Status.Phase
		}
	}

	return &K8SJobStatus{
		Job:       sj,
		Pods:      pods,
		PodPhases: phases,
	}, nil
}

// Get logs stream
func (j *Job) GetLog(sinceTime *time.Time) (*bytes.Buffer, error) {
	controllerUid := j.k8sJob.GetUID()
	pods, err := getPodsOfController(j.k8sClient, j.namespace, controllerUid)

	if err != nil {
		return nil, err
	}

	for _, pod := range pods {
		for _, ctr := range pod.Spec.Containers {
			if ctr.Name == ContainerNameBuilder {

				logOpts := v1.PodLogOptions{
					Container:  ctr.Name,
					Timestamps: true,
				}

				if sinceTime != nil {
					logOpts.SinceTime = &metav1.Time{Time: *sinceTime}
				}
				resp := j.k8sClient.CoreV1().Pods(j.namespace).GetLogs(pod.GetName(), &logOpts)
				//noinspection GoShadowedVar
				respStream, err := resp.Stream()
				if err != nil {
					return nil, err
				}
				//noinspection GoUnhandledErrorResult,GoDeferInLoop
				defer respStream.Close()

				// Copy to buffer
				var buf bytes.Buffer
				_, err = io.Copy(&buf, respStream)
				if err != nil {
					return nil, err
				}

				return &buf, nil
			}
		}
	}

	return nil, errors.New("can not find running pod to extract logs")
}

// Delete job
func (j *Job) Delete() error {
	prop := metav1.DeletePropagationBackground
	//noinspection GoUnhandledErrorResult
	//defer j.k8sClient.CoreV1().ConfigMaps(j.namespace).Delete(j.k8sEntrypointMap.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
	return j.k8sClient.BatchV1().Jobs(j.namespace).Delete(j.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
}

const (
	ConfigMapAccessMode int32 = 0744
)
