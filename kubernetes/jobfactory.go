package kubernetes

import (
	"errors"
	"github.com/sirupsen/logrus"
	v13 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	v14 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sisyphus/protocol"
	"sisyphus/shell"
	"sync"
)

const sisyphusStorageClass = "topology-aware-fast"

var ensureOnce sync.Once

// Create new job and start it
func newJobFromGitLab(session *Session, namePrefix string, spec *protocol.JobSpec, k8sJobParams *K8SJobParameters, cacheBucket string) (*Job, error) {
	ensureOnce.Do(func() {
		err := ensureStorageClass(session.k8sClient)
		if err != nil {
			logrus.Error(err)
		}
	})

	// Create config map volume with entrypoint script
	script, err := shell.GenerateScript(spec, cacheBucket)
	if err != nil {
		return nil, err
	}

	entrypointTemplate := newEntryPointScript(namePrefix, script)
	entrypoint, err := session.k8sClient.CoreV1().ConfigMaps(session.Namespace).Create(entrypointTemplate)
	if err != nil {
		return nil, err
	}

	// Create new PVC
	pvcTemplate := newPvc(namePrefix, k8sJobParams.ResourceRequest[v1.ResourceStorage])
	pvc, err := session.k8sClient.CoreV1().PersistentVolumeClaims(session.Namespace).Create(pvcTemplate)
	if err != nil {
		return nil, err
	}

	// Create new Job
	qCpu, ok := k8sJobParams.ResourceRequest[v1.ResourceCPU]
	if !ok {
		return nil, errors.New("unknown quantity of cpu request")
	}
	jobTemplate := jobFromGitHubSpec(namePrefix, spec, k8sJobParams.ActiveDeadlineSec, k8sJobParams.NodeSelector, qCpu, entrypoint.Name, pvc.Name)
	k8sJob, err := session.k8sClient.BatchV1().Jobs(session.Namespace).Create(jobTemplate)
	if err != nil {
		return nil, err
	}

	theJob := Job{
		session:          session,
		k8sJob:           k8sJob,
		k8sEntrypointMap: entrypoint,
		k8sPvc:           pvc,
		k8sClient:        session.k8sClient,
		namespace:        session.Namespace,
		Name:             k8sJob.Name,
	}

	return assignOwners(theJob)
}

// Ensure that custom storage class for PVC is created
func ensureStorageClass(k8sClient *kubernetes.Clientset) error {
	_, err := k8sClient.StorageV1().StorageClasses().Get(sisyphusStorageClass, v12.GetOptions{})
	if err == nil {
		return nil
	}

	bindingMode := v14.VolumeBindingWaitForFirstConsumer
	sClass := v14.StorageClass{
		ObjectMeta: v12.ObjectMeta{
			Name: sisyphusStorageClass,
		},
		Provisioner:       "kubernetes.io/gce-pd",
		VolumeBindingMode: &bindingMode,
		Parameters: map[string]string{
			"type": "pd-ssd",
		},
	}

	_, err = k8sClient.StorageV1().StorageClasses().Create(&sClass)
	if err != nil {
		return err
	}

	return nil
}

// The Job is assigned as an owner of all other objects
func assignOwners(newJob Job) (*Job, error) {
	ownerRef := v12.OwnerReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       newJob.k8sJob.Name,
		UID:        newJob.k8sJob.UID,
	}

	modJob, err := patchEntryPoint(newJob, ownerRef)
	if err != nil {
		return nil, err
	}

	modJob, err = patchPvc(newJob, ownerRef)
	if err != nil {
		return nil, err
	}

	return modJob, nil
}

func patchEntryPoint(newJob Job, ownerRef v12.OwnerReference) (*Job, error) {
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

func patchPvc(newJob Job, ownerRef v12.OwnerReference) (*Job, error) {
	// Modify configMap script ownership
	origObj := newJob.k8sPvc
	modObj := origObj.DeepCopy()
	modObj.OwnerReferences = append(modObj.OwnerReferences, ownerRef)
	objectName := origObj.Name

	patchData, err := genPatch(origObj, modObj)
	if err != nil {
		return nil, err
	}

	modPvc, err := newJob.k8sClient.CoreV1().PersistentVolumeClaims(newJob.namespace).Patch(objectName, types.StrategicMergePatchType, patchData)
	if err != nil {
		return nil, err
	}
	newJob.k8sPvc = modPvc

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

func newPvc(nameTemplate string, volumeSize resource.Quantity) *v1.PersistentVolumeClaim {
	sClass := sisyphusStorageClass
	return &v1.PersistentVolumeClaim{
		ObjectMeta: v12.ObjectMeta{
			GenerateName: nameTemplate,
		},

		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			StorageClassName: &sClass,
			Resources: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceStorage: volumeSize,
				},
			},
		},
	}
}

// Create K8S job from github spec
func jobFromGitHubSpec(namePrefix string,
	spec *protocol.JobSpec,
	activeDeadlineSec int64,
	nodeSelector map[string]string,
	cpuRequest resource.Quantity,
	entryPointName string,
	pvcName string) *v13.Job {

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
					ActiveDeadlineSeconds: &activeDeadlineSec,

					Containers: []v1.Container{
						{
							Name:    ContainerNameBuilder,
							Command: []string{"/jobscripts/entrypoint.sh", "||", "sleep 5"},

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
								{
									Name:      "buildpvc",
									MountPath: "/build",
								},
							},

							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU: cpuRequest,
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

						{
							Name: "buildpvc",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},

					NodeSelector: nodeSelector,
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
