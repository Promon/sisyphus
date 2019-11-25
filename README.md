### Sisyphus - Kubernetes-only gitlab runner

This is a simple Kubernetes-only runner for gitlab.
It aims to be a more stable alternative to original gitlab runner.

This runner was created specifically for product needs and may not fit other projects. It was developed and tested only
on GCP. Other providers like AWS, DO etc were not tested and probably will require additional adjustments.

This runner uses Kubernetes [Job](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.15/#job-v1-batch)
objects. The main advantage of it is that the K8S makes the best effort to run the Job to completion under many adverse scenarios.
For example, the node running the job may "dissappear",
then the K8S will rerun the job on another node until it gets the exit code from main process.
This is particularly useful if you are using cheap spare capacity nodes like preemptible nodes on GCP.

Currently this runner supports:

| Feature | Status |
|---------|--------|
| 	Variables               | yes | `json:"variables"`
|  	Image                   | yes | `json:"image"`
|  	Services                | **no** | `json:"services"`
|  	Artifacts               | yes | `json:"artifacts"`
|  	Cache                   | yes | `json:"cache"`
|  	Shared                  | yes | `json:"shared"`
|  	UploadMultipleArtifacts | yes | `json:"upload_multiple_artifacts"`
|  	UploadRawArtifacts      | **no** | `json:"upload_raw_artifacts"`
|  	Session                 | **no** | `json:"session"`
|  	Terminal                | **no** | `json:"terminal"`
|  	Refspecs                | yes | `json:"refspecs"`
|  	Masking                 | **no** | `json:"masking"`
|  	Proxy                   | **no** | `json:"proxy"`

### Building and deploying
The build and deployments is handled using skaffold, docker, and helm scripts (files/charts). 