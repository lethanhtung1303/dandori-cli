package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type HashInput struct {
	PrevHash   string `json:"prev_hash"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Details    string `json:"details"`
	Timestamp  string `json:"timestamp"`
}

func ComputeHash(input HashInput) string {
	data, _ := json.Marshal(input)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func ComputeAuditHash(prevHash, actor, action, entityType, entityID, details string, ts time.Time) string {
	input := HashInput{
		PrevHash:   prevHash,
		Actor:      actor,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    details,
		Timestamp:  ts.UTC().Format(time.RFC3339Nano),
	}
	return ComputeHash(input)
}

func GenerateRunID() string {
	now := time.Now()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d-%d", now.UnixNano(), now.UnixMicro())))
	return hex.EncodeToString(hash[:])[:16]
}

func GenerateWorkstationID() string {
	now := time.Now()
	hash := sha256.Sum256([]byte(fmt.Sprintf("ws-%d", now.UnixNano())))
	return "ws-" + hex.EncodeToString(hash[:])[:8]
}
