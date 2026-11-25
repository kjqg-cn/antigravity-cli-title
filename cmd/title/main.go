package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
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
	ID   string                 `json:"id"`
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

const appVersion = "0.4.0"

var (
	rxMultiWs       = regexp.MustCompile(`[\r\n\t]+`)
	rxMultiSpaces   = regexp.MustCompile(`\s+`)
	rxCmdRegex      = regexp.MustCompile(`(?s)^(?:command|run_command)\(regex:(.*)\)$`)
	rxCmdGlob       = regexp.MustCompile(`(?s)^(?:command|run_command)\((.*)\)$`)
	rxUserReq       = regexp.MustCompile(`(?s)<USER_REQUEST>(.*?)</USER_REQUEST>`)
	rxOrigTask      = regexp.MustCompile(`(?s)<original_task>(.*?)</original_task>`)
	rxMsgContent    = regexp.MustCompile(`(?s)\[Message\][^\n]*\s+content=(.*)`)
	rxRuleToolParen = regexp.MustCompile(`\(.*$`)
	rxToolCallRule  = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\((.*)\)$`)
	rxConvID        = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

func isPathInsideCwd(cwd, target string) bool {
	if cwd == "" || target == "" {
		return false
	}
	cwdClean := filepath.Clean(cwd)
	if !filepath.IsAbs(cwdClean) {
		if abs, err := filepath.Abs(cwdClean); err == nil {
			cwdClean = abs
		}
	}
	targetClean := target
	if filepath.IsAbs(targetClean) {
		// already absolute
	} else if strings.HasPrefix(targetClean, "/") || strings.HasPrefix(targetClean, "\\") {
		// Root-relative path. On Windows, attach cwd's volume if any.
		vol := filepath.VolumeName(cwdClean)
		if vol != "" {
			targetClean = filepath.Join(vol+"\\", targetClean)
		} else {
			targetClean = filepath.Clean(targetClean)
		}
	} else {
		if runtime.GOOS == "windows" && filepath.VolumeName(targetClean) != "" {
			return false // Drive-relative paths need per-drive cwd context we do not have.
		}
		targetClean = filepath.Join(cwdClean, targetClean)
	}
	targetClean = filepath.Clean(targetClean)

	rel, err := filepath.Rel(cwdClean, targetClean)
	if err != nil {
		return false
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") {
		return false
	}
	return true
}

func isWordDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '"', '\'', '`', ';', '|', '&', '(', ')', '[', ']', '{', '}', '<', '>', '=', ',', ':', '/', '\\':
		return true
	default:
		return false
	}
}

func isPhraseBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '"', '\'', '`', ';', '|', '&', '(', ')', '[', ']', '{', '}', '<', '>', '=', ',':
		return true
	default:
		return false
	}
}

func isStatementEnd(b byte) bool {
	switch b {
	case ';', '&', '|', '"', '\'', '`', ')', '}', '>', '<':
		return true
	default:
		return false
	}
}

func isDirLeftBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '"', '\'', '`', '(', '{', '[':
		return true
	default:
		return false
	}
}

func isDirRightBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', ';', '"', '\'', '`', '&', '|', ')', '}', ']', '>', '<':
		return true
	default:
		return false
	}
}

func matchRequiredDir(cmdline, reqDir string) bool {
	reqDir = strings.TrimRight(reqDir, "\\")
	if reqDir == "" {
		return true
	}
	rLen := len(reqDir)
	cLen := len(cmdline)
	if rLen > cLen {
		return false
	}

	idx := 0
	for {
		pos := strings.Index(cmdline[idx:], reqDir)
		if pos == -1 {
			break
		}
		start := idx + pos
		end := start + rLen

		leftOk := (start == 0 || isDirLeftBoundary(cmdline[start-1]))

		rightOk := false
		if end == cLen {
			rightOk = true
		} else {
			k := end
			if cmdline[k] == '\\' {
				k++
			}
			if k == cLen || isDirRightBoundary(cmdline[k]) {
				rightOk = true
			}
		}

		if leftOk && rightOk {
			return true
		}
		idx = start + 1
	}
	return false
}

func isBareFilename(tok string) bool {
	return strings.Contains(tok, ".") && !strings.ContainsAny(tok, "/\\")
}

func matchArgToken(s, tok string) bool {
	if s == "" || tok == "" {
		return false
	}
	if !isBareFilename(tok) {
		return matchWord(s, tok)
	}

	s = strings.ToLower(s)
	tok = strings.ToLower(tok)
	tLen := len(tok)
	sLen := len(s)
	if tLen > sLen {
		return false
	}

	isLeftArgBoundary := func(b byte) bool {
		switch b {
		case ' ', '\t', '\r', '\n', '"', '\'', '`', '=', '(':
			return true
		default:
			return false
		}
	}

	isRightArgBoundary := func(b byte) bool {
		switch b {
		case ' ', '\t', '\r', '\n', '"', '\'', '`', ';', '&', '|', ')', '}', '>', '<', ',':
			return true
		default:
			return false
		}
	}

	idx := 0
	for {
		pos := strings.Index(s[idx:], tok)
		if pos == -1 {
			break
		}
		start := idx + pos
		end := start + tLen

		leftOk := (start == 0 || isLeftArgBoundary(s[start-1]))
		rightOk := (end == sLen || isRightArgBoundary(s[end]))

		if leftOk && rightOk {
			return true
		}
		idx = start + 1
	}
	return false
}

func isPrepStatement(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Trim(s, "\"'`")
	if s == "" {
		return true
	}
	for _, prefix := range []string{"cd ", "cd\\", "cd/", "cd.", "chdir ", "chdir.", "set-location ", "sl ", "pushd ", "push-location ", "popd ", "pop-location "} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	if s == "cd" || s == "chdir" || s == "set-location" || s == "sl" || s == "pushd" || s == "push-location" || s == "popd" || s == "pop-location" {
		return true
	}
	if strings.HasPrefix(s, "$env:") || strings.HasPrefix(s, "set ") || strings.HasPrefix(s, "export ") {
		return true
	}
	if strings.HasPrefix(s, "chcp ") || s == "chcp" {
		return true
	}
	return false
}

