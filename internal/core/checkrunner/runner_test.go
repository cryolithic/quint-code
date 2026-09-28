package checkrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunnerHelperProcess(t *testing.T) {
	index := slices.Index(os.Args, "--")
	if index < 0 {
		return
	}
	if index+1 >= len(os.Args) {
		os.Exit(2)
	}
	switch os.Args[index+1] {
	case "pass":
		_, _ = os.Stdout.WriteString("pass\n")
		_, _ = os.Stderr.WriteString("warning\n")
	case "fail":
		_, _ = os.Stdout.WriteString("assertion failed\n")
		os.Exit(7)
	case "large":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("ab"), 64_000))
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("z"), 96_000))
	case "combined":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("a"), 700))
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("b"), 700))
	case "sleep":
		time.Sleep(10 * time.Second)
	case "spawn-child", "spawn-child-closed", "spawn-child-fail", "spawn-child-stderr":
		child := exec.Command(os.Args[0], "-test.run=^TestRunnerHelperProcess$", "--", "child", os.Args[index+2], os.Args[index+3])
		if os.Args[index+1] != "spawn-child-closed" {
			child.Stderr = os.Stderr
		}
		if os.Args[index+1] == "spawn-child" || os.Args[index+1] == "spawn-child-fail" {
			child.Stdout = os.Stdout
		}
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		_, _ = os.Stdout.WriteString("spawned\n")
		if os.Args[index+1] == "spawn-child-fail" {
			os.Exit(7)
		}
	case "child":
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
			if _, err := os.Stat(os.Args[index+3]); err == nil {
				_ = os.WriteFile(os.Args[index+2], []byte("child-survived"), 0o600)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "environment":
		marker, _ := os.ReadFile("marker")
		_, _ = fmt.Fprintf(os.Stdout, "%s|%s|%s|%s|%s", marker, os.Getenv("RUNNER_SECRET"), os.Getenv("GOPROXY"), os.Getenv("GOENV"), os.Getenv("CGO_ENABLED"))
	default:
		os.Exit(3)
	}
	os.Exit(0)
}

func testRequest(t *testing.T, mode string) Request {
	t.Helper()
	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		Root:           t.TempDir(),
		Command:        []string{binary, "-test.run=^TestRunnerHelperProcess$", "--", mode},
		Environment:    map[string]string{"GOOS": "linux"},
		Timeout:        3 * time.Second,
		MaxOutputBytes: 1024,
	}
}

func TestRunPreservesExactExitAndFullStreamDigests(t *testing.T) {
	passed := testRequest(t, "pass")
	pass, err := Run(context.Background(), passed)
	if err != nil {
		t.Fatal(err)
	}
	if !pass.Started || pass.Failure != "" || pass.ExitCode == nil || *pass.ExitCode != 0 || pass.OutputExceeded || pass.PipeDrainUncertain || !pass.StdoutDrainComplete || !pass.StderrDrainComplete {
		t.Fatalf("pass process facts: %+v", pass)
	}
	if string(pass.Stdout) != "pass\n" || string(pass.Stderr) != "warning\n" || pass.StdoutBytes != 5 || pass.StderrBytes != 8 {
		t.Fatalf("pass streams: %+v", pass)
	}
	if pass.StdoutDigest != testDigest([]byte("pass\n")) || pass.StderrDigest != testDigest([]byte("warning\n")) {
		t.Fatalf("pass digests: %+v", pass)
	}
	failed := testRequest(t, "fail")
	failure, err := Run(context.Background(), failed)
	if err != nil {
		t.Fatal(err)
	}
	if !failure.Started || failure.Failure != "" || failure.ExitCode == nil || *failure.ExitCode != 7 || failure.PipeDrainUncertain || !failure.StdoutDrainComplete || !failure.StderrDrainComplete {
		t.Fatalf("nonzero exit must remain a process result: %+v", failure)
	}
}

func TestRunDrainsLargeStreamsPastRetainedPrefixes(t *testing.T) {
	request := testRequest(t, "large")
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	stdout := bytes.Repeat([]byte("ab"), 64_000)
	stderr := bytes.Repeat([]byte("z"), 96_000)
	if !result.Started || result.ExitCode == nil || *result.ExitCode != 0 || result.Failure != "" || !result.OutputExceeded {
		t.Fatalf("large process facts: %+v", result)
	}
	if result.StdoutBytes != int64(len(stdout)) || result.StderrBytes != int64(len(stderr)) || len(result.Stdout)+len(result.Stderr) != 1024 {
		t.Fatalf("bounded prefixes and full lengths: %+v", result)
	}
	if !bytes.Equal(result.Stdout, stdout[:len(result.Stdout)]) || !bytes.Equal(result.Stderr, stderr[:len(result.Stderr)]) || result.StdoutDigest != testDigest(stdout) || result.StderrDigest != testDigest(stderr) {
		t.Fatal("large stream prefix or full digest changed")
	}
}

func TestRunEnforcesAggregateLimitAcrossBothStreams(t *testing.T) {
	request := testRequest(t, "combined")
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputExceeded || result.StdoutBytes != 700 || result.StderrBytes != 700 || len(result.Stdout)+len(result.Stderr) != 1024 {
		t.Fatalf("combined output must exceed one shared budget: %+v", result)
	}
	if result.StdoutDigest != testDigest(bytes.Repeat([]byte("a"), 700)) || result.StderrDigest != testDigest(bytes.Repeat([]byte("b"), 700)) {
		t.Fatal("aggregate limit changed full-stream digests")
	}
}

func TestRunDistinguishesTimeoutAndStartFailure(t *testing.T) {
	timeoutRequest := testRequest(t, "sleep")
	timeoutRequest.Timeout = 500 * time.Millisecond
	timedOut, err := Run(context.Background(), timeoutRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !timedOut.Started || timedOut.Failure != "timeout" || timedOut.ExitCode != nil {
		t.Fatalf("timeout process facts: %+v", timedOut)
	}
	missing := testRequest(t, "pass")
	missing.Command = []string{filepath.Join(missing.Root, "missing-runner")}
	notStarted, err := Run(context.Background(), missing)
	if err != nil {
		t.Fatal(err)
	}
	if notStarted.Started || notStarted.Failure != "start_failed" || notStarted.ExitCode != nil {
		t.Fatalf("start failure process facts: %+v", notStarted)
	}
}

func TestRunUsesProjectCwdAndDropsUnlistedHostEnvironment(t *testing.T) {
	t.Setenv("RUNNER_SECRET", "must-not-reach-child")
	request := testRequest(t, "environment")
	if err := os.WriteFile(filepath.Join(request.Root, "marker"), []byte("project"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || strings.TrimSpace(string(result.Stdout)) != "project||off|off|0" {
		t.Fatalf("cwd or sanitized child env changed: %+v", result)
	}
	if _, exposed := result.Environment["RUNNER_SECRET"]; exposed {
		t.Fatal("ambient variable exposed in returned environment")
	}
	if _, exposed := result.Environment["HOME"]; exposed {
		t.Fatal("host home path exposed in returned environment")
	}
}

func testDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum)
}
