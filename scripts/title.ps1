# Set Output & Pipeline Encoding to UTF-8
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8

# CLI Arguments parsing
$SHOW_PREFIX = $true
$CUSTOM_PREFIX = "agy:"
$USE_TEXT_STATUS = $false
$MAX_TITLE_LEN = 35

$argIndex = 0
while ($argIndex -lt $args.Count) {
    $arg = if ($args[$argIndex]) { $args[$argIndex].ToString().ToLower() } else { "" }
    switch ($arg) {
        "--no-prefix" { $SHOW_PREFIX = $false; break }
        "--text"      { $USE_TEXT_STATUS = $true; break }
        "--classic"   { $USE_TEXT_STATUS = $true; break }
        "--max-length" {
            $argIndex++
            if ($argIndex -lt $args.Count) {
                $parsedLen = 0
                if ([int]::TryParse($args[$argIndex], [ref]$parsedLen) -and $parsedLen -gt 5) {
                    $MAX_TITLE_LEN = $parsedLen
                }
            }
            break
        }
        "--prefix" {
            $argIndex++
            if ($argIndex -lt $args.Count) {
                $CUSTOM_PREFIX = $args[$argIndex].ToString()
                $SHOW_PREFIX = $true
            }
            break
        }
    }
    $argIndex++
}

# Read JSON input from stdin
$inputJson = $input | Out-String
if ([string]::IsNullOrWhiteSpace($inputJson)) {
    $cwdName = Split-Path (Get-Location).Path -Leaf
    $initialInd = if ($USE_TEXT_STATUS) { "[READY]" } else { "⚪" }
    $prefixStr = if ($SHOW_PREFIX) { "$CUSTOM_PREFIX " } else { "" }
    [Console]::Out.WriteLine("${prefixStr}${initialInd} $cwdName")
    exit
}

try {
    $data = ConvertFrom-Json $inputJson -ErrorAction Stop
} catch {
    $cwdName = Split-Path (Get-Location).Path -Leaf
    $initialInd = if ($USE_TEXT_STATUS) { "[READY]" } else { "⚪" }
    $prefixStr = if ($SHOW_PREFIX) { "$CUSTOM_PREFIX " } else { "" }
    [Console]::Out.WriteLine("${prefixStr}${initialInd} $cwdName")
    exit
}

# Extract active subagents & tasks count
$activeSubagents = 0
if ($data.subagents) {
    try {
        $activeSubagents = @($data.subagents | Where-Object {
            $_ -and $_.status -and ($_.status.ToString().ToLower() -notin @("completed", "failed", "killed", "error", "cancelled"))
        }).Count
    } catch {}
}

$activeTasks = 0
if ($data.task_count -ne $null) {
    try {
        $activeTasks = [int]$data.task_count
    } catch {}
}

# 1. Native Fast Parent Process Lookup (Precise Window Isolation)
$csharpCode = @"
using System;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

public static class FastProcFinder {
    [StructLayout(LayoutKind.Sequential)]
    private struct PROCESS_BASIC_INFORMATION {
        public IntPtr ExitStatus;
        public IntPtr PebBaseAddress;
        public IntPtr AffinityMask;
        public IntPtr BasePriority;
        public UIntPtr UniqueProcessId;
        public UIntPtr InheritedFromUniqueProcessId;
    }

    [DllImport("ntdll.dll")]
    private static extern int NtQueryInformationProcess(
        IntPtr processHandle,
        int processInformationClass,
        ref PROCESS_BASIC_INFORMATION processInformation,
        int processInformationLength,
        out int returnLength);

    public static int GetParentPid(int pid) {
        try {
            using (var proc = Process.GetProcessById(pid)) {
                var pbi = new PROCESS_BASIC_INFORMATION();
                int retLen;
                int status = NtQueryInformationProcess(proc.Handle, 0, ref pbi, Marshal.SizeOf(pbi), out retLen);
                if (status == 0) {
                    return (int)pbi.InheritedFromUniqueProcessId.ToUInt32();
                }
            }
        } catch {}
        return 0;
    }

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_open_v2", CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_open_v2(string filename, out IntPtr db, int flags, IntPtr zVfs);

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_prepare_v2", CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_prepare_v2(IntPtr db, string zSql, int nByte, out IntPtr ppStmt, IntPtr pzTail);

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_step", CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_step(IntPtr pStmt);

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_column_text", CallingConvention = CallingConvention.Cdecl)]
    private static extern IntPtr sqlite3_column_text(IntPtr pStmt, int iCol);

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_finalize", CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_finalize(IntPtr pStmt);