func matchPhraseBoundary(s, phrase string) bool {
	if s == "" || phrase == "" {
		return false
	}
	s = strings.ToLower(s)
	phrase = strings.ToLower(phrase)
	pLen := len(phrase)
	sLen := len(s)
	if pLen > sLen {
		return false
	}

	phraseHasPipe := strings.Contains(phrase, "|")
	idx := 0
	for {
		pos := strings.Index(s[idx:], phrase)
		if pos == -1 {
			break
		}
		start := idx + pos
		end := start + pLen

		leftOk := false
		if start == 0 {
			leftOk = true
		} else if isPhraseBoundary(s[start-1]) {
			if !phraseHasPipe {
				k := start - 1
				for k >= 0 && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r' || s[k] == '\n') {
					k--
				}
				if k >= 0 && s[k] == '|' {
					leftOk = false
				} else {
					leftOk = true
				}
			} else {
				leftOk = true
			}
		}

		rightOk := false
		if end == sLen {
			rightOk = true
		} else {
			k := end
			for k < sLen && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r' || s[k] == '\n') {
				k++
			}
			if k == sLen {
				rightOk = true
			} else if isStatementEnd(s[k]) {
				rightOk = true
			}
		}

		if leftOk && rightOk {
			return true
		}
		idx = start + 1
	}
	return false
}

func matchWord(s, word string) bool {
	if s == "" || word == "" {
		return false
	}
	s = strings.ToLower(s)
	word = strings.ToLower(word)
	wLen := len(word)
	sLen := len(s)
	if wLen > sLen {
		return false
	}

	idx := 0
	for {
		pos := strings.Index(s[idx:], word)
		if pos == -1 {
			break
		}
		start := idx + pos
		end := start + wLen

		leftOk := (start == 0) || isWordDelimiter(s[start-1])
		rightOk := (end == sLen) || isWordDelimiter(s[end])
		if !rightOk && s[end] == '.' {
			rest := s[end:]
			if strings.HasPrefix(rest, ".exe") || strings.HasPrefix(rest, ".cmd") ||
				strings.HasPrefix(rest, ".bat") || strings.HasPrefix(rest, ".ps1") ||
				strings.HasPrefix(rest, ".sh") {
				rightOk = true
			}
		}

		if leftOk && rightOk {
			return true
		}
		idx = start + 1
	}
	return false
}

func printUsage() {
	fmt.Printf("Antigravity CLI Window Title (Native Engine) v%s\n\n", appVersion)
	fmt.Println("Usage:")
	fmt.Println("  title [options] < telemetry_payload.json")
	fmt.Println("  echo '{\"agent_state\":\"working\"}' | title [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  --animate                Animate Working title on Windows (opt-in)")
	fmt.Println("  --no-animate             Disable animation (also --animate=false)")
	fmt.Println("  --no-prefix              Disable 'agy:' prefix in title")
	fmt.Println("  --prefix <str>           Set custom prefix (default: 'agy:')")
	fmt.Println("  --text, --classic        Use text indicators ([READY], [WORKING], etc.)")
	fmt.Println("  --max-length <n>         Set max window title character length (default: 35)")
	fmt.Println("  --task-timeout <sec>     Background task activity timeout in seconds (default: 60)")
	fmt.Println("  --ready <symbol>         Custom symbol for Ready state")
	fmt.Println("  --working <symbol>       Custom symbol for Working state")
	fmt.Println("  --waiting <symbol>       Custom symbol for Waiting state")
	fmt.Println("  --done <symbol>          Custom symbol for Done state")
	fmt.Println("  -h, --help               Show help")
	fmt.Println("  -v, --version            Show version")
}

func globToRegex(glob string) (string, string) {
	escaped := regexp.QuoteMeta(glob)
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
	escaped = strings.ReplaceAll(escaped, `\?`, `.`)
	pat := "(?is)^" + escaped + "$"
	patSub := "(?is)(?:^|\\s)" + escaped + "$"
	return pat, patSub
}

