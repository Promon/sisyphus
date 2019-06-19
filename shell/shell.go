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
func GenerateScript(spec *protocol.JobSpec) string {

	ctx := ScriptContext{}

	ctx.printPrelude(spec.JobInfo.Name)

	// GIT
	ctx.printGitClone()
	ctx.printGitCleanReset()
	ctx.printGitCheckout()

	// Steps from YAML
	for _, step := range spec.Steps {
		ctx.printJobStep(step)
	}

	// Upload artifacts
	for _, artifact := range spec.Artifacts {
		ctx.printUploadArtifact(&artifact, spec.Id, spec.Token)
	}

	return ctx.builder.String()
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
		"set -euxo",
	}

	s.addLines(lines)

	// Make working dir
	wdir := fmt.Sprintf("/%s/%s", "build", projectName)
	s.addFline("mkdir -p '%s'", wdir)
	s.addFline("cd '%s'", wdir)
	s.addFline("pwd")
}

// Generate git clone code
func (s *ScriptContext) printGitClone() {
	lines := []string{
		"echo 'Fetching git remotes'",
		"git clone ${CI_REPOSITORY_URL} ./",
		"git config fetch.recurseSubmodules false",
		"git fetch --prune",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitCleanReset() {
	lines := []string{
		"rm -f '.git/index.lock'",
		"rm -f '.git/shallow.lock'",
		"rm -f '.git/HEAD.lock'",
		"rm -f '.git/hooks/post-checkout'",
		"git clean -ffdx",
		"git reset --hard",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printGitCheckout() {
	lines := []string{
		"echo \"Checking out ${CI_COMMIT_SHA}\"",
		"git checkout -f -q ${CI_COMMIT_SHA}",
	}

	s.addLines(lines)
}

func (s *ScriptContext) printJobStep(step protocol.JobStep) {
	s.addFline("echo 'Step `%s` has %d commands'", step.Name, len(step.Script))
	s.addLines(step.Script)
}

func (s *ScriptContext) printUploadArtifact(artifact *protocol.JobArtifact, jobId int, jobToken string) {
	s.addFline("TMPDIR=$(mktemp -d)")

	// ZIP command
	inFiles := strings.Join(artifact.Paths, " ")

	zipFile := fmt.Sprintf("${TMPDIR}/%s.zip", DefaultUploadName)
	zipCommand := fmt.Sprintf("zip -p %s %s", zipFile, inFiles)
	s.addFline(zipCommand)

	// Upload
	uploadLines := genUploadSnippet(artifact, jobId, jobToken, zipFile)
	s.addLines(uploadLines)

	// Cleanup
	lines := []string{
		fmt.Sprintf("(rm %s) || true", zipFile),
		"unset TMPDIR",
	}
	s.addLines(lines)
}

func genUploadSnippet(artifact *protocol.JobArtifact, jobId int, jobToken string, localFilePath string) []string {
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
