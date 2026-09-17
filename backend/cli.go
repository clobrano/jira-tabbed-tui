package backend

import (
	"fmt"
	"os/exec"
	"strings"
)

// Runner is the interface all backend operations use to call the Jira CLI.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// CLIRunner shells out to the configured Jira CLI binary.
type CLIRunner struct {
	Binary    string
	ExtraArgs []string
}

// NewCLIRunner creates a runner for the given binary.
func NewCLIRunner(binary string, extraArgs []string) *CLIRunner {
	return &CLIRunner{Binary: binary, ExtraArgs: extraArgs}
}

func (r *CLIRunner) Run(args ...string) ([]byte, error) {
	allArgs := make([]string, 0, len(r.ExtraArgs)+len(args))
	allArgs = append(allArgs, r.ExtraArgs...)
	allArgs = append(allArgs, args...)
	cmd := exec.Command(r.Binary, allArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("%s", stderr)
			}
		}
		return nil, err
	}
	return out, nil
}

// FakeRunner is a test double that returns pre-canned responses keyed by joined args.
type FakeRunner struct {
	responses map[string][]byte
	errors    map[string]error
}

// NewFakeRunner creates an empty FakeRunner.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		responses: make(map[string][]byte),
		errors:    make(map[string]error),
	}
}

// Register registers a successful response for the given args.
func (f *FakeRunner) Register(response []byte, args ...string) {
	f.responses[strings.Join(args, " ")] = response
}

// RegisterError registers an error response for the given args.
func (f *FakeRunner) RegisterError(err error, args ...string) {
	f.errors[strings.Join(args, " ")] = err
}

func (f *FakeRunner) Run(args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	if data, ok := f.responses[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("FakeRunner: no response registered for: %s", key)
}
