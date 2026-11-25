package main

import "fmt"

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type animationState struct {
	Animate      bool
	SettingsPath string // Empty for CLI-only/custom-config invocations.
	Prefix       string
	Title        string
	StaticTitle  string
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
