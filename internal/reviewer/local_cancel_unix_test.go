//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package reviewer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunLocalCancellationStopsCodexProcessGroup(t *testing.T) {
	dir := initLocalGitRepo(t)
	binDir := t.TempDir()
	stateDir := t.TempDir()
	pidFile := filepath.Join(stateDir, "child.pid")
	tickFile := filepath.Join(stateDir, "ticks.txt")
	writeLocalExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
trap '' INT
(
  trap '' INT
  while :; do
    printf 'tick\n' >> "$TICK_FILE"
    sleep 0.1
  done
) &
printf '%s\n' "$!" > "$PID_FILE"
wait "$!"
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PID_FILE", pidFile)
	t.Setenv("TICK_FILE", tickFile)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunLocal(ctx, LocalOptions{
			Dir:    dir,
			Base:   "origin/main",
			Report: "codex-review/test.md",
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
		})
	}()

	waitForFile(t, pidFile, 2*time.Second)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("RunLocal() error = nil after cancellation")
		}
		if !strings.Contains(err.Error(), "run codex review") {
			t.Fatalf("RunLocal() error = %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("RunLocal() did not return after cancellation")
	}

	sizeAfterReturn := fileSize(t, tickFile)
	time.Sleep(400 * time.Millisecond)
	if got := fileSize(t, tickFile); got != sizeAfterReturn {
		t.Fatalf("codex child kept writing after cancellation: size before=%d after=%d", sizeAfterReturn, got)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	return info.Size()
}
