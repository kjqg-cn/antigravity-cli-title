package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Opt-in, bounded metadata only. Never include prompts, commands, titles or paths.
type stateTrace struct {
	Time            string `json:"time"`
	Source          string `json:"source"`
	AgentState      string `json:"agent_state,omitempty"`
	Reason          string `json:"reason"`
	ActiveTasks     int    `json:"active_tasks,omitempty"`
	ActiveSubagents int    `json:"active_subagents,omitempty"`
	Animate         bool   `json:"animate"`
	Error           string `json:"error,omitempty"`
}

func appendStateTrace(homeDir string, pid int, ticks int64, entry stateTrace) {
	if homeDir == "" || pid <= 0 || ticks <= 0 {
		return
	}
	entry.Time = time.Now().UTC().Format(time.RFC3339Nano)
	path := filepath.Join(homeDir, ".gemini", "antigravity-cli-title", "diagnostic_"+animationKey(pid, ticks)+".jsonl")
	b, err := json.Marshal(entry)
	if err != nil || len(b) > 4096 {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size()+int64(len(b))+1 <= 1<<20 {
		_, _ = f.Write(append(b, '\n'))
	}
}

func recordWorkerTrace(homeDir string, pid int, ticks int64, state animationState, err error) {
	if !state.Trace {
		return
	}
	entry := stateTrace{Source: "worker", Reason: state.Reason, Animate: state.Animate}
	if err != nil {
		entry.Error = err.Error()
	}
	appendStateTrace(homeDir, pid, ticks, entry)
}
