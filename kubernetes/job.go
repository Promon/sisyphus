package kubernetes

import (
	"bytes"
	"errors"
	"io"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sisyphus/protocol"
	"sisyphus/shell"
	"time"
)

const ContainerNameBuilder = "builder"

type Job struct {
	session *Session

	// The job submitted to the cluster
	k8sJobTemplate *batchv1.Job

	// Configmap with entrypoint script(s)
	k8sEntrypointMap *v1.ConfigMap

	// for faster access these values are copied from session
	k8sClient *kubernetes.Clientset
	namespace string
	Name      string
}

// Additional parameters for K8S job spec
type K8SJobParameters struct {
	ResourceRequest   v1.ResourceList `json:"resource_request"`
	ActiveDeadlineSec int64           `json:"active_deadline_sec"`
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
	JobStatus batchv1.JobStatus

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
	pods, err := getPodsOfController(j.k8sClient, j.namespace, j.k8sJobTemplate.UID)
	if err != nil {
		return nil, err
	}

	for _, p := range pods {
		for _, c := range p.Spec.Containers {
			phases[c.Name] = p.Status.Phase
		}
	}

	return &K8SJobStatus{
		JobStatus: sj.Status,
		Pods:      pods,
		PodPhases: phases,
	}, nil
}

// Get logs stream
func (j *Job) GetLog(sinceTime *time.Time) (*bytes.Buffer, error) {
	controllerUid := j.k8sJobTemplate.GetUID()
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
	defer j.k8sClient.CoreV1().ConfigMaps(j.namespace).Delete(j.k8sEntrypointMap.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
	return j.k8sClient.BatchV1().Jobs(j.namespace).Delete(j.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
}

// Create new job and start it
func newJobFromGitLab(session *Session, namePrefix string, spec *protocol.JobSpec, k8sJobParams *K8SJobParameters, cacheBucket string) (*Job, error) {
	// Create config map volume with entrypoint script
	script := shell.GenerateScript(spec, cacheBucket)
	entrypointTemplate := newEntryPointScript(namePrefix, script)

	entrypoint, err := session.k8sClient.CoreV1().ConfigMaps(session.Namespace).Create(entrypointTemplate)
	if err != nil {
		return nil, err
	}

	jobTemplate := jobFromGitHubSpec(namePrefix, spec, k8sJobParams, entrypoint.Name)
	k8sJob, err := session.k8sClient.BatchV1().Jobs(session.Namespace).Create(jobTemplate)
	if err != nil {
		return nil, err
	}

	return &Job{
		session:          session,
		k8sJobTemplate:   k8sJob,
		k8sEntrypointMap: entrypoint,
		k8sClient:        session.k8sClient,
		namespace:        session.Namespace,
		Name:             k8sJob.Name,
	}, nil
}

func newEntryPointScript(nameTemplate string, script string) *v1.ConfigMap {

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nameTemplate,
		},

		Data: map[string]string{
			"entrypoint.sh": script,
		},
	}
}

const (
	ConfigMapAccessMode int32 = 0744
)

func jobFromGitHubSpec(namePrefix string, spec *protocol.JobSpec, k8sParams *K8SJobParameters, entryPointName string) *batchv1.Job {
	backOffLimit := int32(1)
	accessMode := int32(ConfigMapAccessMode)

	theJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
		},

		Spec: batchv1.JobSpec{
			BackoffLimit: &backOffLimit,

			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy:         v1.RestartPolicyOnFailure,
					ActiveDeadlineSeconds: &k8sParams.ActiveDeadlineSec,
					Containers: []v1.Container{
						{
							Name:    ContainerNameBuilder,
							Command: []string{"/jobscripts/entrypoint.sh"},

							// Image
							Image:           spec.Image.Name,
							ImagePullPolicy: v1.PullAlways,

							Env: convertEnvVars(spec.Variables),

							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "jobscripts",
									MountPath: "/jobscripts",
									ReadOnly:  true,
								},
							},

							Resources: v1.ResourceRequirements{
								Requests: k8sParams.ResourceRequest,
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "jobscripts",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: entryPointName,
									},

									DefaultMode: &accessMode,
								},
							},
						},
					},

					NodeSelector: map[string]string{
						"cloud.google.com/gke-preemptible": "true",
					},
				},
			},
		},
	}

	return theJob
}

func convertEnvVars(vars []protocol.JobVariable) []v1.EnvVar {
	result := make([]v1.EnvVar, len(vars))

	for i, v := range vars {
		result[i] = v1.EnvVar{
			Name:  v.Key,
			Value: v.Value,
		}
	}

	return result
}
