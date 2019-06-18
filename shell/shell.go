package shell

import (
	"fmt"
	"sisyphus/protocol"
	"strings"
)

// Shell script generator

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

	for _, step := range spec.Steps {
		ctx.printJobStep(step)
	}

	return ctx.builder.String()
}

func (s *ScriptContext) printFLine(format string, a ...interface{}) {
	fmt.Fprintf(&s.builder, format, a...)
	fmt.Fprintln(&s.builder)
}

func (s *ScriptContext) addLines(lines []string) {
	for _, l := range lines {
		fmt.Fprintln(&s.builder, l)
	}
}

func (s *ScriptContext) printPrelude(projectName string) {
	lines := []string{"#!/usr/bin/env bash",
		"set -euxo",
		"echo 'Hello World !'"}

	s.addLines(lines)

	// Make working dir
	wdir := fmt.Sprintf("/%s/%s", "build", projectName)
	s.printFLine("mkdir -p '%s'", wdir)
	s.printFLine("cd '%s'", wdir)
	s.printFLine("pwd")
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
	s.printFLine("echo 'Step %s has %d commands'", step.Name, len(step.Script))
	s.addLines(step.Script)
}
