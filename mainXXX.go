package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"

	// GCP Auth provider
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func mainXXX() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	job := createJob()
	sjob, err := clientset.BatchV1().Jobs("default").Create(job)

	if err != nil {
		panic(err.Error())
	}

	for i := 0; i < 10; i++ {

		jobz, err := clientset.BatchV1().Jobs("default").Get(sjob.Name, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		fmt.Print(jobz)
	}

	const testJobName = "test-job-1"
	defer deleteJob(clientset, testJobName)

	jobdesc, err := json.MarshalIndent(sjob, "", " ")
	fmt.Println(string(jobdesc))

	zpods, err := getPodsOfController(clientset, sjob.ObjectMeta.UID)

	fmt.Printf("Job UID = %v", sjob.ObjectMeta.UID)
	fmt.Println()

	for id, pod := range zpods {
		for _, co := range pod.Spec.Containers {
			if co.Name == "fubar" {

				clientset.CoreV1().ComponentStatuses()

				fmt.Printf("FUBAR POD %v, %v", id, pod.Name)
				fmt.Println()

				logOpts := &v1.PodLogOptions{
					Container:  "fubar",
					Follow:     true,
					Timestamps: true,
				}
				logRequest := clientset.CoreV1().Pods("default").GetLogs(pod.Name, logOpts)

				rd, err := logRequest.Stream()

				if err != nil {
					panic(err.Error())
				}

				err = nil

				for err == nil || err != io.EOF {
					b, err := ioutil.ReadAll(rd)

					switch err {
					case io.EOF:
						fmt.Println("END")
					case nil:
						break
					default:
						panic(err.Error())
					}

					fmt.Print(string(b))
				}
			}
		}

	}
}

func deleteJob(clientset *kubernetes.Clientset, name string) {
	prop := metav1.DeletePropagationBackground
	err := clientset.BatchV1().Jobs("default").Delete("test-job-1",
		&metav1.DeleteOptions{
			PropagationPolicy: &prop,
		},
	)

	if err != nil {
		fmt.Println(err.Error())
	}
}

func createJob() *batchv1.Job {
	theJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-job-1",
		},

		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyOnFailure,
					Containers: []v1.Container{
						{
							Name:    "fubar",
							Command: []string{"printenv"},
							Image:   "ubuntu:latest",
						},
					},
				},
			},
		},
	}

	return theJob
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

// List pods belonging to the same controller. For example Job
func getPodsOfController(clientSet *kubernetes.Clientset, controllerUid types.UID) ([]v1.Pod, error) {
	labelSelector := fmt.Sprintf("controller-uid=%v", controllerUid)
	pl, err := clientSet.CoreV1().Pods("default").List(metav1.ListOptions{LabelSelector: labelSelector})

	if err != nil {
		return nil, err
	}

	return pl.Items, nil
}
