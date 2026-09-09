// state.go: per-topic last-message timestamps, persisted as JSON next to the
// executable. On startup the stream resumes from the oldest recorded
// timestamp and already-seen messages are dropped.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const stateFileName = "ntfy-tray-state.json"

var (
	stateMu   sync.Mutex
	stateSeen = map[string]int64{}
	statePath string
	// startedAt is the fallback watermark for topics with no recorded state:
	// only messages newer than program start are shown for them.
	startedAt = time.Now().Unix()
)

type stateFile struct {
	Topics map[string]int64 `json:"topics"`
}

func initState() {
	statePath = filepath.Join(exeDir(), stateFileName)

	b, err := os.ReadFile(statePath)
	if err != nil {
		return // first run: no state file yet
	}
	var f stateFile
	if err := json.Unmarshal(b, &f); err == nil && f.Topics != nil {
		stateSeen = f.Topics
		log.Printf("loaded state for %d topic(s) from %s", len(stateSeen), statePath)
	} else {
		log.Printf("ignoring unreadable state file %s: %v", statePath, err)
	}
}

// lastSeen returns the watermark for a topic (startup time if unknown).
func lastSeen(topic string) int64 {
	stateMu.Lock()
	defer stateMu.Unlock()
	if t, ok := stateSeen[topic]; ok {
		return t
	}
	return startedAt
}

// ensureTopics records a startup watermark for every subscribed topic that
// has no state yet. Without this, a kill before the first notification would
// leave the topic stateless, and the next run would treat it as new and skip
// messages published while the program was offline.
func ensureTopics(topics []string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	changed := false
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := stateSeen[t]; !ok {
			stateSeen[t] = startedAt
			changed = true
		}
	}
	if changed {
		saveLocked()
		log.Printf("recorded startup watermark for new topic(s)")
	}
}

// markSeen records a message timestamp and persists the state file.
func markSeen(topic string, ts int64) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if ts <= stateSeen[topic] {
		return
	}
	stateSeen[topic] = ts
	saveLocked()
}

// minStateTime returns the oldest watermark across all topics, used as the
// stream's `since` parameter so per-topic dedupe can run client-side.
func minStateTime() (int64, bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	var min int64
	ok := false
	for _, t := range stateSeen {
		if !ok || t < min {
			min, ok = t, true
		}
	}
	return min, ok
}

// saveLocked writes atomically via tmp+rename. Caller holds stateMu.
func saveLocked() {
	b, err := json.MarshalIndent(stateFile{Topics: stateSeen}, "", "  ")
	if err != nil {
		return
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("state save failed: %v", err)
		return
	}
	if err := os.Rename(tmp, statePath); err != nil {
		log.Printf("state rename failed: %v", err)
	}
}