    [DllImport("winsqlite3.dll", EntryPoint = "sqlite3_close", CallingConvention = CallingConvention.Cdecl)]
    private static extern int sqlite3_close(IntPtr db);

    public static string GetConversationTitle(string dbPath, string convId) {
        IntPtr db = IntPtr.Zero;
        IntPtr stmt = IntPtr.Zero;
        try {
            if (sqlite3_open_v2(dbPath, out db, 1, IntPtr.Zero) != 0) return null;
            string sql = "SELECT title FROM conversation_summaries WHERE conversation_id = '" + convId.Replace("'", "''") + "' LIMIT 1";
            if (sqlite3_prepare_v2(db, sql, -1, out stmt, IntPtr.Zero) != 0) return null;
            if (sqlite3_step(stmt) == 100) {
                IntPtr ptr = sqlite3_column_text(stmt, 0);
                if (ptr != IntPtr.Zero) {
                    int len = 0;
                    while (Marshal.ReadByte(ptr, len) != 0) len++;
                    byte[] bytes = new byte[len];
                    Marshal.Copy(ptr, bytes, 0, len);
                    return Encoding.UTF8.GetString(bytes);
                }
            }
        } catch {
        } finally {
            if (stmt != IntPtr.Zero) sqlite3_finalize(stmt);
            if (db != IntPtr.Zero) sqlite3_close(db);
        }
        return null;
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Auto)]
    private struct PROCESSENTRY32 {
        public uint dwSize;
        public uint cntUsage;
        public uint th32ProcessID;
        public IntPtr th32DefaultHeapID;
        public uint th32ModuleID;
        public uint cntThreads;
        public uint th32ParentProcessID;
        public int pcPriClassBase;
        public uint dwFlags;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 260)]
        public string szExeFile;
    }

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool ReadProcessMemory(
        IntPtr hProcess,
        IntPtr lpBaseAddress,
        [Out] byte[] lpBuffer,
        int dwSize,
        out IntPtr lpNumberOfBytesRead);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr OpenProcess(
        int dwDesiredAccess,
        bool bInheritHandle,
        int dwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    private static extern IntPtr CreateToolhelp32Snapshot(uint dwFlags, uint th32ProcessID);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    private static extern bool Process32First(IntPtr hSnapshot, ref PROCESSENTRY32 lppe);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    private static extern bool Process32Next(IntPtr hSnapshot, ref PROCESSENTRY32 lppe);

    private const int PROCESS_QUERY_INFORMATION = 0x0400;
    private const int PROCESS_VM_READ = 0x0010;

    public static string GetCommandLine(int pid) {
        IntPtr hProc = OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, false, pid);
        if (hProc == IntPtr.Zero) return "";
        try {
            PROCESS_BASIC_INFORMATION pbi = new PROCESS_BASIC_INFORMATION();
            int retLen;
            int status = NtQueryInformationProcess(hProc, 0, ref pbi, Marshal.SizeOf(pbi), out retLen);
            if (status != 0 || pbi.PebBaseAddress == IntPtr.Zero) return "";

            long procParams = 0;
            long bufferPtr = 0;
            short length = 0;

            if (IntPtr.Size == 8) {
                IntPtr ppOffset = new IntPtr(pbi.PebBaseAddress.ToInt64() + 0x20);
                byte[] addrBuf = new byte[8];
                IntPtr read;
                if (!ReadProcessMemory(hProc, ppOffset, addrBuf, 8, out read) || read.ToInt32() != 8) return "";
                procParams = BitConverter.ToInt64(addrBuf, 0);
                if (procParams == 0) return "";

                byte[] header = new byte[16];
                if (!ReadProcessMemory(hProc, new IntPtr(procParams + 0x70), header, 16, out read) || read.ToInt32() != 16) return "";
                length = BitConverter.ToInt16(header, 0);
                bufferPtr = BitConverter.ToInt64(header, 8);
            } else {
                IntPtr ppOffset = new IntPtr(pbi.PebBaseAddress.ToInt32() + 0x10);
                byte[] addrBuf = new byte[4];
                IntPtr read;
                if (!ReadProcessMemory(hProc, ppOffset, addrBuf, 4, out read) || read.ToInt32() != 4) return "";
                procParams = BitConverter.ToInt32(addrBuf, 0);
                if (procParams == 0) return "";

                byte[] header = new byte[8];
                if (!ReadProcessMemory(hProc, new IntPtr(procParams + 0x40), header, 8, out read) || read.ToInt32() != 8) return "";
                length = BitConverter.ToInt16(header, 0);
                bufferPtr = BitConverter.ToInt32(header, 4);
            }

            if (length <= 0 || bufferPtr == 0) return "";
            byte[] cmdBytes = new byte[length];
            IntPtr finalRead;
            if (!ReadProcessMemory(hProc, new IntPtr(bufferPtr), cmdBytes, length, out finalRead) || finalRead.ToInt32() != length) return "";
            return Encoding.Unicode.GetString(cmdBytes);
        } catch {
            return "";
        } finally {
            CloseHandle(hProc);
        }
    }

    public static bool HasActiveWorker(int agyPid, int currentPid) {
        if (agyPid <= 0) return false;

        var selfAncestors = new System.Collections.Generic.HashSet<int>();
        int cur = currentPid;
        for (int i = 0; i < 6; i++) {
            if (cur <= 0 || cur == agyPid) break;
            selfAncestors.Add(cur);
            cur = GetParentPid(cur);
        }

        IntPtr snap = CreateToolhelp32Snapshot(2, 0);
        if (snap == IntPtr.Zero || snap == new IntPtr(-1)) return false;

        try {
            PROCESSENTRY32 entry = new PROCESSENTRY32();
            entry.dwSize = (uint)Marshal.SizeOf(typeof(PROCESSENTRY32));
            if (Process32First(snap, ref entry)) {
                do {
                    if (entry.th32ParentProcessID == (uint)agyPid) {
                        int childPid = (int)entry.th32ProcessID;
                        if (selfAncestors.Contains(childPid)) continue;

                        string exe = (entry.szExeFile ?? "").ToLowerInvariant();
                        if (exe == "conhost.exe") continue;

                        string cmd = GetCommandLine(childPid).ToLowerInvariant();
                        if (cmd.Contains("statusline") || cmd.Contains("title.ps1") || cmd.Contains("title.sh") || cmd.Contains("conhost")) {
                            continue;
                        }

                        if (!string.IsNullOrEmpty(cmd)) {
                            return true;
                        }

                        // Fallback: if cmd is restricted, non-shell binaries represent active commands
                        if (exe != "cmd.exe" && exe != "powershell.exe" && exe != "pwsh.exe" && exe.Length > 0) {
                            return true;
                        }
                    }
                } while (Process32Next(snap, ref entry));
            }
        } finally {
            CloseHandle(snap);
        }
        return false;
    }
}
"@

