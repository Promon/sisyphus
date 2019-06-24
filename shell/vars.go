package shell

// Special environment variables
const (
	// GCS url for git cache. If this variable is set, the runner will use git cache strategy instead of clone
	SfsEnvVarGitCache = "SFS_GIT_CACHE_URL"

	// Custom resources request, json encoded
	SfsResourceRequest = "SFS_RESOURCE_REQUEST"
)
