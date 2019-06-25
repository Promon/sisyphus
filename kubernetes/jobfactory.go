package kubernetes

import (
	v13 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sisyphus/protocol"
	"sisyphus/shell"
)

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

	theJob := Job{
		session:          session,
		k8sJob:           k8sJob,
		k8sEntrypointMap: entrypoint,
		k8sClient:        session.k8sClient,
		namespace:        session.Namespace,
		Name:             k8sJob.Name,
	}

	return assignOwners(theJob)
}

// The Job is assigned as an owner of all other objects
func assignOwners(newJob Job) (*Job, error) {
	ownerRef := v12.OwnerReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       newJob.k8sJob.Name,
		UID:        newJob.k8sJob.UID,
	}

	// Modify configMap script ownership
	origObj := newJob.k8sEntrypointMap
	modObj := origObj.DeepCopy()
	modObj.OwnerReferences = append(modObj.OwnerReferences, ownerRef)
	objectName := origObj.Name

	patchData, err := genPatch(origObj, modObj)
	if err != nil {
		return nil, err
	}

	modMap, err := newJob.k8sClient.CoreV1().ConfigMaps(newJob.namespace).Patch(objectName, types.StrategicMergePatchType, patchData)
	if err != nil {
		return nil, err
	}

	newJob.k8sEntrypointMap = modMap

	return &newJob, nil
}

// Create entry point script
func newEntryPointScript(nameTemplate string, script string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: v12.ObjectMeta{
			GenerateName: nameTemplate,
		},

		Data: map[string]string{
			"entrypoint.sh": script,
		},
	}
}

// Create K8S job from github spec
func jobFromGitHubSpec(namePrefix string, spec *protocol.JobSpec, k8sParams *K8SJobParameters, entryPointName string) *v13.Job {
	backOffLimit := int32(1)
	accessMode := int32(ConfigMapAccessMode)

	theJob := &v13.Job{
		ObjectMeta: v12.ObjectMeta{
			GenerateName: namePrefix,
		},

		Spec: v13.JobSpec{
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
