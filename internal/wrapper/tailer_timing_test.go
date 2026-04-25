package wrapper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleAssistantLine returns one JSONL assistant line with usage tokens.
func sampleAssistantLine(inTok, outTok int, model string) string {
	return fmt.Sprintf(
		`{"type":"assistant","message":{"model":%q,"role":"assistant","usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n",
		model, inTok, outTok,
	)
}

func writeFixture(t *testing.T, path, line string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func appendFixture(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatalf("append fixture: %v", err)
	}
}

// newTempSessionSnapshot creates a SessionSnapshot whose Dir points at t.TempDir().
// Caller will later drop session-*.jsonl files into that dir to simulate Claude.
func newTempSessionSnapshot(t *testing.T) (*SessionSnapshot, string) {
	t.Helper()
	dir := t.TempDir()
	return &SessionSnapshot{Files: map[string]time.Time{}, Dir: dir}, dir
}

// TestTailer_SessionAppearsAfterExit reproduces the P1 bug:
// child exits, then session JSONL appears ~1s later.
// Tailer must wait inside PostExitTimeout and capture tokens.
func TestTailer_SessionAppearsAfterExit(t *testing.T) {
	snap, dir := newTempSessionSnapshot(t)
	sessionPath := filepath.Join(dir, "delayed-session.jsonl")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel() // child exit
		time.Sleep(1 * time.Second)
		writeFixture(t, sessionPath, sampleAssistantLine(1000, 500, "claude-sonnet-4-6"))
	}()

	start := time.Now()
	usage := TailSessionLogWithTimeout(ctx, dir, snap, 3*time.Second)
	elapsed := time.Since(start)

	if usage.Input != 1000 || usage.Output != 500 {
		t.Errorf("expected Input=1000 Output=500, got Input=%d Output=%d", usage.Input, usage.Output)
	}
	if elapsed > 2500*time.Millisecond {
		t.Errorf("post-exit wait too long: %v (expected ~1.1-2s)", elapsed)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("returned too early: %v (session needs ~1s to appear)", elapsed)
	}
}

// TestTailer_TimeoutIfSessionNeverAppears ensures tailer does not hang forever.
func TestTailer_TimeoutIfSessionNeverAppears(t *testing.T) {
	snap, dir := newTempSessionSnapshot(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled → straight into post-exit phase

	start := time.Now()
	usage := TailSessionLogWithTimeout(ctx, dir, snap, 500*time.Millisecond)
	elapsed := time.Since(start)

	if usage.Input != 0 {
		t.Errorf("expected empty usage on timeout, got %+v", usage)
	}
	if elapsed > 900*time.Millisecond {
		t.Errorf("did not respect timeout: %v", elapsed)
	}
}

// TestTailer_NoWaitFlag verifies postExitTimeout=0 skips the post-exit wait.
func TestTailer_NoWaitFlag(t *testing.T) {
	snap, dir := newTempSessionSnapshot(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	usage := TailSessionLogWithTimeout(ctx, dir, snap, 0)
	elapsed := time.Since(start)

	if usage.Input != 0 {
		t.Errorf("expected empty usage with no-wait, got %+v", usage)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("no-wait returned slow: %v", elapsed)
	}
}

// TestTailer_PartialSessionThenComplete verifies incremental read —
// partial tokens before exit, more tokens after exit, final usage is merged.
func TestTailer_PartialSessionThenComplete(t *testing.T) {
	snap, dir := newTempSessionSnapshot(t)
	sessionPath := filepath.Join(dir, "partial-session.jsonl")

	writeFixture(t, sessionPath, sampleAssistantLine(500, 200, "claude-sonnet-4-6"))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
		time.Sleep(500 * time.Millisecond)
		appendFixture(t, sessionPath, sampleAssistantLine(300, 100, "claude-sonnet-4-6"))
	}()

	usage := TailSessionLogWithTimeout(ctx, dir, snap, 2*time.Second)

	// mergeUsage sums all tokens across assistant lines.
	if usage.Input != 800 {
		t.Errorf("expected merged Input=800, got %d", usage.Input)
	}
	if usage.Output != 300 {
		t.Errorf("expected merged Output=300, got %d", usage.Output)
	}
	if usage.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model set, got %q", usage.Model)
	}
}

// TestTailer_BackwardCompat_DefaultEntryPoint ensures old API still works
// (TailSessionLog is preserved, delegates to WithTimeout).
func TestTailer_BackwardCompat_DefaultEntryPoint(t *testing.T) {
	snap, dir := newTempSessionSnapshot(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return quickly with empty usage (no session was ever written).
	start := time.Now()
	usage := TailSessionLog(ctx, dir, snap)
	elapsed := time.Since(start)

	if usage.Input != 0 {
		t.Errorf("expected empty usage, got %+v", usage)
	}
	// Default timeout is 10s but nothing appears → it'll wait full 10s.
	// We accept up to 11s here but flag if something's grossly wrong.
	if elapsed > 11*time.Second {
		t.Errorf("default entry hung: %v", elapsed)
	}
}