if (-not ([System.Management.Automation.PSTypeName]'FastProcFinder').Type) {
    try {
        Add-Type -TypeDefinition $csharpCode -Language CSharp -ErrorAction SilentlyContinue
    } catch {}
}

function Get-ExactAncestorAgy {
    if (([System.Management.Automation.PSTypeName]'FastProcFinder').Type) {
        $cur = $PID
        for ($i = 0; $i -lt 5; $i++) {
            $parent = [FastProcFinder]::GetParentPid($cur)
            if ($parent -le 0) { break }
            try {
                $p = [System.Diagnostics.Process]::GetProcessById($parent)
                if ($p.ProcessName -eq "agy") { return $p }
            } catch { break }
            $cur = $parent
        }
    }

    # WMI fallback along ancestor chain
    $curPid = $PID
    for ($i = 0; $i -lt 4; $i++) {
        $p = Get-CimInstance Win32_Process -Filter "ProcessId = $curPid" -ErrorAction SilentlyContinue
        if (-not $p -or -not $p.ParentProcessId) { break }
        $parentPid = [int]$p.ParentProcessId
        $parentProc = Get-Process -Id $parentPid -ErrorAction SilentlyContinue
        if ($parentProc -and $parentProc.ProcessName -eq "agy") {
            return $parentProc
        }
        $curPid = $parentPid
    }
    return $null
}

