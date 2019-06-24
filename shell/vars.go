package shell

// Special environment variables
const (
	// GCS url for git cache. If this variable is set, the runner will use git cache strategy instead of clone
	SfsEnvVarGitCache = "SFS_GIT_CACHE_URL"

	// Custom resources request, json encoded map
	SfsResourceRequest = "SFS_RESOURCE_REQUEST"

	// The number of seconds after job start until it is killed
	SfsActiveDeadline = "SFS_ACTIVE_DEADLINE_SEC"
)
