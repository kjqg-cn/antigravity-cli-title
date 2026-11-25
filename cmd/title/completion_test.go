package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCallbackSpinnerHasNoGear(t *testing.T) {
	s := animationState{Animate: true, Prefix: "agy: ", Title: "Task", StaticTitle: "agy: ⚙️ Task"}
	now := time.Unix(0, 0).Add(235 * time.Millisecond)
	if runtime.GOOS == "windows" {
		if got := s.callbackTitle(nil, now); got != s.frameAt(now) || strings.Contains(got, "⚙") {
			t.Fatal(got)
		}
	} else if s.callbackTitle(nil, now) != s.StaticTitle {
		t.Fatal("non-Windows static fallback changed")
	}
	if s.callbackTitle(errors.New("publish failed"), now) != s.StaticTitle {
		t.Fatal("failed animation must retain static title")
	}
}

func TestBackgroundCompletionOnlyAfterKnownIdle(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks")
	if err := os.Mkdir(tasks, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(tasks, "task.log")
	if err := os.WriteFile(log, []byte("activity"), 0600); err != nil {
		t.Fatal(err)
	}
	completion := &backgroundCompletion{ConversationID: "session", TranscriptPath: filepath.Join(dir, "logs", "transcript.jsonl"), TaskTimeout: time.Second, DoneTitle: "✅ Task"}
	s := animationState{Animate: true, Reason: "background_tasks", Completion: completion}
	if got := s.refreshCompletion(""); !got.Animate {
		t.Fatal("recent task completed early")
	}
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(log, old, old); err != nil {
		t.Fatal(err)
	}
	if got := s.refreshCompletion(""); got.Animate || got.StaticTitle != "✅ Task" || got.Reason != "background_timeout" {
		t.Fatal(got)
	}
	for _, reason := range []string{"foreground", "subagents"} {
		active := animationState{Animate: true, Reason: reason}
		if !active.refreshCompletion("").Animate {
			t.Fatal("silence completed active work", reason)
		}
	}
	missing := s
	cp := *completion
	cp.TranscriptPath = filepath.Join(dir, "unknown", "logs", "transcript.jsonl")
	missing.Completion = &cp
	if !missing.refreshCompletion("").Animate {
		t.Fatal("missing logs are not proof of completion")
	}
}

func TestStateTraceMetadata(t *testing.T) {
	home := t.TempDir()
	appendStateTrace(home, 123, 456, stateTrace{Source: "callback", AgentState: "idle", Reason: "background_tasks", ActiveTasks: 2, Animate: true})
	p := filepath.Join(home, ".gemini", "antigravity-cli-title", "diagnostic_123_456.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]interface{}
	if json.Unmarshal(b, &entry) != nil {
		t.Fatal(string(b))
	}
	for _, key := range []string{"title", "command", "content", "cwd", "transcript_path"} {
		if _, ok := entry[key]; ok {
			t.Fatal("private data field", key)
		}
	}
	if entry["reason"] != "background_tasks" || entry["active_tasks"] != float64(2) {
		t.Fatal(entry)
	}
}

func TestCompletionReasons(t *testing.T) {
	symbols := resolveStatusSymbols(true, "", "", "", "")
	for _, tc := range []struct {
		state            string
		tasks, subagents bool
		reason           string
	}{
		{"working", true, false, "foreground"},
		{"idle", true, false, "background_tasks"},
		{"idle", true, true, "subagents"},
		{"waiting", true, true, "waiting"},
	} {
		in := StateInput{AgentState: tc.state, HasActiveBgTasks: tc.tasks, Symbols: symbols}
		if tc.subagents {
			in.ActiveSubagents = 1
		}
		if result := resolveState(in); result.Reason != tc.reason {
			t.Fatal(tc, result)
		}
	}
}
