package wrapper

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SessionSnapshot struct {
	Files map[string]time.Time
	Dir   string
}

func SnapshotSessionDir(cwd string) *SessionSnapshot {
	claudeDir := getClaudeProjectDir(cwd)
	if claudeDir == "" {
		return &SessionSnapshot{Files: make(map[string]time.Time)}
	}

	snapshot := &SessionSnapshot{
		Files: make(map[string]time.Time),
		Dir:   claudeDir,
	}

	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return snapshot
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		snapshot.Files[entry.Name()] = info.ModTime()
	}

	return snapshot
}

func DetectSessionID(cwd string, before *SessionSnapshot) string {
	if before == nil || before.Dir == "" {
		return ""
	}

	entries, err := os.ReadDir(before.Dir)
	if err != nil {
		return ""
	}

	var newestFile string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		prevTime, existed := before.Files[entry.Name()]
		if !existed || info.ModTime().After(prevTime) {
			if info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
				newestFile = entry.Name()
			}
		}
	}

	if newestFile == "" {
		return ""
	}

	return strings.TrimSuffix(newestFile, ".jsonl")
}

func getClaudeProjectDir(cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	cwdHash := hashCWD(cwd)
	projectDir := filepath.Join(home, ".claude", "projects", cwdHash)

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return ""
	}

	return projectDir
}

func hashCWD(cwd string) string {
	h := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(h[:])[:16]
}

func GetSessionLogPath(cwd string, before *SessionSnapshot) string {
	sessionID := DetectSessionID(cwd, before)
	if sessionID == "" || before == nil || before.Dir == "" {
		return ""
	}
	return filepath.Join(before.Dir, sessionID+".jsonl")
}
