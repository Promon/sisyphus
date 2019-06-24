package shell

// Special environment variables
const (
	// GCS url for git cache. If this variable is set, the runner will use git cache strategy instead of clone
	EnvVarGitCache = "SFS_GIT_CACHE_URL"
)
