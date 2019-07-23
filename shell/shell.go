package shell

import (
	"fmt"
	"net/url"
	"sisyphus/protocol"
	"strings"
)

// Shell script generator
const DefaultUploadName = "artifacts"

type ScriptContext struct {
	builder strings.Builder
}

// Generate job script
func GenerateScript(spec *protocol.JobSpec, cacheBucketName string) (string, error) {
	env := protocol.GetEnvVars(spec)
	ctx := ScriptContext{}

	ctx.printPrelude(spec.JobInfo.ProjectName)

	// GIT
	if env["GIT_STRATEGY"] == "none" {
		ctx.addFline("echo 'Skipping GIT checkout. GIT_STRATEGY = none'")
	} else {
		_, hasCacheVar := env[SfsEnvVarGitCache]
		if hasCacheVar && len(env[SfsEnvVarGitCache]) > 0 {
			// use gitcache
			ctx.printGitDownloadCache(env[SfsEnvVarGitCache])
		} else {
			// fetch git in normal way
			ctx.printGitClone()
			ctx.printGitCleanReset()
			ctx.printGitCheckout()
			ctx.printGitSyncSubmodules()
		}
	}

	// Download caches
	for _, cache := range spec.Cache {
		if cache.Policy == protocol.CachePolicyPull ||
			cache.Policy == protocol.CachePolicyPullPush ||
			cache.Policy == protocol.CachePolicyUndefined {

			ctx.printDownloadCache(&cache, cacheBucketName, spec.JobInfo.ProjectName)
		}
	}

	// Download dependencies
	for _, dep := range spec.Dependencies {
		ctx.printDownloadDependency(&dep)
	}

	// Steps from YAML
	for _, step := range spec.Steps {
		err := ctx.printJobStep(step)
		if err != nil {
			return "", err
		}
	}

	// Upload artifacts
	for _, artifact := range spec.Artifacts {
		ctx.printUploadArtifact(&artifact, spec.Id, spec.Token)
	}

	// Upload caches
	for _, cache := range spec.Cache {
		if cache.Policy != protocol.CachePolicyPull {
			ctx.printUploadCache(&cache, cacheBucketName, spec.JobInfo.ProjectName)
		}
	}

	return ctx.builder.String(), nil
}

func (s *ScriptContext) addFline(format string, a ...interface{}) {
	fmt.Fprintf(&s.builder, format, a...)
	fmt.Fprintln(&s.builder)
}

func (s *ScriptContext) addLines(lines []string) {
	for _, l := range lines {
		fmt.Fprintln(&s.builder, l)
	}
}

func (s *ScriptContext) printPrelude(projectName string) {
	lines := []string{
		"#!/usr/bin/env bash",
		"# Prelude",
		"set -euxo",
	}

	s.addLines(lines)

	// Make working dir
	projectDir := "/build/sfs"
	s.addFline("export CI_PROJECT_DIR=%s", projectDir)
	s.addFline("rm -rf %s", projectDir)
	s.addFline("mkdir -p '%s'", projectDir)
	s.addFline("cd '%s'", projectDir)
	s.addFline("pwd")
}