func fileGlobToRegex(glob string) (string, string) {
	if strings.HasPrefix(glob, "~/") || strings.HasPrefix(glob, `~\`) {
		if homeDir, err := os.UserHomeDir(); err == nil {
			glob = filepath.Join(homeDir, glob[2:])
		}
	}
	norm := filepath.ToSlash(glob)
	escaped := regexp.QuoteMeta(norm)
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
	escaped = strings.ReplaceAll(escaped, `\?`, `.`)
	mode := "(?s)"
	if runtime.GOOS == "windows" {
		mode = "(?is)"
	}
	pat := mode + "^" + escaped + "$"
	patSub := mode + "(?:^|/)" + escaped + "$"
	return pat, patSub
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
	readLen := int64(4 << 20) // Bounded tail, including large full-transcript tool outputs.
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
	genericCount := 0
	completed := make(map[string]bool)
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
			genericCount++
			var result struct {
				ToolCallID string `json:"tool_call_id"`
			}
			if json.Unmarshal(line, &result) == nil && result.ToolCallID != "" {
				completed[result.ToolCallID] = true
			}
			continue
		}

		if hdr.Type == "PLANNER_RESPONSE" {
			var pr PlannerResponse
			if err := json.Unmarshal(line, &pr); err == nil {
				if genericCount >= len(pr.ToolCalls) {
					return nil, nil
				}
				pending := pr.ToolCalls[:0]
				for _, tc := range pr.ToolCalls {
					if tc.ID == "" || !completed[tc.ID] {
						pending = append(pending, tc)
					}
				}
				pr.ToolCalls = pending
				return &pr, nil
			}
			return nil, nil
		}

		if hdr.Type == "USER_INPUT" || hdr.Type == "SYSTEM_MESSAGE" {
			break
		}
	}
	return nil, nil
}

func cleanPromptLine(rl string, maxTitleLen int) string {
	rl = strings.TrimSpace(rl)
	rl = strings.TrimLeft(rl, "# \t")
	if strings.HasPrefix(strings.ToLower(rl), "**task**:") {
		rl = strings.TrimSpace(rl[9:])
	} else if strings.HasPrefix(strings.ToLower(rl), "task:") {
		rl = strings.TrimSpace(rl[5:])
	}
	rl = strings.TrimLeft(rl, "*# \t")
	runes := []rune(rl)
	if maxTitleLen < 4 {
		maxTitleLen = 4
	}
	if len(runes) > maxTitleLen {
		rl = string(runes[:maxTitleLen-3]) + "..."
	}
	return rl
}

func getFirstPromptFromTranscript(transcriptPath string, maxTitleLen int) (string, bool) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(io.LimitReader(f, 4<<20))
	scanner.Buffer(make([]byte, 32768), 4<<20)

	var subagentTask string
	hasSubagentTask := false

	for i := 0; i < 2000 && scanner.Scan(); i++ {
		line := bytes.TrimSpace(scanner.Bytes())
		line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		var hdr stepHeader
		if json.Unmarshal(line, &hdr) != nil {
			continue
		}
		if hdr.Type == "USER_INPUT" {
			var obj map[string]interface{}
			if err := json.Unmarshal(line, &obj); err == nil {
				if content, ok := obj["content"].(string); ok && content != "" {
					raw := content
					if m := rxUserReq.FindStringSubmatch(content); len(m) > 1 {
						raw = m[1]
					}
					rawLines := strings.Split(raw, "\n")
					for _, rl := range rawLines {
						cleaned := cleanPromptLine(rl, maxTitleLen)
						if len(cleaned) > 0 {
							return cleaned, true
						}
					}
				}
			}
			return "", true
		}

		if !hasSubagentTask && hdr.Type == "SYSTEM_MESSAGE" {
			var obj map[string]interface{}
			if err := json.Unmarshal(line, &obj); err == nil {
				if content, ok := obj["content"].(string); ok && content != "" {
					raw := ""
					if m := rxOrigTask.FindStringSubmatch(content); len(m) > 1 {
						raw = m[1]
					} else if m := rxMsgContent.FindStringSubmatch(content); len(m) > 1 {
						raw = m[1]
					}
					if raw != "" {
						rawLines := strings.Split(raw, "\n")
						for _, rl := range rawLines {
							cleaned := cleanPromptLine(rl, maxTitleLen)
							if len(cleaned) > 0 {
								subagentTask = cleaned
								hasSubagentTask = true
								break
							}
						}
					}
				}
			}
		}
	}

	if hasSubagentTask {
		return subagentTask, true
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

func splitArgs(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	var quoteChar rune
	escaped := false
	hasToken := false

	for _, r := range s {
		if inQuote {
			if escaped {
				if r != quoteChar && r != '\\' {
					cur.WriteRune('\\')
				}
				cur.WriteRune(r)
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quoteChar {
				inQuote = false
			} else {
				cur.WriteRune(r)
			}
			continue
		}

		// Outside quotes: backslash is NOT an escape character (retains Windows paths like C:\agy)
		if r == '"' || r == '\'' {
			inQuote = true
			quoteChar = r
			hasToken = true
			continue
		}
		if unicode.IsSpace(r) {
			if hasToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				hasToken = false
			}
			continue
		}
		cur.WriteRune(r)
		hasToken = true
	}
	if inQuote && escaped {
		cur.WriteRune('\\')
	}
	if hasToken {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func tokenizeWords(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n', '"', '\'', '`', ';', '|', '&', '(', ')', '[', ']', '{', '}', '<', '>', '=', ',':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func splitCommandStatements(s string) []string {
	var stmts []string
	for _, part := range splitCommandParts(s) {
		stmts = append(stmts, part.Text)
	}
	return stmts
}

func splitCommandParts(s string) []commandStatement {
	var stmts []commandStatement
	var cur strings.Builder
	inQuote := false
	var quoteChar rune
	piped := false

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
			cur.WriteRune(r)
			i += size
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = true
			quoteChar = r
			cur.WriteRune(r)
			i += size
			continue
		}
		if r == ';' || r == '\n' || r == '\r' || r == '|' {
			trimmed := strings.TrimSpace(cur.String())
			if trimmed != "" {
				stmts = append(stmts, commandStatement{trimmed, piped})
			}
			cur.Reset()
			i += size
			piped = r == '|'
			if r == '|' && i < len(s) && s[i] == '|' {
				i++
				piped = false
			}
			continue
		}
		if r == '&' {
			i += size
			if i < len(s) && s[i] == '&' {
				i++
			}
			trimmed := strings.TrimSpace(cur.String())
			if trimmed != "" {
				stmts = append(stmts, commandStatement{trimmed, piped})
			}
			piped = false
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
		i += size
	}
	trimmed := strings.TrimSpace(cur.String())
	if trimmed != "" {
		stmts = append(stmts, commandStatement{trimmed, piped})
	}
	return stmts
}

func normalizeCmdText(s string) string {
	fields := strings.Fields(strings.ToLower(s))
	return strings.Join(fields, " ")
}

