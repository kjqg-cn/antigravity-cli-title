package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"
)

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var errSupersededCallback = errors.New("superseded callback")

func animationPath(homeDir, key string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity-cli-title", "animation_"+key+".json")
}

type animationState struct {
	Enabled      bool // Animation mode enabled, even when this callback is static.
	Animate      bool
	SettingsPath string // Empty for CLI-only/custom-config invocations.
	Prefix       string
	Title        string
	StaticTitle  string
	UpdatedAt    int64
	Reason       string
	Trace        bool
	Completion   *backgroundCompletion `json:",omitempty"`
}

type backgroundCompletion struct {
	ConversationID string
	TranscriptPath string
	TaskTimeout    time.Duration
	DoneTitle      string
}

func (s animationState) frameAt(now time.Time) string {
	return s.frame(int((now.UnixMilli() / 100) % int64(len(spinnerFrames))))
}

func (s animationState) callbackTitle(publishErr error, now time.Time) string {
	if runtime.GOOS == "windows" && publishErr == nil && s.Animate {
		return s.frameAt(now)
	}
	return cleanTitleText(s.StaticTitle)
}

// Only an explicit idle event with background tasks may decay. Foreground work
// and active subagents must never complete merely because telemetry is silent.
func (s animationState) refreshCompletion(homeDir string) animationState {
	if !s.Animate || s.Reason != "background_tasks" || s.Completion == nil {
		return s
	}
	c := s.Completion
	if c.TaskTimeout <= 0 {
		return s
	}
	active, known := taskActivity(homeDir, c.ConversationID, c.TranscriptPath, c.TaskTimeout)
	if known && !active {
		s.Animate = false
		s.StaticTitle = c.DoneTitle
		s.Reason = "background_timeout"
	}
	return s
}

func (s animationState) frame(index int) string {
	if !s.Animate {
		return cleanTitleText(s.StaticTitle)
	}
	return cleanTitleText(s.Prefix + spinnerFrames[index%len(spinnerFrames)] + " " + s.Title)
}

func animationKey(pid int, ticks int64) string { return fmt.Sprintf("%d_%d", pid, ticks) }

func animationEnabled(cfg RuntimeConfig) bool {
	return cfg.Animate && !cfg.UseTextStatus && cleanSymbol(cfg.CustomWorking) == ""
}
