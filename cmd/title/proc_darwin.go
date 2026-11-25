//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type darwinProcNode struct {
	pid     int
	ppid    int
	comm    string
	ticks   int64
	cmdline string
}

func getDarwinProcessSnapshot() (map[int]darwinProcNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,comm=,lstart=,command=")
	var origLCAll string
	hasLCAll := false
	hasLCCType := false
	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "LC_ALL=") {
			hasLCAll = true
			origLCAll = strings.TrimPrefix(e, "LC_ALL=")
			continue
		}
		if strings.HasPrefix(e, "LC_TIME=") {
			continue
		}
		if strings.HasPrefix(e, "LC_CTYPE=") {
			hasLCCType = true
		}
		env = append(env, e)
	}
	if hasLCAll && !hasLCCType && origLCAll != "" {
		env = append(env, "LC_CTYPE="+origLCAll)
	}
	env = append(env, "LC_TIME=C")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[int]darwinProcNode)
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if pid, ppid, comm, ticks, cmdline, ok := parseDarwinPsLine(l); ok {
			result[pid] = darwinProcNode{
				pid:     pid,
				ppid:    ppid,
				comm:    strings.ToLower(comm),
				ticks:   ticks,
				cmdline: strings.ToLower(cmdline),
			}
		}
	}
	return result, nil
}

func getExactAncestorAgy(startPid int) (int, int64) {
	procMap, err := getDarwinProcessSnapshot()
	if err != nil {
		return 0, 0
	}

	cur := startPid
	for i := 0; i < 6; i++ {
		node, ok := procMap[cur]
		if !ok || node.ppid <= 0 {
			break
		}
		cur = node.ppid
		pNode, ok := procMap[cur]
		if !ok {
			break
		}
		baseComm := filepath.Base(pNode.comm)
		if pNode.comm == "agy" || baseComm == "agy" {
			ticks := pNode.ticks
			if ticks == 0 {
				ticks = int64(pNode.pid)
			}
			return pNode.pid, ticks
		}
	}
	return 0, 0
}

func hasActiveWorker(agyPid, currentPid int, targetCmd string) bool {
	if agyPid <= 0 {
		return false
	}

	procMap, err := getDarwinProcessSnapshot()
	if err != nil {
		return false
	}

	selfAncestors := make(map[int]bool)
	cur := currentPid
	for i := 0; i < 6; i++ {
		if cur <= 0 || cur == agyPid {
			break
		}
		selfAncestors[cur] = true
		node, ok := procMap[cur]
		if !ok || node.ppid <= 0 {
			break
		}
		cur = node.ppid
	}

	descendants := make(map[int]bool)
	visited := make(map[int]bool)
	type queueItem struct {
		pid   int
		depth int
	}
	queue := []queueItem{{pid: agyPid, depth: 0}}
	visited[agyPid] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= 5 {
			continue
		}

		for pid, node := range procMap {
			if node.ppid == item.pid && !visited[pid] {
				visited[pid] = true
				descendants[pid] = true
				queue = append(queue, queueItem{pid: pid, depth: item.depth + 1})
			}
		}
	}

	pat := parseWorkerTarget(targetCmd)
	if !pat.valid {
		return false
	}

	for childPid := range descendants {
		if selfAncestors[childPid] {
			continue
		}
		node := procMap[childPid]
		comm := node.comm
		baseComm := filepath.Base(comm)

		if comm == "title" || comm == "statusline" ||
			baseComm == "title" || baseComm == "statusline" ||
			strings.HasSuffix(comm, "/title") || strings.HasSuffix(comm, "/statusline") {
			continue
		}

		cmdline := node.cmdline
		if matchWorkerProcess(comm, cmdline, pat) {
			return true
		}
	}

	return false
}

func getConversationTitle(dbPath, convID string) string {
	if convID == "" || dbPath == "" {
		return ""
	}
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		sqlitePath = "/usr/bin/sqlite3"
	}
	sql := fmt.Sprintf("SELECT title FROM conversation_summaries WHERE conversation_id = '%s' LIMIT 1", strings.ReplaceAll(convID, "'", "''"))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, sqlitePath, "-init", "/dev/null", "-noheader", "-readonly", dbPath, sql)
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