$agyProc = Get-ExactAncestorAgy

# 2. Conversation & History Inspection
$title = ""
$hasHistory = $false
$transcriptPath = $null
$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$convId = if ($data.conversation_id) { $data.conversation_id.ToString().Trim() } else { "" }

# Robust transcript path detection (payload direct path -> cli brain -> standalone brain)
if ($data.transcript_path -and (Test-Path $data.transcript_path)) {
    $transcriptPath = $data.transcript_path.ToString().Trim()
} elseif ($convId) {
    $p1 = Join-Path $homeDir ".gemini\antigravity-cli\brain\$convId\.system_generated\logs\transcript.jsonl"
    $p2 = Join-Path $homeDir ".gemini\antigravity\brain\$convId\.system_generated\logs\transcript.jsonl"
    if (Test-Path $p1) { $transcriptPath = $p1 }
    elseif (Test-Path $p2) { $transcriptPath = $p2 }
    else { $transcriptPath = $p1 }
}

if ($data.conversation_title) {
    $candidateTitle = $data.conversation_title.ToString().Trim()
    if ($candidateTitle) {
        $title = $candidateTitle
        $hasHistory = $true
    }
}

# 2.1 Query conversation_summaries.db (matches /resume title)
if (-not $title -and $convId -and ([System.Management.Automation.PSTypeName]'FastProcFinder').Type) {
    $dbPath1 = Join-Path $homeDir ".gemini\antigravity-cli\conversation_summaries.db"
    $dbPath2 = Join-Path $homeDir ".gemini\antigravity\conversation_summaries.db"
    $dbPath = if (Test-Path $dbPath1) { $dbPath1 } elseif (Test-Path $dbPath2) { $dbPath2 } else { $null }
    if ($dbPath) {
        try {
            $summaryTitle = [FastProcFinder]::GetConversationTitle($dbPath, $convId)
            if ($summaryTitle -and $summaryTitle.Trim().Length -gt 0) {
                $title = $summaryTitle.Trim()
                $hasHistory = $true
            }
        } catch {}
    }
}

