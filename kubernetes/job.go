package kubernetes

import (
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

// Get job status
func (j *Job) GetStatus() (*batchv1.JobStatus, error) {
	sj, err := j.k8sClient.BatchV1().Jobs(j.namespace).Get(j.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &sj.Status, nil
}

type ReadinessStatus struct {
	JobStatus      *batchv1.JobStatus
	PodPhases      map[string]v1.PodPhase
	PodPhaseCounts map[v1.PodPhase]int
}

func (j *Job) GetReadinessStatus() (*ReadinessStatus, error) {
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

	phaseCounts := map[v1.PodPhase]int{
		v1.PodPending:   0,
		v1.PodRunning:   0,
		v1.PodSucceeded: 0,
		v1.PodFailed:    0,
		v1.PodUnknown:   0,
	}

	for _, phase := range phases {
		phaseCounts[phase] = phaseCounts[phase] + 1
	}

	return &ReadinessStatus{
		&sj.Status,
		phases,
		phaseCounts,
	}, nil
}

// Get logs stream
func (j *Job) GetLog(sinceTime *time.Time) (io.ReadCloser, error) {
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

				req := j.k8sClient.CoreV1().Pods(j.namespace).GetLogs(pod.GetName(), &logOpts)
				return req.Stream()
			}
		}
	}

	return nil, errors.New("can not find running pod to extract logs")
}

// Delete job
func (j *Job) Delete() error {
	prop := metav1.DeletePropagationBackground
	defer j.k8sClient.CoreV1().ConfigMaps(j.namespace).Delete(j.k8sEntrypointMap.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
	return j.k8sClient.BatchV1().Jobs(j.namespace).Delete(j.Name, &metav1.DeleteOptions{PropagationPolicy: &prop})
}

// Create new job and start it
func newJobFromGitLab(session *Session, namePrefix string, spec *protocol.JobSpec, cacheBucket string) (*Job, error) {
	// Create config map volume with entrypoint script
	script := shell.GenerateScript(spec, cacheBucket)
	entrypointTemplate := newEntryPointScript(namePrefix, script)

	entrypoint, err := session.k8sClient.CoreV1().ConfigMaps(session.Namespace).Create(entrypointTemplate)
	if err != nil {
		return nil, err
	}

	jobTemplate := jobFromGitHubSpec(namePrefix, spec, entrypoint.Name)
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

func jobFromGitHubSpec(namePrefix string, spec *protocol.JobSpec, entryPointName string) *batchv1.Job {
	backOffLimit := int32(2)
	accessMode := int32(ConfigMapAccessMode)

	theJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
		},

		Spec: batchv1.JobSpec{
			BackoffLimit: &backOffLimit,

			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyOnFailure,

					Containers: []v1.Container{
						{
							Name: ContainerNameBuilder,
							// TODO : introduce script here
							Command: []string{"/jobscripts/entrypoint.sh"},
							//Args:    []string{"Hello World"},

							//
							Image: spec.Image.Name,

							Env: convertEnvVars(spec.Variables),

							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "jobscripts",
									MountPath: "/jobscripts",
									ReadOnly:  true,
								},
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
