//go:build darwin || linux

package checkrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

type pipeFailureProbe struct {
	Result       Result `json:"result"`
	Error        string `json:"error"`
	IsEMFILE     bool   `json:"is_emfile"`
	HeldFDs      int    `json:"held_fds"`
	SetupFailure string `json:"setup_failure,omitempty"`
}

func TestRunPipeSetupFailureKeepsNoStartFacts(t *testing.T) {
	if os.Getenv("HAFT_PIPE_FAILURE_HELPER") == "1" {
		probe := runPipeFailureProbe()
		if err := json.NewEncoder(os.Stdout).Encode(probe); err != nil {
			os.Exit(2)
		}
		return
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-test.run=^TestRunPipeSetupFailureKeepsNoStartFacts$")
	command.Env = append(os.Environ(), "HAFT_PIPE_FAILURE_HELPER=1")
	raw, err := command.Output()
	if err != nil {
		t.Fatalf("owned EMFILE helper: %v", err)
	}
	var probe pipeFailureProbe
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&probe); err != nil {
		t.Fatalf("owned EMFILE helper result: %v: %s", err, raw)
	}
	if probe.SetupFailure != "" {
		t.Fatal(probe.SetupFailure)
	}
	result := probe.Result
	if !probe.IsEMFILE || !strings.Contains(probe.Error, "pipe") || probe.HeldFDs < 1 {
		t.Fatalf("actual owned descriptor exhaustion did not reach a pipe failure: %+v", probe)
	}
	if result.Started || result.ExitCode != nil || result.Failure != "start_failed" || result.StdoutBytes != 0 || result.StderrBytes != 0 || len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		t.Fatalf("pre-start process or stream facts were fabricated: %+v", result)
	}
	if result.StdoutDigest != carrier.Digest(nil) || result.StderrDigest != carrier.Digest(nil) || !result.PipeDrainUncertain || result.StdoutDrainComplete || result.StderrDrainComplete || result.CleanupUncertain {
		t.Fatalf("empty observed streams were confused with complete pipe drains: %+v", result)
	}
}

func runPipeFailureProbe() pipeFailureProbe {
	root, err := os.Getwd()
	if err != nil {
		return pipeFailureProbe{SetupFailure: err.Error()}
	}
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return pipeFailureProbe{SetupFailure: err.Error()}
	}
	limit.Cur = min(limit.Cur, 64)
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return pipeFailureProbe{SetupFailure: err.Error()}
	}
	files := []*os.File{}
	for {
		file, err := os.Open(os.DevNull)
		if err != nil {
			break
		}
		files = append(files, file)
	}
	result, runErr := Run(context.Background(), Request{
		Root: root, Command: []string{"/bin/true"}, Environment: map[string]string{},
		Timeout: time.Second, MaxOutputBytes: 1024,
	})
	for _, file := range files {
		_ = file.Close()
	}
	if runErr == nil {
		return pipeFailureProbe{Result: result, HeldFDs: len(files), SetupFailure: "os.Pipe unexpectedly succeeded under EMFILE"}
	}
	return pipeFailureProbe{Result: result, Error: runErr.Error(), IsEMFILE: errors.Is(runErr, syscall.EMFILE), HeldFDs: len(files)}
}