func stripQuotesAndEscapes(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r != '"' && r != '\'' && r != '`' && r != '\\' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

type targetCommandPattern struct {
	targetCmd      string
	targetExe      string
	altExe         string
	innerExe       string
	coreTokens     []string
	allTokens      []string
	valid          bool
	normalizedFull string
	firstStmt      string
	subStatements  []string
	requiredDir    string
}

func parseWorkerTarget(targetCmd string) targetCommandPattern {
	stmts := splitCommandStatements(targetCmd)
	if len(stmts) == 0 {
		return targetCommandPattern{valid: false}
	}

	primaryStmt := stmts[0]
	for _, s := range stmts {
		if !isPrepStatement(s) {
			primaryStmt = s
			break
		}
	}

	tokens := tokenizeWords(primaryStmt)
	if len(tokens) == 0 {
		return targetCommandPattern{valid: false}
	}

	rawFirst := tokens[0]
	baseFirst := strings.ToLower(filepath.Base(rawFirst))
	baseName := strings.TrimSuffix(baseFirst, ".exe")
	if baseName == "" {
		return targetCommandPattern{valid: false}
	}

	isShell := (baseName == "powershell" || baseName == "pwsh" || baseName == "cmd" ||
		baseName == "sh" || baseName == "bash" || baseName == "zsh")

	var alt string
	if baseName == "powershell" {
		alt = "pwsh"
	} else if baseName == "pwsh" {
		alt = "powershell"
	} else if baseName == "python" {
		alt = "python3"
	} else if baseName == "python3" {
		alt = "python"
	}

	var innerExe string
	var coreTokens []string
	var allTokens []string

	isShellNoise := func(s string) bool {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "-")
		s = strings.TrimPrefix(s, "/")
		switch s {
		case "bypass", "noprofile", "executionpolicy", "command", "c", "e", "noninteractive", "ep":
			return true
		default:
			return false
		}
	}

	for _, tok := range tokens[1:] {
		tokClean := strings.Trim(tok, "\"'`(),;:")
		if tokClean == "" {
			continue
		}
		low := strings.ToLower(tokClean)
		if isShell && isShellNoise(low) {
			continue
		}

		// If this is a shell invocation and we haven't found the inner command yet
		if isShell && innerExe == "" {
			innerBase := strings.TrimSuffix(strings.ToLower(filepath.Base(low)), ".exe")
			if len([]rune(innerBase)) >= 2 && !strings.HasPrefix(innerBase, "-") && !strings.HasPrefix(innerBase, "/") {
				innerExe = innerBase
			}
		}

		allTokens = append(allTokens, low)

		// Legacy coreTokens filter (retained for backward compatibility):
		if strings.HasPrefix(tokClean, "-") || strings.HasPrefix(tokClean, "/") {
			continue
		}
		isNum := true
		for _, r := range tokClean {
			if !unicode.IsDigit(r) {
				isNum = false
				break
			}
		}
		if isNum {
			continue
		}
		if len([]rune(low)) < 3 && low != innerExe {
			continue
		}
		coreTokens = append(coreTokens, low)
	}

	if isShell && len(allTokens) == 0 {
		return targetCommandPattern{valid: false}
	}

	normFull := normalizeCmdText(targetCmd)
	var normStmts []string
	var firstStmt string
	var requiredDir string

	extractDir := func(s string) string {
		for _, prefix := range []string{"cd ", "cd\\", "cd/", "chdir ", "set-location ", "sl ", "pushd ", "push-location "} {
			if strings.HasPrefix(s, prefix) {
				d := strings.TrimSpace(strings.TrimPrefix(s, prefix))
				var cleanD strings.Builder
				inQ := false
				var qChar rune
				for _, r := range d {
					if inQ {
						if r == qChar {
							inQ = false
						}
						cleanD.WriteRune(r)
						continue
					}
					if r == '"' || r == '\'' || r == '`' {
						inQ = true
						qChar = r
						cleanD.WriteRune(r)
						continue
					}
					if r == ';' || r == '&' || r == '|' {
						break
					}
					cleanD.WriteRune(r)
				}
				d = strings.TrimSpace(cleanD.String())
				if strings.HasPrefix(strings.ToLower(d), "/d ") {
					d = strings.TrimSpace(d[3:])
				}
				lowD := strings.ToLower(d)
				if strings.HasPrefix(lowD, "-path ") {
					d = strings.TrimSpace(d[6:])
				} else if strings.HasPrefix(lowD, "-literalpath ") {
					d = strings.TrimSpace(d[13:])
				}
				d = strings.Trim(d, "\"'`")
				if d == "" || strings.Contains(d, "$") || strings.Contains(d, "%") {
					return ""
				}
				return d
			}
		}
		return ""
	}

	for _, stmt := range stmts {
		ns := normalizeCmdText(stmt)
		if ns != "" {
			if d := extractDir(ns); d != "" && requiredDir == "" {
				requiredDir = d
			}
			if !isPrepStatement(ns) {
				normStmts = append(normStmts, ns)
				if firstStmt == "" {
					firstStmt = ns
				}
			}
		}
	}

	// Extract inner commands if wrapped in a shell execution flag
	if isShell {
		if innerPart, ok := shellPayload(targetCmd); ok {
			for _, stmt := range splitCommandStatements(innerPart) {
				ns := normalizeCmdText(stmt)
				if d := extractDir(ns); d != "" && requiredDir == "" {
					requiredDir = d
				}
				if ns != "" && !isPrepStatement(ns) {
					normStmts = append(normStmts, ns)
				}
			}
		}
	}

	return targetCommandPattern{
		targetCmd:      targetCmd,
		targetExe:      baseName,
		altExe:         alt,
		innerExe:       innerExe,
		coreTokens:     coreTokens,
		allTokens:      allTokens,
		valid:          true,
		normalizedFull: normFull,
		firstStmt:      firstStmt,
		subStatements:  normStmts,
		requiredDir:    requiredDir,
	}
}

func matchWorkerProcess(procExe, cmdline string, pat targetCommandPattern, processCwd ...string) bool {
	if !pat.valid || strings.TrimSpace(cmdline) == "" {
		return false
	}
	if pat.requiredDir != "" && cmdline != "" {
		verifiedCwd := len(processCwd) > 0 && processCwd[0] != "" && filepath.IsAbs(pat.requiredDir) &&
			isPathInsideCwd(pat.requiredDir, processCwd[0]) && isPathInsideCwd(processCwd[0], pat.requiredDir)
		cleanCmd := strings.ReplaceAll(strings.ToLower(cmdline), "/", "\\")
		cleanReq := strings.ReplaceAll(strings.ToLower(pat.requiredDir), "/", "\\")
		if !verifiedCwd && !matchRequiredDir(cleanCmd, cleanReq) {
			return false
		}
	}

	normCmdline := normalizeCmdText(cmdline)
	cleanCmdline := stripQuotesAndEscapes(normCmdline)

	stripExeSuffix := func(s string) string {
		s = strings.ReplaceAll(s, ".exe ", " ")
		s = strings.ReplaceAll(s, ".exe\"", "\"")
		s = strings.ReplaceAll(s, ".exe'", "'")
		s = strings.ReplaceAll(s, ".exe;", ";")
		s = strings.ReplaceAll(s, ".exe&", "&")
		s = strings.ReplaceAll(s, ".exe|", "|")
		s = strings.ReplaceAll(s, ".exe)", ")")
		s = strings.ReplaceAll(s, ".exe}", "}")
		s = strings.TrimSuffix(s, ".exe")
		return s
	}

	noExeCmdline := stripExeSuffix(normCmdline)
	targets := executableStatements(pat.targetCmd)
	candidates := executableStatements(cmdline)
	if len(targets) > 1 {
		if len(targets) == len(candidates) {
			for i := range targets {
				if !sameCommand(candidates[i], targets[i]) {
					return false
				}
			}
			return true
		}
		// A native final-stage child can outlive its shell; a shared first stage
		// cannot establish that a different compound command was approved.
		lastArgs := splitArgs(targets[len(targets)-1])
		return len(candidates) == 1 && len(lastArgs) > 0 && commandBase(procExe) == commandBase(lastArgs[0]) && sameCommand(candidates[0], targets[len(targets)-1])
	}
	for _, candidate := range candidates {
		for _, target := range targets {
			if sameCommand(candidate, target) {
				return true
			}
		}
	}

	procExe = strings.TrimSuffix(strings.ToLower(filepath.Base(procExe)), ".exe")
	exeMatches := (procExe == pat.targetExe || (pat.altExe != "" && procExe == pat.altExe) || (pat.innerExe != "" && procExe == pat.innerExe))

	if exeMatches {
		if len(pat.allTokens) == 0 && len(pat.coreTokens) == 0 {
			if cmdline != "" {
				if pat.normalizedFull != "" && !matchPhraseBoundary(noExeCmdline, stripExeSuffix(pat.normalizedFull)) {
					return false
				}
			}
			return true
		}
		if cmdline != "" {
			for _, tok := range pat.coreTokens {
				if procExe == pat.innerExe && tok == pat.targetExe {
					continue
				}
				if !matchArgToken(cmdline, tok) {
					return false
				}
			}
			for _, tok := range pat.allTokens {
				if procExe == pat.innerExe && tok == pat.targetExe {
					continue
				}
				if !matchArgToken(cmdline, tok) {
					return false
				}
			}
			if len(pat.allTokens) > 0 {
				argPhrase := strings.Join(pat.allTokens, " ")
				if !matchPhraseBoundary(normCmdline, argPhrase) && !matchPhraseBoundary(cleanCmdline, argPhrase) {
					if pat.normalizedFull != "" && !matchPhraseBoundary(noExeCmdline, stripExeSuffix(pat.normalizedFull)) {
						return false
					}
				}
			}
			return true
		} else {
			return false
		}
	}

	return false
}