// Generate git clone code
func (s *ScriptContext) printGitClone() {
	lines := []string{
		"# GIT Clone",
		"echo 'Cloning git repo'",
		"git clone ${CI_REPOSITORY_URL} ./",
		"git config fetch.recurseSubmodules false",
		"git fetch --prune",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitFetch() {
	lines := []string{
		"echo 'Fetching git remotes'",
		"git remote set-url origin ${CI_REPOSITORY_URL}",
		"git config fetch.recurseSubmodules false",
		"git fetch --prune",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitCheckout() {
	lines := []string{
		"# Git checkout",
		"echo \"Checking out ${CI_COMMIT_SHA}\"",
		"git checkout -f -q ${CI_COMMIT_SHA}",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitCleanReset() {
	lines := []string{
		"# GIT cleanup",
		"rm -f '.git/index.lock'",
		"rm -f '.git/shallow.lock'",
		"rm -f '.git/HEAD.lock'",
		"rm -f '.git/hooks/post-checkout'",
		"git clean -ffdx",
		"git reset --hard",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitSyncSubmodules() {
	lines := []string{
		"echo 'Synchronizing submodules'",
		"git submodule sync --recursive",
		"git submodule foreach --recursive git clean -ffxd",
		"git submodule foreach --recursive git reset --hard",
		"git submodule update --init --recursive",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitDownloadCache(cacheUrl string) {
	s.addFline("# Fetching GIT cache from %s", cacheUrl)
	s.addFline("gsutil cat %s | tar -zx", cacheUrl)

	// Cached git has wrong token in submodule urls
	s.addFline("# Fix CI tokens")
	s.addFline(`new_token="s/gitlab-ci-token:[a-zA-Z0-9_-]\+/gitlab-ci-token:${CI_JOB_TOKEN}/g"
sed -i -e ${new_token} '.git/config'

for CNF in $(find ./.git/modules/ -type f -name 'config'); do
	sed -i -e ${new_token} "${CNF}"
done`)

	s.printGitCleanReset()
	s.printGitFetch()
	s.printGitCheckout()
	s.printGitSyncSubmodules()
}

func (s *ScriptContext) printJobStep(step protocol.JobStep) error {
	s.addFline("# STEP %s", step.Name)
	s.addFline("echo 'Step `%s` has %d commands'", step.Name, len(step.Script))
	augmented, err := genStepScript(step.Script)
	if err != nil {
		return err
	}

	s.addFline(augmented)
	return nil
}

func (s *ScriptContext) printUploadArtifact(artifact *protocol.JobArtifact, jobId int, jobToken string) {
	s.addFline("# Upload artifact %s", artifact.Name)
	s.addFline("TMPDIR=$(mktemp -d)")

	// ZIP command
	inFiles := strings.Join(artifact.Paths, " ")
	zipFile := fmt.Sprintf("${TMPDIR}/%s.zip", DefaultUploadName)
	zipCommand := fmt.Sprintf("zip -p -r %s %s", zipFile, inFiles)
	s.addFline(zipCommand)

	// Upload
	uploadLines := genUploadArtifactSnippet(artifact, jobId, jobToken, zipFile)
	s.addLines(uploadLines)

	// Cleanup
	lines := []string{
		fmt.Sprintf("(rm -rf ${TMPDIR}) || true"),
		"unset TMPDIR",
	}
	s.addLines(lines)
}

func (s *ScriptContext) printDownloadDependency(dep *protocol.JobDependency) {
	s.addFline("# Download job dependency %s", dep.Name)
	s.addFline("TMPDIR=$(mktemp -d)")

	// Download
	dlFile := fmt.Sprintf("${TMPDIR}/%s.zip", DefaultUploadName)
	dlLines := genDownloadArtifactsSnippet(dep, dlFile)
	s.addLines(dlLines)

	// Unzip
	unzipCommand := fmt.Sprintf("unzip -o %s", dlFile)
	s.addFline(unzipCommand)

	// cleanup
	lines := []string{
		fmt.Sprintf("(rm -rf ${TMPDIR}) || true"),
		"unset TMPDIR",
	}
	s.addLines(lines)
}

func genUploadArtifactSnippet(artifact *protocol.JobArtifact, jobId int, jobToken string, localFilePath string) []string {
	q := url.Values{}
	if len(artifact.ExpireIn) > 0 {
		q.Set("expire_in", artifact.ExpireIn)
	}

	// Upload command
	postUrl := fmt.Sprintf("${CI_API_V4_URL}/jobs/%d/artifacts?%s", jobId, q.Encode())
	curlCmd := fmt.Sprintf("curl -H \"JOB-TOKEN: %s\" -F \"file=@%s\" %s", jobToken, localFilePath, postUrl)

	return []string{
		curlCmd,
	}
}

func genDownloadArtifactsSnippet(dep *protocol.JobDependency, outputFile string) []string {
	getUrl := fmt.Sprintf("${CI_API_V4_URL}/jobs/%d/artifacts", dep.Id)
	curlCmd := fmt.Sprintf("curl -H \"JOB-TOKEN: %s\" --output \"%s\" %s", dep.Token, outputFile, getUrl)

	return []string{
		curlCmd,
	}
}

func makeCacheUrl(cacheBucketName string, projectName string, cacheKey string) string {
	return fmt.Sprintf("gs://%s/%s/%s.tar.gz", cacheBucketName, projectName, cacheKey)
}

func (s *ScriptContext) printDownloadCache(cache *protocol.JobCache, cacheBucketName string, projectName string) {
	cUrl := makeCacheUrl(cacheBucketName, projectName, cache.Key)
	line := fmt.Sprintf("(gsutil cat %s | tar -zx) || echo \"No cache file found %s\"", cUrl, cUrl)

	lines := []string{
		fmt.Sprintf("echo \"Downloading cache %s from %s\"", cache.Key, cUrl),
		line,
	}

	s.addLines(lines)
}

func (s *ScriptContext) printUploadCache(cache *protocol.JobCache, cacheBucketName string, projectName string) {
	cUrl := makeCacheUrl(cacheBucketName, projectName, cache.Key)
	inDirs := strings.Join(cache.Paths, " ")

	tarCmd := fmt.Sprintf("tar -cz %s", inDirs)
	line := fmt.Sprintf("(%s | gsutil cp - %s) || true", tarCmd, cUrl)

	lines := []string{
		fmt.Sprintf("echo \"Uploading cache %s to %s\"", cache.Key, cUrl),
		line,
	}

	s.addLines(lines)
}

// Augment script lines
func genStepScript(lines []string) (string, error) {
	var sb strings.Builder

	for _, l := range lines {
		_, err := sb.WriteString(l)
		if err != nil {
			return "", err
		}

		_, err = fmt.Fprintln(&sb)
		if err != nil {
			return "", err
		}
	}

	// Close subshell
	errHandler := fmt.Sprintf("(%s) || (EXIT=$?; echo \"Failed with code $EXIT\"; sleep 10 && exit $EXIT)", sb.String())
	return errHandler, nil
}
