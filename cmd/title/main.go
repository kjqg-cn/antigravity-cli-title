package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type Subagent struct {
	Status string `json:"status"`
}

type TelemetryInput struct {
	Cwd               string      `json:"cwd"`
	ConversationID    string      `json:"conversation_id"`
	ConversationTitle string      `json:"conversation_title"`
	AgentState        string      `json:"agent_state"`
	Subagents         []Subagent  `json:"subagents"`
	TaskCount         interface{} `json:"task_count"`
	TranscriptPath    string      `json:"transcript_path"`
}

type ToolCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type PlannerResponse struct {
	Type      string     `json:"type"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

type Permissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
	Ask   []string `json:"ask"`
}

type TitleConfig struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
	Ready   string `json:"ready"`
	Working string `json:"working"`
	Waiting string `json:"waiting"`
	Done    string `json:"done"`
}

type Settings struct {
	Permissions *Permissions `json:"permissions"`
	Title       *TitleConfig `json:"title"`
}

var (
	rxMultiWs  = regexp.MustCompile(`[\r\n\t]+`)
	rxCmdRegex = regexp.MustCompile(`^command\(regex:(.*)\)$`)
	rxCmdGlob  = regexp.MustCompile(`^command\((.*)\)$`)
)

func globToRegex(glob string) (string, string) {
	escaped := regexp.QuoteMeta(glob)
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
	escaped = strings.ReplaceAll(escaped, `\?`, `.`)
	pat := "(?i)^" + escaped + "$"
	patSub := "(?i)(?:^|\\s)" + escaped + "$"
	return pat, patSub
}

func fileGlobToRegex(glob string) (string, string) {
	norm := filepath.ToSlash(glob)
	escaped := regexp.QuoteMeta(norm)
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
	escaped = strings.ReplaceAll(escaped, `\?`, `.`)
	pat := "(?i)^" + escaped + "$"
	patSub := "(?i)(?:^|/)" + escaped + "$"
	return pat, patSub
}

func unescapeCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, `\n`, "\n")
	cmd = strings.ReplaceAll(cmd, `\r`, "\r")
	cmd = strings.ReplaceAll(cmd, `\t`, "\t")
	cmd = strings.ReplaceAll(cmd, `\"`, `"`)
	return cmd
}

type stepHeader struct {
	Type string `json:"type"`
}

func parseTranscriptTail(transcriptPath string) (*PlannerResponse, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	readLen := int64(131072) // 128KB
	if readLen > size {
		readLen = size
	}
	if readLen <= 0 {
		return nil, nil
	}

	buf := make([]byte, readLen)
	_, err = f.Seek(-readLen, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	buf = buf[:n]

	lines := bytes.Split(buf, []byte("\n"))
	hasGenericAfterPlanner := false
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		if len(line) == 0 || line[0] != '{' || line[len(line)-1] != '}' {
			continue
		}

		var hdr stepHeader
		if err := json.Unmarshal(line, &hdr); err != nil {
			continue
		}

		if hdr.Type == "GENERIC" {
			hasGenericAfterPlanner = true
			continue
		}

		if hdr.Type == "PLANNER_RESPONSE" {
			if hasGenericAfterPlanner {
				return nil, nil
			}
			var pr PlannerResponse
			if err := json.Unmarshal(line, &pr); err == nil {
				return &pr, nil
			}
		}

		if hdr.Type == "USER_INPUT" {
			break
		}
	}
	return nil, nil
}

func getFirstPromptFromTranscript(transcriptPath string, maxTitleLen int) (string, bool) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", false
	}
	defer f.Close()

	buf := make([]byte, 32768)
	n, _ := f.Read(buf)
	if n <= 0 {
		return "", false
	}

	lines := bytes.Split(buf[:n], []byte("\n"))
	limit := 50
	if len(lines) < limit {
		limit = len(lines)
	}

	rxUserReq := regexp.MustCompile(`(?s)<USER_REQUEST>(.*?)</USER_REQUEST>`)

	for i := 0; i < limit; i++ {
		line := bytes.TrimSpace(lines[i])
		if bytes.Contains(line, []byte(`"type":"USER_INPUT"`)) || bytes.Contains(line, []byte(`"type": "USER_INPUT"`)) {
			var obj map[string]interface{}
			if err := json.Unmarshal(line, &obj); err == nil {
				if content, ok := obj["content"].(string); ok && content != "" {
					raw := content
					if m := rxUserReq.FindStringSubmatch(content); len(m) > 1 {
						raw = m[1]
					}
					rawLines := strings.Split(raw, "\n")
					for _, rl := range rawLines {
						rl = strings.TrimSpace(rl)
						if len(rl) > 0 {
							firstLine := strings.TrimLeft(rl, "# \t")
							runes := []rune(firstLine)
							if len(runes) > maxTitleLen {
								firstLine = string(runes[:maxTitleLen-3]) + "..."
							}
							return firstLine, true
						}
					}
				}
			}
			return "", true
		}
	}
	return "", false
}

func isInternalOrTempPath(p string) bool {
	pLower := strings.ToLower(filepath.ToSlash(p))
	if strings.Contains(pLower, "/.gemini/") || strings.Contains(pLower, "/.antigravity/") {
		return true
	}
	if strings.Contains(pLower, "/appdata/local/temp/") || strings.Contains(pLower, "/appdata/local/agy/") {
		return true
	}
	tempDir := strings.ToLower(filepath.ToSlash(os.TempDir()))
	if tempDir != "" && strings.HasPrefix(pLower, tempDir) {
		return true
	}
	return false
}

func hasRecentlyActiveTasks(homeDir, convID, transcriptPath string, timeout time.Duration) bool {
	now := time.Now()
	var candidateDirs []string
	if transcriptPath != "" {
		logsDir := filepath.Dir(transcriptPath)
		sysGenDir := filepath.Dir(logsDir)
		candidateDirs = append(candidateDirs, filepath.Join(sysGenDir, "tasks"))
	}
	if convID != "" && homeDir != "" {
		candidateDirs = append(candidateDirs,
			filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "tasks"),
			filepath.Join(homeDir, ".gemini", "antigravity", "brain", convID, ".system_generated", "tasks"),
		)
	}

	for _, dir := range candidateDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		foundLog := false
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			foundLog = true
			if info, err := e.Info(); err == nil {
				if now.Sub(info.ModTime()) <= timeout {
					return true
				}
			}
		}
		if foundLog {
			return false
		}
	}
	return false
}

func cleanStdin(b []byte) []byte {
	// UTF-8 BOM
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	// UTF-16LE BOM
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		u16 := make([]uint16, (len(b)-2)/2)
		for i := 0; i < len(u16); i++ {
			u16[i] = uint16(b[2+i*2]) | (uint16(b[3+i*2]) << 8)
		}
		return []byte(string(utf16.Decode(u16)))
	}
	// UTF-16BE BOM
	if len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff {
		u16 := make([]uint16, (len(b)-2)/2)
		for i := 0; i < len(u16); i++ {
			u16[i] = (uint16(b[2+i*2]) << 8) | uint16(b[3+i*2])
		}
		return []byte(string(utf16.Decode(u16)))
	}
	return b
}

type StatusSymbols struct {
	Ready   string
	Working string
	Waiting string
	Done    string
}

func cleanSymbol(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

func parseCommandFlags(cmdStr string) (ready, working, waiting, done string) {
	parts := strings.Fields(cmdStr)
	for i := 0; i < len(parts); i++ {
		arg := strings.ToLower(parts[i])
		switch arg {
		case "--ready", "--ready-symbol":
			if i+1 < len(parts) {
				ready = parts[i+1]
				i++
			}
		case "--working", "--working-symbol":
			if i+1 < len(parts) {
				working = parts[i+1]
				i++
			}
		case "--waiting", "--waiting-symbol":
			if i+1 < len(parts) {
				waiting = parts[i+1]
				i++
			}
		case "--done", "--done-symbol":
			if i+1 < len(parts) {
				done = parts[i+1]
				i++
			}
		}
	}
	return
}

func resolveStatusSymbols(useTextStatus bool, customReady, customWorking, customWaiting, customDone string) StatusSymbols {
	return resolveConfiguredSymbols(useTextStatus, customReady, customWorking, customWaiting, customDone, nil)
}

func resolveConfiguredSymbols(useTextStatus bool, cliReady, cliWorking, cliWaiting, cliDone string, cfg *TitleConfig) StatusSymbols {
	s := StatusSymbols{
		Ready:   "⚪",
		Working: "⚙️",
		Waiting: "⚠️",
		Done:    "✅",
	}
	if useTextStatus {
		s.Ready = "[READY]"
		s.Working = "[WORKING]"
		s.Waiting = "[WAITING]"
		s.Done = "[DONE]"
	}

	if cfg != nil {
		// 1. Hot-reload flags from settings.json "command" string
		cmdReady, cmdWorking, cmdWaiting, cmdDone := parseCommandFlags(cfg.Command)
		if r := cleanSymbol(cmdReady); r != "" {
			s.Ready = r
		}
		if w := cleanSymbol(cmdWorking); w != "" {
			s.Working = w
		}
		if wt := cleanSymbol(cmdWaiting); wt != "" {
			s.Waiting = wt
		}
		if d := cleanSymbol(cmdDone); d != "" {
			s.Done = d
		}

		// 2. Direct keys in settings.json have higher priority than "command" string flags
		if r := cleanSymbol(cfg.Ready); r != "" {
			s.Ready = r
		}
		if w := cleanSymbol(cfg.Working); w != "" {
			s.Working = w
		}
		if wt := cleanSymbol(cfg.Waiting); wt != "" {
			s.Waiting = wt
		}
		if d := cleanSymbol(cfg.Done); d != "" {
			s.Done = d
		}
	}

	// 3. Explicit CLI flags override settings.json
	if r := cleanSymbol(cliReady); r != "" {
		s.Ready = r
	}
	if w := cleanSymbol(cliWorking); w != "" {
		s.Working = w
	}
	if wt := cleanSymbol(cliWaiting); wt != "" {
		s.Waiting = wt
	}
	if d := cleanSymbol(cliDone); d != "" {
		s.Done = d
	}

	return s
}

func main() {
	showPrefix := true
	customPrefix := "agy:"
	useTextStatus := false
	maxTitleLen := 35
	taskTimeout := 60 * time.Second
	customReady := ""
	customWorking := ""
	customWaiting := ""
	customDone := ""

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(args[i])
		switch arg {
		case "--no-prefix":
			showPrefix = false
		case "--text", "--classic":
			useTextStatus = true
		case "--max-length":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil && v > 5 {
					maxTitleLen = v
					i++
				}
			}
		case "--task-timeout":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil && v > 0 {
					taskTimeout = time.Duration(v) * time.Second
					i++
				}
			}
		case "--prefix":
			if i+1 < len(args) {
				customPrefix = args[i+1]
				showPrefix = true
				i++
			}
		case "--ready", "--ready-symbol":
			if i+1 < len(args) {
				customReady = args[i+1]
				i++
			}
		case "--working", "--working-symbol":
			if i+1 < len(args) {
				customWorking = args[i+1]
				i++
			}
		case "--waiting", "--waiting-symbol":
			if i+1 < len(args) {
				customWaiting = args[i+1]
				i++
			}
		case "--done", "--done-symbol":
			if i+1 < len(args) {
				customDone = args[i+1]
				i++
			}
		}
	}

	// 1. Drain stdin immediately to release parent pipe without IPC blocking
	inputBytes, _ := io.ReadAll(os.Stdin)
	inputBytes = cleanStdin(inputBytes)
	if len(bytes.TrimSpace(inputBytes)) == 0 {
		cwd, _ := os.Getwd()
		cwdName := filepath.Base(cwd)
		pStr := ""
		if showPrefix {
			pStr = customPrefix + " "
		}
		fmt.Printf("%s⚪ %s\n", pStr, cwdName)
		return
	}

	var data TelemetryInput
	if err := json.Unmarshal(inputBytes, &data); err != nil {
		cwd, _ := os.Getwd()
		cwdName := filepath.Base(cwd)
		pStr := ""
		if showPrefix {
			pStr = customPrefix + " "
		}
		fmt.Printf("%s⚪ %s\n", pStr, cwdName)
		return
	}

	// 2. Fast light-weight title config extraction (avoid parsing heavy permissions)
	homeDir, _ := os.UserHomeDir()
	settingsPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "settings.json")
	var titleCfg *TitleConfig
	if sf, err := os.ReadFile(settingsPath); err == nil {
		var sobj struct {
			Title *TitleConfig `json:"title"`
		}
		if err := json.Unmarshal(sf, &sobj); err == nil {
			titleCfg = sobj.Title
		}
	}

	symbols := resolveConfiguredSymbols(useTextStatus, customReady, customWorking, customWaiting, customDone, titleCfg)

	activeSubagents := 0
	for _, sub := range data.Subagents {
		st := strings.ToLower(strings.TrimSpace(sub.Status))
		if st != "" && st != "completed" && st != "failed" && st != "killed" && st != "error" && st != "cancelled" {
			activeSubagents++
		}
	}

	activeTasks := 0
	if data.TaskCount != nil {
		switch v := data.TaskCount.(type) {
		case float64:
			activeTasks = int(v)
		case int:
			activeTasks = v
		case string:
			if iv, err := strconv.Atoi(v); err == nil {
				activeTasks = iv
			}
		}
	}

	agyPid, agyTicks := getExactAncestorAgy(os.Getpid())

	title := ""
	hasHistory := false
	transcriptPath := ""
	convID := strings.TrimSpace(data.ConversationID)

	if data.TranscriptPath != "" {
		if _, err := os.Stat(data.TranscriptPath); err == nil {
			transcriptPath = data.TranscriptPath
		}
	}
	if transcriptPath == "" && convID != "" {
		pFull1 := filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs", "transcript_full.jsonl")
		p1 := filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs", "transcript.jsonl")
		pFull2 := filepath.Join(homeDir, ".gemini", "antigravity", "brain", convID, ".system_generated", "logs", "transcript_full.jsonl")
		p2 := filepath.Join(homeDir, ".gemini", "antigravity", "brain", convID, ".system_generated", "logs", "transcript.jsonl")
		if _, err := os.Stat(pFull1); err == nil {
			transcriptPath = pFull1
		} else if _, err := os.Stat(p1); err == nil {
			transcriptPath = p1
		} else if _, err := os.Stat(pFull2); err == nil {
			transcriptPath = pFull2
		} else if _, err := os.Stat(p2); err == nil {
			transcriptPath = p2
		} else {
			transcriptPath = p1
		}
	} else if transcriptPath != "" {
		pFull := strings.Replace(transcriptPath, "transcript.jsonl", "transcript_full.jsonl", 1)
		if _, err := os.Stat(pFull); err == nil {
			transcriptPath = pFull
		}
	}

	if t := strings.TrimSpace(data.ConversationTitle); t != "" {
		title = t
		hasHistory = true
	}

	if title == "" && convID != "" {
		db1 := filepath.Join(homeDir, ".gemini", "antigravity-cli", "conversation_summaries.db")
		db2 := filepath.Join(homeDir, ".gemini", "antigravity", "conversation_summaries.db")
		dbPath := ""
		if _, err := os.Stat(db1); err == nil {
			dbPath = db1
		} else if _, err := os.Stat(db2); err == nil {
			dbPath = db2
		}
		if dbPath != "" {
			if sTitle := getConversationTitle(dbPath, convID); strings.TrimSpace(sTitle) != "" {
				title = strings.TrimSpace(sTitle)
				hasHistory = true
			}
		}
	}

	if transcriptPath != "" {
		if t, found := getFirstPromptFromTranscript(transcriptPath, maxTitleLen); found {
			hasHistory = true
			if title == "" && t != "" {
				title = t
			}
		}
	}

	if title == "" {
		if data.Cwd != "" {
			title = filepath.Base(data.Cwd)
		} else {
			cwd, _ := os.Getwd()
			title = filepath.Base(cwd)
		}
	}

	statusIndicator := symbols.Ready

	agentState := strings.ToLower(strings.TrimSpace(data.AgentState))
	if agentState == "" {
		agentState = "idle"
	}

	targetMarkerDir := filepath.Join(homeDir, ".gemini", "antigravity-cli-title")
	markerPath := ""
	if convID != "" {
		_ = os.MkdirAll(targetMarkerDir, 0755)
		markerPath = filepath.Join(targetMarkerDir, fmt.Sprintf("marker_%s.txt", convID))
	}

	writeMarker := func(state string) {
		if agyPid > 0 && markerPath != "" {
			_ = os.WriteFile(markerPath, []byte(fmt.Sprintf("%d:%d:%s", agyPid, agyTicks, state)), 0644)
		}
	}

	switch agentState {
	case "thinking", "working":
		statusIndicator = symbols.Working
		writeMarker("RUNNING")

	case "waiting", "paused":
		statusIndicator = symbols.Waiting
		writeMarker("RUNNING")

	case "tool_use":
		statusIndicator = symbols.Working
		writeMarker("RUNNING")

		if transcriptPath != "" {
			pr, err := parseTranscriptTail(transcriptPath)
			if err == nil && pr != nil && len(pr.ToolCalls) > 0 {
				isWaitingForUser := false

				var perms *Permissions
				loadPerms := func() *Permissions {
					if perms == nil {
						if sf, err := os.ReadFile(settingsPath); err == nil {
							var sobj struct {
								Permissions *Permissions `json:"permissions"`
							}
							if err := json.Unmarshal(sf, &sobj); err == nil {
								perms = sobj.Permissions
							}
						}
					}
					return perms
				}

				for _, tc := range pr.ToolCalls {
					toolName := tc.Name
					switch toolName {
					case "ask_question", "ask", "prompt_user", "get_confirmation", "request_feedback":
						isWaitingForUser = true
					}
					if isWaitingForUser {
						break
					}

					perms := loadPerms()
					if perms != nil {
						if toolName == "run_command" && tc.Args != nil {
							if rawCmd, ok := tc.Args["CommandLine"]; ok && rawCmd != nil {
								cmd := unescapeCommand(fmt.Sprint(rawCmd))
								isAsk := false
								if perms.Ask != nil {
									for _, rule := range perms.Ask {
										if m := rxCmdRegex.FindStringSubmatch(rule); len(m) > 1 {
											if rx, err := regexp.Compile(m[1]); err == nil && rx.MatchString(cmd) {
												isAsk = true
												break
											}
										} else if m := rxCmdGlob.FindStringSubmatch(rule); len(m) > 1 {
											pat, patSub := globToRegex(m[1])
											if rx, err := regexp.Compile(pat); err == nil && rx.MatchString(cmd) {
												isAsk = true
												break
											}
											if rx, err := regexp.Compile(patSub); err == nil && rx.MatchString(cmd) {
												isAsk = true
												break
											}
										}
									}
								}

								if isAsk {
									hasWorker := false
									if agyPid > 0 {
										hasWorker = hasActiveWorker(agyPid, os.Getpid(), cmd)
									}
									if !hasWorker {
										isWaitingForUser = true
										break
									}
								} else {
									isAllow := false
									if perms.Allow != nil {
										for _, rule := range perms.Allow {
											if rule == "command(*)" || rule == "*" {
												isAllow = true
												break
											}
											if m := rxCmdRegex.FindStringSubmatch(rule); len(m) > 1 {
												if rx, err := regexp.Compile(m[1]); err == nil && rx.MatchString(cmd) {
													isAllow = true
													break
												}
											} else if m := rxCmdGlob.FindStringSubmatch(rule); len(m) > 1 {
												pat, patSub := globToRegex(m[1])
												if rx, err := regexp.Compile(pat); err == nil && rx.MatchString(cmd) {
													isAllow = true
													break
												}
												if rx, err := regexp.Compile(patSub); err == nil && rx.MatchString(cmd) {
													isAllow = true
													break
												}
											}
										}
									}
									if !isAllow {
										hasWorker := false
										if agyPid > 0 {
											hasWorker = hasActiveWorker(agyPid, os.Getpid(), cmd)
										}
										if !hasWorker {
											isWaitingForUser = true
											break
										}
									}
								}
							}
						} else if (toolName == "read_file" || toolName == "write_file" || toolName == "edit_file") && tc.Args != nil {
							if rawPath, ok := tc.Args["Path"]; ok && rawPath != nil {
								fPath := fmt.Sprint(rawPath)
								normPath := filepath.ToSlash(fPath)
								if perms.Ask != nil {
									rxToolRule := regexp.MustCompile(`^` + regexp.QuoteMeta(toolName) + `\((.*)\)$`)
									for _, rule := range perms.Ask {
										if m := rxToolRule.FindStringSubmatch(rule); len(m) > 1 {
											inner := m[1]
											if strings.HasPrefix(inner, "regex:") {
												regPat := strings.TrimPrefix(inner, "regex:")
												if rx, err := regexp.Compile(regPat); err == nil {
													if rx.MatchString(normPath) || rx.MatchString(fPath) {
														isWaitingForUser = true
														break
													}
												}
											} else {
												pat, patSub := fileGlobToRegex(inner)
												if rx, err := regexp.Compile(pat); err == nil && (rx.MatchString(normPath) || rx.MatchString(fPath)) {
													isWaitingForUser = true
													break
												}
												if rx, err := regexp.Compile(patSub); err == nil && rx.MatchString(normPath) {
													isWaitingForUser = true
													break
												}
												if strings.HasPrefix(filepath.ToSlash(inner), "../") && data.Cwd != "" {
													if !isInternalOrTempPath(normPath) {
														normCwd := strings.TrimRight(filepath.ToSlash(data.Cwd), "/")
														if !strings.HasPrefix(strings.ToLower(normPath), strings.ToLower(normCwd)) {
															isWaitingForUser = true
															break
														}
													}
												}
											}
										}
									}
									if isWaitingForUser {
										break
									}
								}
							}
						} else {
							// Generic / MCP tool matching in perms.ask
							if perms.Ask != nil {
								for _, rule := range perms.Ask {
									ruleTool := strings.TrimSpace(regexp.MustCompile(`\(.*$`).ReplaceAllString(rule, ""))
									if ruleTool == toolName || rule == "*" {
										isWaitingForUser = true
										break
									}
									if strings.Contains(ruleTool, "*") {
										pat, _ := globToRegex(ruleTool)
										if rx, err := regexp.Compile(pat); err == nil && rx.MatchString(toolName) {
											isWaitingForUser = true
											break
										}
									}
								}
								if isWaitingForUser {
									break
								}
							}
						}
					}
				}

				if isWaitingForUser {
					statusIndicator = symbols.Waiting
				}
			}
		}

	default: // idle and inactive states
		hasActiveBgTasks := false
		if activeTasks > 0 {
			hasActiveBgTasks = hasRecentlyActiveTasks(homeDir, convID, transcriptPath, taskTimeout)
		}

		if activeSubagents > 0 || hasActiveBgTasks {
			statusIndicator = symbols.Working
			writeMarker("RUNNING")
		} else {
			statusIndicator = symbols.Ready
			if hasHistory && convID != "" && markerPath != "" {
				if mf, err := os.ReadFile(markerPath); err == nil {
					content := strings.TrimSpace(string(mf))
					parts := strings.Split(content, ":")
					if len(parts) >= 3 && agyPid > 0 {
						savedPid := parts[0]
						savedTicks := parts[1]
						if strconv.Itoa(agyPid) == savedPid && strconv.FormatInt(agyTicks, 10) == savedTicks {
							statusIndicator = symbols.Done
							writeMarker("COMPLETED")
						}
					}
				}
			}
		}
	}

	cleanTitle := rxMultiWs.ReplaceAllString(title, " ")
	cleanTitle = strings.TrimSpace(cleanTitle)
	runes := []rune(cleanTitle)
	if len(runes) > maxTitleLen {
		cleanTitle = string(runes[:maxTitleLen-3]) + "..."
	}

	prefixStr := ""
	if showPrefix {
		prefixStr = customPrefix + " "
	}

	// Safe UTF-8 output
	fmt.Printf("%s%s %s\n", prefixStr, statusIndicator, cleanTitle)
}
