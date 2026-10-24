//go:build !windows

package main

func getExactAncestorAgy(startPid int) (int, int64) {
	return 0, 0
}

func hasActiveWorker(agyPid, currentPid int, targetCmd string) bool {
	return false
}

func getConversationTitle(dbPath, convID string) string {
	return ""
}