func cleanTitleText(s string) string {
	var sb strings.Builder
	for _, r := range s {
		// Strip C0 controls (0x00-0x1F, which includes \a / BEL 0x07, \x1b / ESC 0x1B, \r, \n, \t)
		// Strip DEL (0x7F)
		// Strip C1 controls (0x80-0x9F)
		// Strip unicode.ReplacementChar (which arises from invalid UTF-8 byte sequences)
		if (r < 0x20) || (r >= 0x7F && r <= 0x9F) || r == unicode.ReplacementChar {
			sb.WriteRune(' ')
			continue
		}
		sb.WriteRune(r)
	}
	res := rxMultiSpaces.ReplaceAllString(sb.String(), " ")
	return strings.TrimSpace(res)
}

type ParsedFlags struct {
	Animate       bool
	ShowPrefix    *bool
	Prefix        *string
	UseTextStatus *bool
	MaxLength     *int
	TaskTimeout   *time.Duration
	Ready         string
	Working       string
	Waiting       string
	Done          string
}

func parseCommandFlags(cmdStr string) ParsedFlags {
	tokens := expandFlagEquals(splitArgs(cmdStr))
	var res ParsedFlags
	for i := 0; i < len(tokens); i++ {
		t := strings.ToLower(tokens[i])
		switch t {
		case "--animate":
			res.Animate = true
		case "--no-animate":
			res.Animate = false
		case "--no-prefix":
			val := false
			res.ShowPrefix = &val
		case "--text", "--classic":
			val := true
			res.UseTextStatus = &val
		case "--prefix":
			if i+1 < len(tokens) {
				val := tokens[i+1]
				res.Prefix = &val
				i++
			}
		case "--max-length":
			if i+1 < len(tokens) {
				if v, err := strconv.Atoi(tokens[i+1]); err == nil && v > 5 {
					res.MaxLength = &v
				}
				i++
			}
		case "--task-timeout":
			if i+1 < len(tokens) {
				if v, err := strconv.Atoi(tokens[i+1]); err == nil && v > 0 {
					dur := time.Duration(v) * time.Second
					res.TaskTimeout = &dur
				}
				i++
			}
		case "--ready", "--ready-symbol":
			if i+1 < len(tokens) {
				res.Ready = tokens[i+1]
				i++
			}
		case "--working", "--working-symbol":
			if i+1 < len(tokens) {
				res.Working = tokens[i+1]
				i++
			}
		case "--waiting", "--waiting-symbol":
			if i+1 < len(tokens) {
				res.Waiting = tokens[i+1]
				i++
			}
		case "--done", "--done-symbol":
			if i+1 < len(tokens) {
				res.Done = tokens[i+1]
				i++
			}
		}
	}
	return res
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

	// 1. Explicit CLI flags override defaults
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

	// 2. Settings.json takes priority over CLI flags (for instant hot-reload)
	if cfg != nil {
		// 2a. Flags from settings.json "command" string
		flags := parseCommandFlags(cfg.Command)
		if r := cleanSymbol(flags.Ready); r != "" {
			s.Ready = r
		}
		if w := cleanSymbol(flags.Working); w != "" {
			s.Working = w
		}
		if wt := cleanSymbol(flags.Waiting); wt != "" {
			s.Waiting = wt
		}
		if d := cleanSymbol(flags.Done); d != "" {
			s.Done = d
		}

		// 2b. Direct keys in settings.json have highest priority
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

	return s
}

type StateInput struct {
	AgentState       string
	ToolCalls        []ToolCall
	Perms            *Permissions
	AgyPid           int
	CurrentPid       int
	ActiveTasks      int
	HasActiveBgTasks bool
	ActiveSubagents  int
	HasHistory       bool
	MarkerExists     bool
	MarkerContent    string
	AgyTicks         int64
	Symbols          StatusSymbols
	Cwd              string
	HasWorkerFunc    func(agyPid, currentPid int, targetCmd string) bool
}

type StateResult struct {
	IsWorking       bool
	StatusIndicator string
	MarkerAction    string // "RUNNING", "COMPLETED", or ""
}

func resolveState(in StateInput) StateResult {
	agentState := strings.ToLower(strings.TrimSpace(in.AgentState))
	if agentState == "" {
		agentState = "idle"
	}

	switch agentState {
	case "thinking", "working":
		return StateResult{
			StatusIndicator: in.Symbols.Working,
			IsWorking:       true,
			MarkerAction:    "RUNNING",
		}

	case "waiting", "paused":
		return StateResult{
			StatusIndicator: in.Symbols.Waiting,
			MarkerAction:    "RUNNING",
		}

	case "tool_use":
		isWaitingForUser := false
		perms := in.Perms
		if perms == nil {
			perms = &Permissions{}
		}

		for _, tc := range in.ToolCalls {
			toolName := strings.TrimSpace(tc.Name)
			switch toolName {
			case "ask_question", "ask", "prompt_user", "get_confirmation", "request_feedback":
				isWaitingForUser = true
			}
			if isWaitingForUser {
				break
			}

			if toolName == "run_command" && tc.Args != nil {
				if rawCmd, ok := tc.Args["CommandLine"]; ok && rawCmd != nil {
					cmd := fmt.Sprint(rawCmd)
					isAsk := false
					if perms.Ask != nil {
						for _, rule := range perms.Ask {
							if rule == "*" || rule == "run_command" || rule == "command" || rule == "command(*)" || rule == "run_command(*)" {
								isAsk = true
								break
							}
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
						if in.AgyPid > 0 && in.HasWorkerFunc != nil {
							hasWorker = in.HasWorkerFunc(in.AgyPid, in.CurrentPid, cmd)
						}
						if !hasWorker {
							isWaitingForUser = true
							break
						}
					} else {
						isAllow := false
						if perms.Allow != nil {
							for _, rule := range perms.Allow {
								if rule == "command(*)" || rule == "run_command(*)" || rule == "*" || rule == "run_command" || rule == "command" {
									isAllow = true
									break
								}
								if m := rxCmdRegex.FindStringSubmatch(rule); len(m) > 1 {
									if rx, err := regexp.Compile("(?s)^(?:" + m[1] + ")$"); err == nil && rx.MatchString(cmd) {
										isAllow = true
										break
									}
								} else if m := rxCmdGlob.FindStringSubmatch(rule); len(m) > 1 {
									pat, _ := globToRegex(m[1])
									// Item 9: allow permissions matching MUST strictly use anchored pat, NEVER patSub
									if rx, err := regexp.Compile(pat); err == nil && rx.MatchString(cmd) {
										isAllow = true
										break
									}
								}
							}
						}
						if !isAllow {
							hasWorker := false
							if in.AgyPid > 0 && in.HasWorkerFunc != nil {
								hasWorker = in.HasWorkerFunc(in.AgyPid, in.CurrentPid, cmd)
							}
							if !hasWorker {
								isWaitingForUser = true
								break
							}
						}
					}
				}
			} else if (toolName == "read_file" || toolName == "write_file" || toolName == "edit_file" || toolName == "replace_file_content" || toolName == "multi_replace_file_content" || toolName == "write_to_file" || toolName == "view_file") && tc.Args != nil {
				var rawPath interface{}
				for _, k := range []string{"Path", "TargetFile", "path", "file_path", "target_file", "targetFile", "AbsolutePath", "target_path", "filePath"} {
					if v, ok := tc.Args[k]; ok && v != nil {
						rawPath = v
						break
					}
				}
				if rawPath != nil {
					fPath := fmt.Sprint(rawPath)
					if runtime.GOOS == "windows" && filepath.VolumeName(fPath) != "" && !filepath.IsAbs(fPath) {
						// Drive-relative paths depend on per-drive shell state; joining cwd
						// would erase that ambiguity and could falsely classify a write as safe.
						isWaitingForUser = true
						break
					}
					absPath := fPath
					if filepath.IsAbs(absPath) {
						// already absolute
					} else if (strings.HasPrefix(absPath, "/") || strings.HasPrefix(absPath, "\\")) && in.Cwd != "" {
						vol := filepath.VolumeName(in.Cwd)
						if vol != "" {
							absPath = filepath.Join(vol+"\\", absPath)
						}
					} else if in.Cwd != "" {
						absPath = filepath.Join(in.Cwd, absPath)
					}
					absPath = filepath.Clean(absPath)
					normPath := filepath.ToSlash(absPath)
					origNorm := filepath.ToSlash(fPath)

					if perms.Ask != nil {
						for _, rule := range perms.Ask {
							if rule == "*" || rule == toolName || rule == toolName+"(*)" {
								isWaitingForUser = true
								break
							}
							if m := rxToolCallRule.FindStringSubmatch(rule); len(m) > 2 && m[1] == toolName {
								inner := m[2]
								if strings.HasPrefix(inner, "regex:") {
									regPat := strings.TrimPrefix(inner, "regex:")
									if rx, err := regexp.Compile(regPat); err == nil {
										if rx.MatchString(normPath) || rx.MatchString(origNorm) || rx.MatchString(fPath) {
											isWaitingForUser = true
											break
										}
									}
								} else {
									pat, patSub := fileGlobToRegex(inner)
									if rx, err := regexp.Compile(pat); err == nil && (rx.MatchString(normPath) || rx.MatchString(origNorm) || rx.MatchString(fPath)) {
										isWaitingForUser = true
										break
									}
									if rx, err := regexp.Compile(patSub); err == nil && (rx.MatchString(normPath) || rx.MatchString(origNorm)) {
										isWaitingForUser = true
										break
									}
									if (inner == ".." || strings.HasPrefix(filepath.ToSlash(inner), "../")) && in.Cwd != "" {
										if !isInternalOrTempPath(normPath) {
											if !isPathInsideCwd(in.Cwd, absPath) {
												isWaitingForUser = true
												break
											}
										}
									}
								}
							}
						}
					}
					if !isWaitingForUser && (toolName == "write_file" || toolName == "edit_file" || toolName == "replace_file_content" || toolName == "multi_replace_file_content" || toolName == "write_to_file") {
						if in.Cwd != "" && !isInternalOrTempPath(normPath) {
							if !isPathInsideCwd(in.Cwd, absPath) {
								isWaitingForUser = true
								break
							}
						}
					}
					if isWaitingForUser {
						break
					}
				}
			} else {
				// Generic / MCP tool matching in perms.ask
				if perms.Ask != nil {
					for _, rule := range perms.Ask {
						ruleTool := strings.TrimSpace(rxRuleToolParen.ReplaceAllString(rule, ""))
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

		if isWaitingForUser {
			return StateResult{
				StatusIndicator: in.Symbols.Waiting,
				MarkerAction:    "RUNNING",
			}
		}
		return StateResult{
			StatusIndicator: in.Symbols.Working,
			IsWorking:       true,
			MarkerAction:    "RUNNING",
		}

	case "idle", "inactive":
		if in.ActiveSubagents > 0 || in.HasActiveBgTasks {
			return StateResult{
				StatusIndicator: in.Symbols.Working,
				IsWorking:       true,
				MarkerAction:    "RUNNING",
			}
		}

		if in.HasHistory && in.MarkerExists && in.MarkerContent != "" {
			parts := strings.Split(strings.TrimSpace(in.MarkerContent), ":")
			if len(parts) == 3 && (parts[2] == "RUNNING" || parts[2] == "COMPLETED") && in.AgyPid > 0 {
				savedPid := parts[0]
				savedTicks := parts[1]
				if strconv.Itoa(in.AgyPid) == savedPid && strconv.FormatInt(in.AgyTicks, 10) == savedTicks {
					return StateResult{
						StatusIndicator: in.Symbols.Done,
						MarkerAction:    "COMPLETED",
					}
				}
			}
		}

		return StateResult{
			StatusIndicator: in.Symbols.Ready,
			MarkerAction:    "",
		}
	}
	return StateResult{StatusIndicator: in.Symbols.Ready}
}

type RuntimeConfig struct {
	Animate       bool
	ShowPrefix    bool
	CustomPrefix  string
	UseTextStatus bool
	MaxTitleLen   int
	TaskTimeout   time.Duration
	CustomReady   string
	CustomWorking string
	CustomWaiting string
	CustomDone    string
	Symbols       StatusSymbols
}

func resolveRuntimeConfig(cliArgs []string, titleCfg *TitleConfig) RuntimeConfig {
	cliArgs = expandFlagEquals(cliArgs)
	cfg := RuntimeConfig{
		ShowPrefix:    true,
		CustomPrefix:  "agy:",
		UseTextStatus: false,
		MaxTitleLen:   35,
		TaskTimeout:   60 * time.Second,
	}

	// 1. Parse CLI flags from cliArgs (baseline values from invocation)
	for i := 0; i < len(cliArgs); i++ {
		arg := strings.ToLower(cliArgs[i])
		switch arg {
		case "--animate":
			cfg.Animate = true
		case "--no-animate":
			cfg.Animate = false
		case "--no-prefix":
			cfg.ShowPrefix = false
		case "--text", "--classic":
			cfg.UseTextStatus = true
		case "--max-length":
			if i+1 < len(cliArgs) {
				if v, err := strconv.Atoi(cliArgs[i+1]); err == nil && v > 5 {
					cfg.MaxTitleLen = v
				}
				i++
			}
		case "--task-timeout":
			if i+1 < len(cliArgs) {
				if v, err := strconv.Atoi(cliArgs[i+1]); err == nil && v > 0 {
					cfg.TaskTimeout = time.Duration(v) * time.Second
				}
				i++
			}
		case "--prefix":
			if i+1 < len(cliArgs) {
				cfg.CustomPrefix = cliArgs[i+1]
				cfg.ShowPrefix = true
				i++
			}
		case "--ready", "--ready-symbol":
			if i+1 < len(cliArgs) {
				cfg.CustomReady = cliArgs[i+1]
				i++
			}
		case "--working", "--working-symbol":
			if i+1 < len(cliArgs) {
				cfg.CustomWorking = cliArgs[i+1]
				i++
			}
		case "--waiting", "--waiting-symbol":
			if i+1 < len(cliArgs) {
				cfg.CustomWaiting = cliArgs[i+1]
				i++
			}
		case "--done", "--done-symbol":
			if i+1 < len(cliArgs) {
				cfg.CustomDone = cliArgs[i+1]
				i++
			}
		}
	}

	// 2. Hot-reload: settings.json "command" string and direct keys take priority over os.Args
	if titleCfg != nil && strings.TrimSpace(titleCfg.Command) != "" {
		cfg.Animate = false
		cfg.ShowPrefix = true
		cfg.CustomPrefix = "agy:"
		cfg.UseTextStatus = false
		cfg.MaxTitleLen = 35
		cfg.TaskTimeout = 60 * time.Second
		cfg.CustomReady = ""
		cfg.CustomWorking = ""
		cfg.CustomWaiting = ""
		cfg.CustomDone = ""

		flags := parseCommandFlags(titleCfg.Command)
		cfg.Animate = flags.Animate
		if flags.ShowPrefix != nil {
			cfg.ShowPrefix = *flags.ShowPrefix
		}
		if flags.Prefix != nil {
			cfg.CustomPrefix = *flags.Prefix
			cfg.ShowPrefix = true
		}
		if flags.UseTextStatus != nil {
			cfg.UseTextStatus = *flags.UseTextStatus
		}
		if flags.MaxLength != nil {
			cfg.MaxTitleLen = *flags.MaxLength
		}
		if flags.TaskTimeout != nil {
			cfg.TaskTimeout = *flags.TaskTimeout
		}
		if flags.Ready != "" {
			cfg.CustomReady = flags.Ready
		}
		if flags.Working != "" {
			cfg.CustomWorking = flags.Working
		}
		if flags.Waiting != "" {
			cfg.CustomWaiting = flags.Waiting
		}
		if flags.Done != "" {
			cfg.CustomDone = flags.Done
		}
	}
	if titleCfg != nil {
		if value := cleanSymbol(titleCfg.Ready); value != "" {
			cfg.CustomReady = value
		}
		if value := cleanSymbol(titleCfg.Working); value != "" {
			cfg.CustomWorking = value
		}
		if value := cleanSymbol(titleCfg.Waiting); value != "" {
			cfg.CustomWaiting = value
		}
		if value := cleanSymbol(titleCfg.Done); value != "" {
			cfg.CustomDone = value
		}
	}

	// Flags and direct settings keys are already resolved above; do not parse command twice.
	cfg.Symbols = resolveStatusSymbols(cfg.UseTextStatus, cfg.CustomReady, cfg.CustomWorking, cfg.CustomWaiting, cfg.CustomDone)
	return cfg
}

func checkHelpOrVersion(args []string) (isHelp bool, isVersion bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		low := strings.ToLower(a)
		if low == "-h" || low == "--help" {
			return true, false
		}
		if low == "-v" || low == "--version" {
			return false, true
		}
		if low == "--prefix" || low == "--max-length" || low == "--task-timeout" ||
			low == "--ready" || low == "--ready-symbol" ||
			low == "--working" || low == "--working-symbol" ||
			low == "--waiting" || low == "--waiting-symbol" ||
			low == "--done" || low == "--done-symbol" {
			i++
		}
	}
	return false, false
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--animation-worker" {
		runAnimationWorker(os.Args[2:])
		return
	}
	if isHelp, isVersion := checkHelpOrVersion(os.Args[1:]); isHelp {
		printUsage()
		return
	} else if isVersion {
		fmt.Printf("antigravity-cli-title v%s\n", appVersion)
		return
	}

	if fi, err := os.Stdin.Stat(); err == nil {
		if (fi.Mode() & os.ModeCharDevice) != 0 {
			printUsage()
			return
		}
	}

	// 1. Title config and permissions extraction from settings.json
	homeDir, _ := os.UserHomeDir()
	settingsPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "settings.json")
	agyPid, agyTicks := getExactAncestorAgy(os.Getpid())
	settingsPath = callbackSettingsPath(settingsPath, agyPid)
	titleCfg, perms := readSettings(settingsPath)
	animationSettingsPath := ""
	// Hot reload belongs to agy callbacks, not independent manual invocations.
	if agyPid == 0 || !isTitleCommand(titleCfg) {
		titleCfg = nil
	} else {
		animationSettingsPath = settingsPath
	}

	// 2. Resolve runtime configuration with hot-reload priority
	runCfg := resolveRuntimeConfig(os.Args[1:], titleCfg)
	showPrefix := runCfg.ShowPrefix
	customPrefix := runCfg.CustomPrefix
	maxTitleLen := runCfg.MaxTitleLen
	taskTimeout := runCfg.TaskTimeout
	symbols := runCfg.Symbols

	// 3. Drain stdin immediately to release parent pipe without IPC blocking
	inputBytes, _ := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
	inputBytes = cleanStdin(inputBytes)
	if len(bytes.TrimSpace(inputBytes)) == 0 {
		cwd, _ := os.Getwd()
		cwdName := filepath.Base(cwd)
		pStr := ""
		if showPrefix {
			if cp := cleanTitleText(customPrefix); cp != "" {
				pStr = cp + " "
			}
		}
		fmt.Printf("%s%s %s\n", pStr, cleanTitleText(symbols.Ready), cleanTitleText(cwdName))
		return
	}

	var data TelemetryInput
	if err := json.Unmarshal(inputBytes, &data); err != nil {
		cwd, _ := os.Getwd()
		cwdName := filepath.Base(cwd)
		pStr := ""
		if showPrefix {
			if cp := cleanTitleText(customPrefix); cp != "" {
				pStr = cp + " "
			}
		}
		fmt.Printf("%s%s %s\n", pStr, cleanTitleText(symbols.Ready), cleanTitleText(cwdName))
		return
	}

	activeSubagents := 0
	for _, sub := range data.Subagents {
		st := strings.ToLower(strings.TrimSpace(sub.Status))
		if st != "" && st != "completed" && st != "failed" && st != "killed" && st != "error" && st != "cancelled" && st != "canceled" {
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

	title := ""
	hasHistory := false
	transcriptPath := ""
	convID := strings.TrimSpace(data.ConversationID)
	if !rxConvID.MatchString(convID) {
		convID = ""
	}

	if data.TranscriptPath != "" {
		if _, err := os.Stat(data.TranscriptPath); err == nil {
			transcriptPath = data.TranscriptPath
		}
	}
	if transcriptPath == "" && convID != "" && homeDir != "" {
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

	if title == "" && convID != "" && homeDir != "" {
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
	if convID != "" && homeDir != "" {
		_ = os.MkdirAll(targetMarkerDir, 0755)
		markerPath = filepath.Join(targetMarkerDir, fmt.Sprintf("marker_%s.txt", convID))
	}

	writeMarker := func(state string) {
		if agyPid > 0 && markerPath != "" {
			tmpPath := fmt.Sprintf("%s.tmp.%d", markerPath, os.Getpid())
			payload := []byte(fmt.Sprintf("%d:%d:%s", agyPid, agyTicks, state))
			if err := os.WriteFile(tmpPath, payload, 0644); err == nil {
				if err := os.Rename(tmpPath, markerPath); err != nil {
					_ = os.Remove(markerPath)
					_ = os.Rename(tmpPath, markerPath)
				}
			}
		}
	}

	var toolCalls []ToolCall
	if agentState == "tool_use" && transcriptPath != "" {
		if pr, err := parseTranscriptTail(transcriptPath); err == nil && pr != nil {
			toolCalls = pr.ToolCalls
		}
	}

	hasActiveBgTasks := false
	if activeTasks > 0 {
		hasActiveBgTasks = hasRecentlyActiveTasks(homeDir, convID, transcriptPath, taskTimeout)
	}

	markerExists := false
	markerContent := ""
	if markerPath != "" {
		if mf, err := readBounded(markerPath, 4096); err == nil {
			markerExists = true
			markerContent = string(mf)
		}
	}

	stateRes := resolveState(StateInput{
		AgentState:       agentState,
		ToolCalls:        toolCalls,
		Perms:            perms,
		AgyPid:           agyPid,
		CurrentPid:       os.Getpid(),
		ActiveTasks:      activeTasks,
		HasActiveBgTasks: hasActiveBgTasks,
		ActiveSubagents:  activeSubagents,
		HasHistory:       hasHistory,
		MarkerExists:     markerExists,
		MarkerContent:    markerContent,
		AgyTicks:         agyTicks,
		Symbols:          symbols,
		Cwd:              data.Cwd,
		HasWorkerFunc:    hasActiveWorker,
	})

	statusIndicator = stateRes.StatusIndicator
	if stateRes.MarkerAction != "" {
		writeMarker(stateRes.MarkerAction)
	}

	cleanTitle := cleanTitleText(title)
	runes := []rune(cleanTitle)
	if len(runes) > maxTitleLen {
		cleanTitle = string(runes[:maxTitleLen-3]) + "..."
	}

	prefixStr := ""
	if showPrefix {
		if cp := cleanTitleText(customPrefix); cp != "" {
			prefixStr = cp + " "
		}
	}

	staticTitle := prefixStr + cleanTitleText(statusIndicator) + " " + cleanTitle
	animate := animationEnabled(runCfg)
	updateAnimation(homeDir, agyPid, agyTicks, animationState{
		Animate:      animate && stateRes.IsWorking,
		SettingsPath: animationSettingsPath,
		Prefix:       prefixStr, Title: cleanTitle, StaticTitle: staticTitle,
	})
	fmt.Println(staticTitle)
}
