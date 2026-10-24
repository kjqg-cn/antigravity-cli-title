//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type linuxProcInfo struct {
	pid       int
	comm      string
	ppid      int
	starttime int64
}

func readLinuxProcStat(pid int) (*linuxProcInfo, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}
	line := strings.TrimSpace(string(data))
	firstParen := strings.Index(line, "(")
	lastParen := strings.LastIndex(line, ")")
	if firstParen == -1 || lastParen == -1 || lastParen <= firstParen {
		return nil, fmt.Errorf("invalid stat line")
	}

	comm := line[firstParen+1 : lastParen]
	after := strings.Fields(line[lastParen+1:])
	if len(after) < 20 {
		return nil, fmt.Errorf("stat fields too short")
	}

	ppid, _ := strconv.Atoi(after[1])       // field 4 is index 1
	starttime, _ := strconv.ParseInt(after[19], 10, 64) // field 22 is index 19

	return &linuxProcInfo{
		pid:       pid,
		comm:      comm,
		ppid:      ppid,
		starttime: starttime,
	}, nil
}

func getExactAncestorAgy(startPid int) (int, int64) {
	cur := startPid
	for i := 0; i < 6; i++ {
		info, err := readLinuxProcStat(cur)
		if err != nil || info.ppid <= 0 {
			break
		}
		cur = info.ppid

		pInfo, err := readLinuxProcStat(cur)
		if err != nil {
			break
		}
		comm := strings.ToLower(pInfo.comm)
		if comm == "agy" || strings.HasPrefix(comm, "agy") {
			return pInfo.pid, pInfo.starttime
		}
	}
	return 0, 0
}

func hasActiveWorker(agyPid, currentPid int, targetCmd string) bool {
	if agyPid <= 0 {
		return false
	}

	// 1. Identify self ancestors to avoid false positives
	selfAncestors := make(map[int]bool)
	cur := currentPid
	for i := 0; i < 6; i++ {
		if cur <= 0 || cur == agyPid {
			break
		}
		selfAncestors[cur] = true
		info, err := readLinuxProcStat(cur)
		if err != nil || info.ppid <= 0 {
			break
		}
		cur = info.ppid
	}

	// 2. Scan all process entries in /proc
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	type procNode struct {
		pid  int
		ppid int
		comm string
	}
	procMap := make(map[int]procNode)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if info, err := readLinuxProcStat(pid); err == nil {
			procMap[pid] = procNode{
				pid:  pid,
				ppid: info.ppid,
				comm: strings.ToLower(info.comm),
			}
		}
	}

	// 3. BFS traversal to collect all descendants of agyPid up to 5 levels
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

	// 4. Tokenize target command
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

		cmdBytes, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", childPid))
		cmdline := strings.ToLower(strings.ReplaceAll(string(cmdBytes), "\x00", " "))

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
		return ""
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
