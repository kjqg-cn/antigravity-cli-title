//go:build !windows

package main

func callbackSettingsPath(defaultPath string, agyPid int) string { return defaultPath }

func updateAnimation(homeDir string, pid int, ticks int64, state animationState) error { return nil }
func runAnimationWorker(args []string)                                                 {}
