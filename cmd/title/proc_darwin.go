//go:build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type darwinProcNode struct {
	pid  int
	ppid int
	comm string
}

func getDarwinProcessSnapshot() (map[int]darwinProcNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[int]darwinProcNode)
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) >= 3 {
			pid, _ := strconv.Atoi(f[0])
			ppid, _ := strconv.Atoi(f[1])
			comm := strings.ToLower(filepath.Base(f[2]))
			if pid > 0 {
				result[pid] = darwinProcNode{
					pid:  pid,
					ppid: ppid,
					comm: comm,
				}
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
		if pNode.comm == "agy" || strings.HasPrefix(pNode.comm, "agy") {
			// Query start time for ticks
			ticks := int64(pNode.pid)
			out, err := exec.Command("ps", "-p", strconv.Itoa(pNode.pid), "-o", "lstart=").Output()
			if err == nil {
				if t, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out))); err == nil {
					ticks = t.UnixNano()
				}
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

	cleanedTarget := strings.ToLower(strings.TrimSpace(targetCmd))
	var targetTokens []string
	if cleanedTarget != "" {
		fields := strings.FieldsFunc(cleanedTarget, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '"' || r == '\'' || r == ';'
		})
		for _, f := range fields {
			f = strings.Trim(f, "\"',;")
			if f == "bash" || f == "sh" || f == "zsh" || f == "-c" || f == "-e" {
				continue
			}
			if len(f) >= 2 {
				targetTokens = append(targetTokens, f)
			}
		}
	}

	hasAnyValidDescendant := false
	for childPid := range descendants {
		if selfAncestors[childPid] {
			continue
		}
		node := procMap[childPid]
		comm := node.comm

		out, _ := exec.Command("ps", "-p", strconv.Itoa(childPid), "-o", "command=").Output()
		cmdline := strings.ToLower(strings.TrimSpace(string(out)))

		if strings.Contains(cmdline, "statusline") || strings.Contains(cmdline, "title") {
			continue
		}

		hasAnyValidDescendant = true

		if len(targetTokens) == 0 {
			return true
		}

		if cmdline != "" {
			if strings.Contains(cmdline, cleanedTarget) {
				return true
			}
			for _, tok := range targetTokens {
				if strings.Contains(cmdline, tok) {
					return true
				}
			}
		}

		for _, tok := range targetTokens {
			if comm == tok {
				return true
			}
		}
	}

	return hasAnyValidDescendant
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
	cmd := exec.CommandContext(ctx, sqlitePath, "file:"+filepath.ToSlash(dbPath)+"?mode=ro", sql)
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