# 2.2 Inspect transcript for actual user conversation requests (fallback when no summary title yet)
if ($transcriptPath -and (Test-Path $transcriptPath)) {
    try {
        $stream = [System.IO.File]::Open($transcriptPath, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
        $reader = [System.IO.StreamReader]::new($stream, [System.Text.Encoding]::UTF8)
        $count = 0
        while ((-not $reader.EndOfStream) -and ($count -lt 50)) {
            $count++
            $line = $reader.ReadLine()
            if ($line -match '"type"\s*:\s*"USER_INPUT"') {
                $hasHistory = $true
                if (-not $title) {
                    $step = ConvertFrom-Json $line -ErrorAction SilentlyContinue
                    if ($step -and $step.content) {
                        $raw = $step.content.ToString()
                        if ($raw -match '(?s)<USER_REQUEST>(.*?)</USER_REQUEST>') {
                            $raw = $matches[1]
                        }
                        # Wrap in @() so single-line strings do not unwrap into char indexing
                        $lines = @($raw -split "[\r\n]+" | Where-Object { $_.Trim().Length -gt 0 })
                        if ($lines -and $lines.Count -gt 0) {
                            $firstLine = ($lines[0].ToString().Trim() -replace '^#+\s*', '').Trim()
                            if ($firstLine.Length -gt $MAX_TITLE_LEN) {
                                $title = $firstLine.Substring(0, [Math]::Max(1, $MAX_TITLE_LEN - 3)) + "..."
                            } else {
                                $title = $firstLine
                            }
                        }
                    }
                }
                break
            }
        }
        $reader.Close()
        $stream.Close()
    } catch {}
}

# Fallback to directory name
if (-not $title) {
    if ($data.cwd) {
        $title = Split-Path $data.cwd -Leaf
    } else {
        $title = Split-Path (Get-Location).Path -Leaf
    }
}

# 3. Determine State: ⚪ Ready / ⚙️ Working / ⚠️ Waiting / ✅ Done
$statusIndicator = if ($USE_TEXT_STATUS) { "[READY]" } else { "⚪" }
$agentState = if ($data.agent_state) { $data.agent_state.ToString().ToLower().Trim() } else { "idle" }
$targetMarkerDir = Join-Path $homeDir ".gemini\antigravity-cli-title"
if ($convId -and -not (Test-Path $targetMarkerDir)) {
    try { [System.IO.Directory]::CreateDirectory($targetMarkerDir) | Out-Null } catch {}
}
$markerPath = if ($convId) { Join-Path $targetMarkerDir "marker_$convId.txt" } else { $null }

switch ($agentState) {
    "thinking" {
        $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
        if ($agyProc -and $markerPath) {
            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
        }
        break
    }
    "working" {
        $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
        if ($agyProc -and $markerPath) {
            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
        }
        break
    }
    "waiting" {
        $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
        if ($agyProc -and $markerPath) {
            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
        }
        break
    }
    "paused" {
        $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
        if ($agyProc -and $markerPath) {
            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
        }
        break
    }
    "tool_use" {
        $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
        if ($agyProc -and $markerPath) {
            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
        }
        try {
            if ($transcriptPath -and (Test-Path $transcriptPath)) {
                $fs = [System.IO.File]::Open($transcriptPath, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
                $len = $fs.Length
                $readLen = [Math]::Min(16384, $len)
                if ($readLen -gt 0) {
                    $fs.Seek(-$readLen, [System.IO.SeekOrigin]::End) | Out-Null
                    $buf = New-Object byte[] $readLen
                    $actualRead = $fs.Read($buf, 0, $readLen)
                    $fs.Close()
                    $tailText = [System.Text.Encoding]::UTF8.GetString($buf, 0, $actualRead)
                    $tailLines = @($tailText -split "[\r\n]+" | Where-Object { $_.Trim().Length -gt 0 })
                    
                    $lastStep = $null
                    for ($i = $tailLines.Count - 1; $i -ge 0; $i--) {
                        $rawLine = ($tailLines[$i].ToString().Trim() -replace '^\uFEFF', '').Trim()
                        if ($rawLine.StartsWith("{") -and $rawLine.EndsWith("}")) {
                            $parsed = ConvertFrom-Json $rawLine -ErrorAction SilentlyContinue
                            if ($parsed -and $parsed.type) {
                                $lastStep = $parsed
                                break
                            }
                        }
                    }
                    
                    # If background tasks or subagents are running, keep WORKING unless actively prompting with ask_question
                    if ($activeTasks -gt 0 -or $activeSubagents -gt 0) {
                        $isAskPrompt = $false
                        if ($lastStep -and $lastStep.type -eq "PLANNER_RESPONSE" -and $lastStep.tool_calls -and $lastStep.tool_calls.Count -gt 0) {
                            $tName = if ($lastStep.tool_calls[0].name) { $lastStep.tool_calls[0].name.ToString() } else { "" }
                            if ($tName -in @("ask_question", "ask", "prompt_user", "get_confirmation", "request_feedback")) {
                                $isAskPrompt = $true
                            }
                        }
                        if (-not $isAskPrompt) {
                            break
                        }
                    }

                    if ($lastStep -and $lastStep.type -eq "PLANNER_RESPONSE" -and $lastStep.tool_calls -and $lastStep.tool_calls.Count -gt 0) {
                        $toolCall = $lastStep.tool_calls[0]
                        $toolName = if ($toolCall.name) { $toolCall.name.ToString() } else { "" }
                        
                        if ($toolName -in @("ask_question", "ask", "prompt_user", "get_confirmation", "request_feedback")) {
                            $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
                        } else {
                            $settingsPath = Join-Path $homeDir ".gemini\antigravity-cli\settings.json"
                            if (Test-Path $settingsPath) {
                                $settingsObj = Get-Content $settingsPath -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction SilentlyContinue
                                if ($settingsObj -and $settingsObj.permissions) {
                                    $perms = $settingsObj.permissions
                                    if ($toolName -eq "run_command" -and $toolCall.args -and $toolCall.args.CommandLine) {
                                        $cmd = $toolCall.args.CommandLine.ToString()
                                        $isAsk = $false
                                        if ($perms.ask) {
                                            foreach ($rule in $perms.ask) {
                                                if ($rule -match '^command\(regex:(.*)\)$') {
                                                    try {
                                                        if ($cmd -match $matches[1]) { $isAsk = $true; break }
                                                    } catch {}
                                                }
                                            }
                                        }
                                        if ($isAsk) {
                                            $hasWorker = $false
                                            if ($agyProc -and ([System.Management.Automation.PSTypeName]'FastProcFinder').Type) {
                                                try { $hasWorker = [FastProcFinder]::HasActiveWorker($agyProc.Id, $PID) } catch {}
                                            }
                                            if ($hasWorker) {
                                                $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
                                            } else {
                                                $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
                                            }
                                        } else {
                                            $isAllow = $false
                                            if ($perms.allow) {
                                                foreach ($rule in $perms.allow) {
                                                    if ($rule -eq "command(*)" -or $rule -eq "*") { $isAllow = $true; break }
                                                    if ($rule -match '^command\(regex:(.*)\)$') {
                                                        try {
                                                            if ($cmd -match $matches[1]) { $isAllow = $true; break }
                                                        } catch {}
                                                    }
                                                }
                                            }
                                            if (-not $isAllow) {
                                                $hasWorker = $false
                                                if ($agyProc -and ([System.Management.Automation.PSTypeName]'FastProcFinder').Type) {
                                                    try { $hasWorker = [FastProcFinder]::HasActiveWorker($agyProc.Id, $PID) } catch {}
                                                }
                                                if ($hasWorker) {
                                                    $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
                                                } else {
                                                    $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
                                                }
                                            }
                                        }
                                    } elseif ($toolName -in @("read_file", "write_file", "edit_file") -and $toolCall.args -and $toolCall.args.Path) {
                                        $fPath = $toolCall.args.Path.ToString()
                                        if ($perms.ask) {
                                            foreach ($rule in $perms.ask) {
                                                if ($rule -match '^' + $toolName + '\((.*)\)$') {
                                                    $pat = [regex]::Escape($matches[1]) -replace '\\\*', '.*'
                                                    if ($fPath -match $pat) { 
                                                        $statusIndicator = if ($USE_TEXT_STATUS) { "[WAITING]" } else { "⚠️" }
                                                        break 
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                } else {
                    $fs.Close()
                }
            }
        } catch {}
        break
    }
    default { # Covers 'idle' and any unknown inactive state
        if ($activeSubagents -gt 0 -or $activeTasks -gt 0) {
            $statusIndicator = if ($USE_TEXT_STATUS) { "[WORKING]" } else { "⚙️" }
            if ($agyProc -and $markerPath) {
                try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):RUNNING", [System.Text.Encoding]::UTF8) } catch {}
            }
        } else {
            $statusIndicator = if ($USE_TEXT_STATUS) { "[READY]" } else { "⚪" }
            if ($hasHistory -and $convId -and $markerPath -and (Test-Path $markerPath)) {
                try {
                    $content = (Get-Content $markerPath -Raw -ErrorAction SilentlyContinue).Trim()
                    $parts = $content -split ":"
                    if ($parts.Count -ge 3 -and $agyProc) {
                        $savedPid = $parts[0]
                        $savedTicks = $parts[1]
                        if ($agyProc.Id.ToString() -eq $savedPid -and $agyProc.StartTime.Ticks.ToString() -eq $savedTicks) {
                            $statusIndicator = if ($USE_TEXT_STATUS) { "[DONE]" } else { "✅" }
                            try { [System.IO.File]::WriteAllText($markerPath, "$($agyProc.Id):$($agyProc.StartTime.Ticks):COMPLETED", [System.Text.Encoding]::UTF8) } catch {}
                        }
                    }
                } catch {}
            }
        }
        break
    }
}

# Clean title text & length guard
$cleanTitle = ($title -replace '[\r\n\t]+', ' ').Trim()
if ($cleanTitle.Length -gt $MAX_TITLE_LEN) {
    $cleanTitle = $cleanTitle.Substring(0, [Math]::Max(1, $MAX_TITLE_LEN - 3)) + "..."
}

# Prefix formatting
$prefixStr = if ($SHOW_PREFIX) { "$CUSTOM_PREFIX " } else { "" }

# Write final title string to stdout
[Console]::Out.WriteLine("${prefixStr}${statusIndicator} $cleanTitle")
