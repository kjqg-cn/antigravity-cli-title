package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReviewCommandMatching(t *testing.T) {
	for _, tc := range []struct {
		cmd, exe, running string
		want              bool
	}{
		{`git push origin main`, "powershell.exe", `powershell -Command "Write-Output 'git push origin main'"`, false},
		{`npm publish`, "rg", `rg "npm publish"`, false},
		{`Start-Sleep -Seconds 5; git push origin main`, "powershell", `powershell -Command "Start-Sleep -Seconds 5; git status"`, false},
		{`Start-Sleep -Seconds 5; git push origin main`, "powershell", `powershell -Command "Start-Sleep -Seconds 5; git push origin main"`, true},
		{`git status`, "git", "", false},
		{`node server.js --port=3001`, "node", `node server.js --port=3001`, true},
		{`git status`, "git", `C:\Git\git.exe status`, true},
		{`python C:\temp\new\runner.py`, "python", `python C:\temp\new\runner.py`, true},
	} {
		t.Run(tc.cmd+tc.exe+tc.running, func(t *testing.T) {
			if got := matchWorkerProcess(tc.exe, tc.running, parseWorkerTarget(tc.cmd)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	if _, ok := shellPayload(`bash --config=hello`); ok {
		t.Fatal("--config is not -c")
	}
	if got, ok := shellPayload(`bash --color -c "sleep 10"`); !ok || got != "sleep 10" {
		t.Fatal(got, ok)
	}
}

func TestReviewSettingsIndependentFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, append([]byte{239, 187, 191}, []byte(`{"title":{"command":"title --animate --text"},"permissions":"bad"}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, perms := readSettings(p)
	if cfg == nil || perms != nil || !parseCommandFlags(cfg.Command).Animate {
		t.Fatal(cfg, perms)
	}
	flags := resolveRuntimeConfig([]string{"--prefix=Bot", "--max-length=45", "--animate"}, nil)
	if flags.CustomPrefix != "Bot" || flags.MaxTitleLen != 45 || !flags.Animate {
		t.Fatal(flags)
	}
	for _, cfg := range []RuntimeConfig{
		resolveRuntimeConfig([]string{"--animate", "--text"}, nil),
		resolveRuntimeConfig([]string{"--animate", "--working", "Custom"}, nil),
	} {
		if cfg.Animate && !cfg.UseTextStatus && cfg.CustomWorking == "" {
			t.Fatal("static override lost", cfg)
		}
	}
}

func TestReviewTranscriptBoundsAndCompletedIDs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	content := `{"type":"SYSTEM_MESSAGE","content":"` + strings.Repeat("x", 40000) + `"}` + "\n" +
		`{"type":"USER_INPUT","content":"Real task"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","tool_calls":[{"id":"q","name":"ask_question"},{"id":"c","name":"run_command"}]}` + "\n" +
		`{"type":"GENERIC","tool_call_id":"q","content":"` + strings.Repeat("x", 150000) + `"}` + "\n"
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if title, ok := getFirstPromptFromTranscript(p, 35); !ok || title != "Real task" {
		t.Fatal(title, ok)
	}
	pr, err := parseTranscriptTail(p)
	if err != nil || pr == nil || len(pr.ToolCalls) != 1 || pr.ToolCalls[0].ID != "c" {
		t.Fatal(pr, err)
	}
}

func TestReviewStateSafety(t *testing.T) {
	symbols := resolveStatusSymbols(true, "", "", "", "")
	for _, st := range []string{"error", "cancelled", "canceled", "unknown"} {
		r := resolveState(StateInput{AgentState: st, HasHistory: true, MarkerExists: true, MarkerContent: "12:34:RUNNING", AgyPid: 12, AgyTicks: 34, Symbols: symbols})
		if r.StatusIndicator == symbols.Done {
			t.Fatal("unknown state marked done", st)
		}
	}
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	r := resolveState(StateInput{AgentState: "tool_use", Cwd: filepath.Join(root, "workspace"), ToolCalls: []ToolCall{{Name: "multi_replace_file_content", Args: map[string]interface{}{"TargetFile": filepath.Join(root, "outside.txt")}}}, Symbols: symbols})
	if r.StatusIndicator != symbols.Waiting {
		t.Fatal("outside multi_replace_file_content was missed", r)
	}
	if runtime.GOOS != "windows" && isPathInsideCwd("/home/User/project", "/home/user/project/file") {
		t.Fatal("case-sensitive paths confused")
	}
}

func TestAnimationFrames(t *testing.T) {
	s := animationState{Animate: true, Prefix: "agy: ", Title: "Task", StaticTitle: "agy: ✅ Task"}
	if s.frame(0) == s.frame(1) || s.frame(0) != s.frame(10) {
		t.Fatal("bad frame cycle")
	}
	s.Animate = false
	if s.frame(5) != s.StaticTitle {
		t.Fatal("static title not restored")
	}
	s.StaticTitle = "Ready\x1b\x07"
	if strings.ContainsAny(s.frame(0), "\x1b\x07") {
		t.Fatal("control characters leaked")
	}
}

func TestReviewRealWorkerDirectory(t *testing.T) {
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	dir := filepath.Join(root, "project")
	pat := parseWorkerTarget("cd " + dir + " && go test ./...")
	if !matchWorkerProcess("go", "go test ./...", pat, dir) {
		t.Fatal("actual process cwd was ignored")
	}
	if matchWorkerProcess("go", "go test ./...", pat, filepath.Join(root, "other")) {
		t.Fatal("wrong process cwd matched")
	}
	if nanos, ok := parseDarwinTime("四 10月/30 01:45:12 2026"); !ok || nanos < 1e18 {
		t.Fatal("localized timestamp fell back to pid", nanos, ok)
	}
}

func TestReviewRoundTwo(t *testing.T) {
	for _, tc := range []struct {
		target, exe, running string
		want                 bool
	}{
		{"go build ./... && go test ./...", "go.exe", "go test ./...", true},
		{"go build ./... && go test ./...", "powershell.exe", `powershell -Command "$ProgressPreference = 'SilentlyContinue'; go build ./...; go test ./..."`, true},
		{"go build ./... && go test ./...", "powershell.exe", `powershell -Command "go build ./...; git status"`, false},
	} {
		if got := matchWorkerProcess(tc.exe, tc.running, parseWorkerTarget(tc.target)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.running, got, tc.want)
		}
	}
	got := executableStatements(`Write-Output 'Remove-Item'; Get-ChildItem | Remove-Item; Remove-Item`)
	if len(got) != 3 || got[2] != "Remove-Item" {
		t.Fatal("repeated pipeline statement location", got)
	}
	got = executableStatements(`echo ready || git status`)
	if len(got) != 2 {
		t.Fatal("logical or mistaken for pipe", got)
	}
	if !animationEnabled(resolveRuntimeConfig([]string{"--animate", "--working", "   "}, nil)) {
		t.Fatal("blank custom symbol disabled animation")
	}
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = `D:\`
	}
	defaultPath := filepath.Join(root, "default.json")
	want := filepath.Join(root, "workspace", "custom.json")
	if got := settingsArgument(`agy --settings=custom.json`, filepath.Join(root, "workspace"), defaultPath); got != want {
		t.Fatal(got, want)
	}
	if runtime.GOOS == "windows" {
		if got := resolveWorkerDir(`D:\workspace`, `\subdir`); got != `D:\subdir` {
			t.Fatal("root-relative worker directory", got)
		}
	}
}

func TestReviewRoundThreeFlagsAndPriority(t *testing.T) {
	if got := resolveRuntimeConfig([]string{"--prefix", "--animate=true"}, nil).CustomPrefix; got != "--animate=true" {
		t.Fatal("flag-looking value was expanded", got)
	}
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"--animate=true"}, true},
		{[]string{"--animate=TRUE"}, true},
		{[]string{"--animate", "--animate=False"}, false},
		{[]string{"--ANIMATE=TrUe", "--animate=FALSE", "--animate=TRUE"}, true},
		{[]string{"--animate=true", "--animate=false"}, false},
		{[]string{"--animate=false", "--animate"}, true},
		{[]string{"--prefix", "--animate=true"}, false},
	} {
		if got := resolveRuntimeConfig(tc.args, nil).Animate; got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.args, got, tc.want)
		}
		if got := parseCommandFlags(strings.Join(tc.args, " ")).Animate; got != tc.want {
			t.Errorf("command %v: got %v, want %v", tc.args, got, tc.want)
		}
	}
	for _, tc := range []struct{ command, direct, want string }{
		{`title --working "Command Value"`, "", "Command Value"},
		{`title --working "Command Value"`, "Direct Value", "Direct Value"},
		{`title --working "Command Value"`, "   ", "Command Value"},
	} {
		cfg := resolveRuntimeConfig([]string{"--working", "CLI"}, &TitleConfig{Command: tc.command, Working: tc.direct})
		if cfg.Symbols.Working != tc.want {
			t.Errorf("direct=%q: got %q, want %q", tc.direct, cfg.Symbols.Working, tc.want)
		}
	}
}
