package util

import (
	"strings"
	"testing"
	"time"
)

func TestComputeHash(t *testing.T) {
	input := HashInput{
		PrevHash:   "",
		Actor:      "phuc",
		Action:     "run_started",
		EntityType: "run",
		EntityID:   "run-123",
		Details:    `{"command":"claude code"}`,
		Timestamp:  "2026-04-18T10:00:00.000000000Z",
	}

	hash1 := ComputeHash(input)
	hash2 := ComputeHash(input)

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}

	if len(hash1) != 64 {
		t.Errorf("expected 64 char hex hash, got %d", len(hash1))
	}
}

func TestComputeHashChain(t *testing.T) {
	ts := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)

	hash1 := ComputeAuditHash("", "phuc", "run_started", "run", "run-1", "{}", ts)
	hash2 := ComputeAuditHash(hash1, "phuc", "run_completed", "run", "run-1", "{}", ts.Add(time.Hour))

	if hash1 == hash2 {
		t.Error("different inputs should produce different hashes")
	}

	hash1Again := ComputeAuditHash("", "phuc", "run_started", "run", "run-1", "{}", ts)
	if hash1 != hash1Again {
		t.Error("hash chain should be deterministic")
	}
}

func TestGenerateRunID(t *testing.T) {
	id1 := GenerateRunID()
	id2 := GenerateRunID()

	if len(id1) != 16 {
		t.Errorf("expected 16 char run ID, got %d", len(id1))
	}

	if id1 == id2 {
		t.Error("run IDs should be unique")
	}
}

func TestGenerateWorkstationID(t *testing.T) {
	id := GenerateWorkstationID()

	if !strings.HasPrefix(id, "ws-") {
		t.Errorf("workstation ID should start with ws-, got %s", id)
	}

	if len(id) != 11 {
		t.Errorf("expected 11 char workstation ID, got %d", len(id))
	}
}
