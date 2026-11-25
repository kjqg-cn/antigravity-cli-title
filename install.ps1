# install.ps1 - Native Installer for Antigravity CLI Window Title (Windows Native Engine)
param(
    [switch]$NoPrefix,
    [switch]$ShowPrefix,
    [switch]$Animate,
    [switch]$NoAnimate,
    [switch]$Text,
    [string]$Prefix = "agy:",
    [int]$MaxLength = 35,
    [int]$TaskTimeout = 60,
    [string]$ReadySymbol = "",
    [string]$WorkingSymbol = "",
    [string]$WaitingSymbol = "",
    [string]$DoneSymbol = "",
    [switch]$WTProfile
)

$callArgs = @{}
foreach ($k in $PSBoundParameters.Keys) { $callArgs[$k] = $PSBoundParameters[$k] }
$callArgs['InvokedScriptRoot'] = $PSScriptRoot
$callArgs['InvokedCommandPath'] = $PSCommandPath

& {
    [CmdletBinding()]
    param(
        [switch]$NoPrefix,
        [switch]$ShowPrefix,
        [switch]$Animate,
        [switch]$NoAnimate,
        [switch]$Text,
        [string]$Prefix = "agy:",
        [int]$MaxLength = 35,
        [int]$TaskTimeout = 60,
        [string]$ReadySymbol = "",
        [string]$WorkingSymbol = "",
        [string]$WaitingSymbol = "",
        [string]$DoneSymbol = "",
        [switch]$WTProfile,
        [string]$InvokedScriptRoot = "",
        [string]$InvokedCommandPath = ""
    )

    $origOutputEncoding = $OutputEncoding
    $origConsoleOutputEncoding = [Console]::OutputEncoding
    try {
        $OutputEncoding = [System.Text.Encoding]::UTF8
        [Console]::OutputEncoding = [System.Text.Encoding]::UTF8

    Write-Host "==========================================================" -ForegroundColor Cyan
$eRocket   = [char]::ConvertFromUtf32(0x1F680)
$eSparkles = [char]::ConvertFromUtf32(0x2728)
$eReady    = [char]::ConvertFromUtf32(0x26AA)
$eWorking  = [char]::ConvertFromUtf32(0x2699) + [char]0xFE0F
$eWaiting  = [char]::ConvertFromUtf32(0x26A0) + [char]0xFE0F
$eDone     = [char]::ConvertFromUtf32(0x2705)

Write-Host "   $eRocket Antigravity CLI Window Title Installer (Windows Native)    " -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan

$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"
if (-not (Test-Path $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    Write-Host "[+] Created target directory: $targetDir" -ForegroundColor Gray
}

$effectiveScriptRoot = if ($InvokedScriptRoot) { $InvokedScriptRoot } elseif ($PSScriptRoot) { $PSScriptRoot } else { "" }
$isRemotePipeline = [string]::IsNullOrEmpty($effectiveScriptRoot)

# 1. Install uninstaller (uninstall.ps1)
$repoBaseUrl = "https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main"
$destUninstall = Join-Path $targetDir "uninstall.ps1"
$hasUninstaller = $false

if (-not $isRemotePipeline) {
    $localUninstall = Join-Path $effectiveScriptRoot "uninstall.ps1"
    if (Test-Path $localUninstall) {
        if ([System.IO.Path]::GetFullPath($localUninstall) -ne [System.IO.Path]::GetFullPath($destUninstall)) {
            Copy-Item -Path $localUninstall -Destination $destUninstall -Force
        }
        Write-Host "[+] Installed uninstaller to: $destUninstall" -ForegroundColor Green
        $hasUninstaller = $true
    }
}

if (-not $hasUninstaller) {
    Write-Host "[*] Fetching uninstaller from GitHub..." -ForegroundColor Gray
    try {
        Invoke-RestMethod -Uri "$repoBaseUrl/uninstall.ps1" -OutFile $destUninstall
        Write-Host "[+] Downloaded uninstaller to: $destUninstall" -ForegroundColor Green
    } catch {
        Write-Warning "[-] Could not fetch uninstall.ps1 from remote: $_"
    }
}

# 2. Install Native Fast Execution Engine (title.exe)
$destExe = Join-Path $targetDir "title.exe"
$hasNativeExe = $false

if (-not $isRemotePipeline) {
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    $goSourceDir = Join-Path $effectiveScriptRoot "cmd\title"
    if ($goCmd -and (Test-Path $goSourceDir)) {
        Write-Host "[*] Compiling native execution engine (title.exe) via local Go..." -ForegroundColor Gray
        Push-Location $effectiveScriptRoot
        try {
            & go build -ldflags="-s -w" -o $destExe "./cmd/title"
            if ($LASTEXITCODE -eq 0 -and (Test-Path $destExe)) {
                Write-Host "[+] Compiled native execution engine: $destExe" -ForegroundColor Green
                $hasNativeExe = $true
            }
        } catch {}
        finally {
            Pop-Location
        }
    }
    if (-not $hasNativeExe) {
        $localExe = Join-Path $effectiveScriptRoot "title.exe"
        if (Test-Path $localExe) {
            if ([System.IO.Path]::GetFullPath($localExe) -ne [System.IO.Path]::GetFullPath($destExe)) {
                Copy-Item -Path $localExe -Destination $destExe -Force
            }
            Write-Host "[+] Installed native fast execution engine: $destExe" -ForegroundColor Green
            $hasNativeExe = $true
        }
    }
}

if (-not $hasNativeExe) {
    try {
        # Keep the release-compatible Windows asset name; local Go builds use the native architecture.
        $releaseExeUrl = "https://github.com/kjqg-cn/antigravity-cli-title/releases/latest/download/title.exe"
        Write-Host "[*] Downloading pre-built native execution engine (title.exe)..." -ForegroundColor Gray
        Invoke-RestMethod -Uri $releaseExeUrl -OutFile $destExe -TimeoutSec 15 -ErrorAction Stop
        if ((Test-Path $destExe) -and (Get-Item $destExe).Length -gt 100000) {
            Write-Host "[+] Downloaded native fast execution engine: $destExe" -ForegroundColor Green
            $hasNativeExe = $true
        } else {
            if (Test-Path $destExe) { Remove-Item $destExe -Force }
        }
    } catch {
        $errMsg = "[-] Failed to obtain title.exe: $_`nPlease install Go (https://go.dev) or compile manually: go build -o title.exe ./cmd/title"
        if ($isRemotePipeline) {
            throw $errMsg
        } else {
            Write-Error $errMsg
            exit 1
        }
    }
}

# 3. Install Antigravity Icon Assets
$assetsDir = Join-Path $targetDir "assets"
if (-not (Test-Path $assetsDir)) { New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null }
$hasAssets = $false

if (-not $isRemotePipeline) {
    $localAssets = Join-Path $effectiveScriptRoot "assets"
    if (Test-Path $localAssets) {
        if ([System.IO.Path]::GetFullPath($localAssets) -ne [System.IO.Path]::GetFullPath($assetsDir)) {
            Copy-Item -Path "$localAssets\*" -Destination $assetsDir -Force -Recurse
        }
        Write-Host "[+] Installed Antigravity icon assets to: $assetsDir" -ForegroundColor Green
        $hasAssets = $true
    }
}

if (-not $hasAssets) {
    try {
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.png" -OutFile (Join-Path $assetsDir "antigravity.png")
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.ico" -OutFile (Join-Path $assetsDir "antigravity.ico")
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.svg" -OutFile (Join-Path $assetsDir "antigravity.svg")
        Write-Host "[+] Downloaded Antigravity icon assets to: $assetsDir" -ForegroundColor Green
    } catch {}
}

function Clean-SymbolValue([string]$val) {
    if (-not $val) { return "" }
    $trimmed = $val.Trim()
    if (($trimmed.StartsWith('"') -and $trimmed.EndsWith('"')) -or ($trimmed.StartsWith("'") -and $trimmed.EndsWith("'"))) {
        if ($trimmed.Length -ge 2) {
            $trimmed = $trimmed.Substring(1, $trimmed.Length - 2).Trim()
        }
    }
    return $trimmed
}

function Format-SymbolArg([string]$flag, [string]$val) {
    if (-not $val) { return $null }
    $clean = Clean-SymbolValue $val
    if (-not $clean) { return $null }
        $escaped = $clean.Replace('\', '\\').Replace('"', '\"')
    return "$flag `"$escaped`""
}

function Split-TitleCommand([string]$command) {
    $tokens = [System.Collections.Generic.List[string]]::new()
    $current = [System.Text.StringBuilder]::new()
    $quote = [char]0
    $hasToken = $false
    for ($i = 0; $i -lt $command.Length; $i++) {
        $c = $command[$i]
        if ($quote -ne [char]0) {
            if ($c -eq [char]92 -and $i + 1 -lt $command.Length -and ($command[$i+1] -eq $quote -or $command[$i+1] -eq [char]92)) {
                $i++
                [void]$current.Append($command[$i])
            } elseif ($c -eq $quote) { $quote = [char]0 }
            else { [void]$current.Append($c) }
            continue
        }
        if ($c -eq [char]34 -or $c -eq [char]39) { $quote = $c; $hasToken = $true; continue }
        if ([char]::IsWhiteSpace($c)) {
            if ($hasToken) { $tokens.Add($current.ToString()); [void]$current.Clear(); $hasToken = $false }
            continue
        }
        [void]$current.Append($c)
        $hasToken = $true
    }
    if ($hasToken) { $tokens.Add($current.ToString()) }
    foreach ($token in $tokens) {
        if ($token -match '^(--(?:prefix|max-length|task-timeout|ready(?:-symbol)?|working(?:-symbol)?|waiting(?:-symbol)?|done(?:-symbol)?))=(.*)$') {
            $Matches[1]; $Matches[2]
        } else { $token }
    }
}

function Get-FileEncodingAndContent([byte[]]$bytes) {
    if ($null -eq $bytes -or $bytes.Length -eq 0) {
        return @{
            Encoding = (New-Object System.Text.UTF8Encoding($false))
            Content  = ""
        }
    }
    # 1. UTF-8 BOM (EF BB BF)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        return @{
            Encoding = [System.Text.Encoding]::UTF8
            Content  = [System.Text.Encoding]::UTF8.GetString($bytes, 3, $bytes.Length - 3)
        }
    }
    # 2. UTF-16 LE BOM (FF FE)
    if ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) {
        return @{
            Encoding = [System.Text.Encoding]::Unicode
            Content  = [System.Text.Encoding]::Unicode.GetString($bytes, 2, $bytes.Length - 2)
        }
    }
    # 3. UTF-16 BE BOM (FE FF)
    if ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF) {
        return @{
            Encoding = [System.Text.Encoding]::BigEndianUnicode
            Content  = [System.Text.Encoding]::BigEndianUnicode.GetString($bytes, 2, $bytes.Length - 2)
        }
    }
    # 4. Strict UTF-8 without BOM
    $strictUtf8 = New-Object System.Text.UTF8Encoding($false, $true)
    try {
        return @{
            Encoding = (New-Object System.Text.UTF8Encoding($false))
            Content  = $strictUtf8.GetString($bytes)
        }
    } catch {
        # 5. Fallback to system ANSI/GBK code page
        $ansi = try {
            [System.Text.Encoding]::GetEncoding(0)
        } catch {
            try {
                [System.Text.Encoding]::GetEncoding([System.Globalization.CultureInfo]::CurrentCulture.TextInfo.ANSICodePage)
            } catch {
                [System.Text.Encoding]::Default
            }
        }
        return @{
            Encoding = $ansi
            Content  = $ansi.GetString($bytes)
        }
    }
}

function ConvertFrom-JsonC([string]$jsonc) {
    $sb = [System.Text.StringBuilder]::new()
    $inString = $false
    $escape = $false
    $len = $jsonc.Length
    for ($i = 0; $i -lt $len; $i++) {
        $c = $jsonc[$i]
        if ($inString) {
            [void]$sb.Append($c)
            if ($escape) {
                $escape = $false
            } elseif ($c -eq [char]92) {
                $escape = $true
            } elseif ($c -eq [char]34) {
                $inString = $false
            }
            continue
        }
        if ($c -eq [char]34) {
            $inString = $true
            [void]$sb.Append($c)
            continue
        }
        if ($c -eq '/' -and $i + 1 -lt $len -and $jsonc[$i + 1] -eq '/') {
            $i += 2
            while ($i -lt $len -and $jsonc[$i] -ne "`n") { $i++ }
            if ($i -lt $len) { [void]$sb.Append($jsonc[$i]) }
            continue
        }
        if ($c -eq '/' -and $i + 1 -lt $len -and $jsonc[$i + 1] -eq '*') {
            $i += 2
            while ($i + 1 -lt $len -and -not ($jsonc[$i] -eq '*' -and $jsonc[$i + 1] -eq '/')) { $i++ }
            $i++
            continue
        }
        [void]$sb.Append($c)
    }
    $clean = $sb.ToString()
    $out = [System.Text.StringBuilder]::new()
    $inString = $false
    $escape = $false
    for ($i = 0; $i -lt $clean.Length; $i++) {
        $c = $clean[$i]
        if ($inString) {
            [void]$out.Append($c)
            if ($escape) { $escape = $false }
            elseif ($c -eq [char]92) { $escape = $true }
            elseif ($c -eq [char]34) { $inString = $false }
            continue
        }
        if ($c -eq [char]34) { $inString = $true }
        if ($c -eq ',') {
            $j = $i + 1
            while ($j -lt $clean.Length -and [char]::IsWhiteSpace($clean[$j])) { $j++ }
            if ($j -lt $clean.Length -and ($clean[$j] -eq '}' -or $clean[$j] -eq ']')) { continue }
        }
        [void]$out.Append($c)
    }
    $noTrailing = $out.ToString()
    return ConvertFrom-Json $noTrailing -ErrorAction Stop
}

function Test-HasJsonComments([string]$jsonc) {
    $inString = $false
    $escape = $false
    $len = $jsonc.Length
    for ($i = 0; $i -lt $len; $i++) {
        $c = $jsonc[$i]
        if ($inString) {
            if ($escape) {
                $escape = $false
            } elseif ($c -eq [char]92) {
                $escape = $true
            } elseif ($c -eq [char]34) {
                $inString = $false
            }
            continue
        }
        if ($c -eq [char]34) {
            $inString = $true
            continue
        }
        if ($c -eq '/' -and $i + 1 -lt $len) {
            $next = $jsonc[$i + 1]
            if ($next -eq '/' -or $next -eq '*') {
                return $true
            }
        }
    }
    return $false
}

# 4. Read settings.json and inherit existing flags on upgrade
$settingsDir = Join-Path $homeDir ".gemini\antigravity-cli"
$settingsPath = Join-Path $settingsDir "settings.json"
if (-not (Test-Path $settingsDir)) {
    New-Item -ItemType Directory -Path $settingsDir -Force | Out-Null
}

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$ansiEncoding = try {
    [System.Text.Encoding]::GetEncoding(0)
} catch {
    try {
        [System.Text.Encoding]::GetEncoding([System.Globalization.CultureInfo]::CurrentCulture.TextInfo.ANSICodePage)
    } catch {
        [System.Text.Encoding]::Default
    }
}
$settingsObj = [ordered]@{}
$raw = ""
$existingCmd = ""

if (Test-Path $settingsPath) {
    $raw = (Get-FileEncodingAndContent ([System.IO.File]::ReadAllBytes($settingsPath))).Content
    if (-not [string]::IsNullOrWhiteSpace($raw)) {
        if ($raw.Trim().StartsWith('[')) {
            $errMsg = "[-] Root element in $settingsPath is not an object (expected JSON object).`nAborting to prevent corrupting settings.json."
            if ($isRemotePipeline) {
                throw $errMsg
            } else {
                Write-Error $errMsg
                exit 1
            }
        }
        try {
            $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop
        } catch {
            $errMsg = "[-] Failed to parse existing $settingsPath as valid JSON: $_`nAborting to prevent corrupting settings.json."
            if ($isRemotePipeline) {
                throw $errMsg
            } else {
                Write-Error $errMsg
                exit 1
            }
        }
        if ($settingsObj -is [System.Array] -or ($settingsObj -isnot [System.Management.Automation.PSCustomObject] -and $settingsObj -isnot [System.Collections.IDictionary])) {
            $errMsg = "[-] Root element in $settingsPath is not an object (expected JSON object).`nAborting to prevent corrupting settings.json."
            if ($isRemotePipeline) {
                throw $errMsg
            } else {
                Write-Error $errMsg
                exit 1
            }
        }
        if ($settingsObj -and $settingsObj.PSObject.Properties['title'] -and $settingsObj.title) {
            if ($settingsObj.title.PSObject.Properties['command']) {
                $existingCmd = [string]$settingsObj.title.command
            } elseif ($settingsObj.title -is [string]) {
                $existingCmd = [string]$settingsObj.title
            }
            if ($settingsObj.title.PSObject.Properties['ready'] -and -not $PSBoundParameters.ContainsKey('ReadySymbol')) {
                $ReadySymbol = [string]$settingsObj.title.ready
            }
            if ($settingsObj.title.PSObject.Properties['working'] -and -not $PSBoundParameters.ContainsKey('WorkingSymbol')) {
                $WorkingSymbol = [string]$settingsObj.title.working
            }
            if ($settingsObj.title.PSObject.Properties['waiting'] -and -not $PSBoundParameters.ContainsKey('WaitingSymbol')) {
                $WaitingSymbol = [string]$settingsObj.title.waiting
            }
            if ($settingsObj.title.PSObject.Properties['done'] -and -not $PSBoundParameters.ContainsKey('DoneSymbol')) {
                $DoneSymbol = [string]$settingsObj.title.done
            }
        }
    }

    # Backup original only after validating it is valid JSON and not an array
    $bakPath = Join-Path $settingsDir "settings.json.pre-title.bak"
    if (-not (Test-Path $bakPath)) {
        if ($raw -and $raw -notmatch 'antigravity-cli-title') {
            [System.IO.File]::WriteAllText($bakPath, $raw, $utf8NoBom)
            Write-Host "[+] Initial backup saved to: $bakPath" -ForegroundColor Gray
        }
    }
}

if ($existingCmd -and $existingCmd -match 'antigravity-cli-title') {
    $toks = @(Split-TitleCommand $existingCmd)

    for ($idx = 1; $idx -lt $toks.Count; $idx++) {
        $t = $toks[$idx]
        # Always consume a value, even when an explicit installer parameter overrides it.
        $hasValue = $false
        $flagValue = $null
        if ($t -match '^--(prefix|max-length|task-timeout|ready(-symbol)?|working(-symbol)?|waiting(-symbol)?|done(-symbol)?)$' -and $idx + 1 -lt $toks.Count) {
            $flagValue = $toks[++$idx]
            $hasValue = $true
        }
        switch -Regex ($t) {
            '^--animate(=true)?$' { if (-not $PSBoundParameters.ContainsKey('Animate') -and -not $NoAnimate) { $Animate = [switch]::Present } }
            '^(--no-animate|--animate=false)$' { if (-not $PSBoundParameters.ContainsKey('Animate') -and -not $NoAnimate) { $Animate = [switch]$false } }
            '^--no-prefix$' {
                if (-not $PSBoundParameters.ContainsKey('NoPrefix') -and -not $PSBoundParameters.ContainsKey('Prefix') -and -not $ShowPrefix) {
                    $NoPrefix = [switch]::Present
                }
            }
            '^(--text|--classic)$' {
                if (-not $PSBoundParameters.ContainsKey('Text')) {
                    $Text = [switch]::Present
                }
            }
            '^--prefix$' {
                if (-not $PSBoundParameters.ContainsKey('Prefix') -and $hasValue) {
                    $Prefix = $flagValue
                }
            }
            '^--max-length$' {
                if (-not $PSBoundParameters.ContainsKey('MaxLength') -and $hasValue) {
                    $val = 0
                    if ([int]::TryParse($flagValue, [ref]$val) -and $val -gt 5) {
                        $MaxLength = $val
                    }
                }
            }
            '^--task-timeout$' {
                if (-not $PSBoundParameters.ContainsKey('TaskTimeout') -and $hasValue) {
                    $val = 0
                    if ([int]::TryParse($flagValue, [ref]$val) -and $val -gt 0) {
                        $TaskTimeout = $val
                    }
                }
            }
            '^(--ready|--ready-symbol)$' {
                if (-not $PSBoundParameters.ContainsKey('ReadySymbol') -and $hasValue) {
                    $ReadySymbol = $flagValue
                }
            }
            '^(--working|--working-symbol)$' {
                if (-not $PSBoundParameters.ContainsKey('WorkingSymbol') -and $hasValue) {
                    $WorkingSymbol = $flagValue
                }
            }
            '^(--waiting|--waiting-symbol)$' {
                if (-not $PSBoundParameters.ContainsKey('WaitingSymbol') -and $hasValue) {
                    $WaitingSymbol = $flagValue
                }
            }
            '^(--done|--done-symbol)$' {
                if (-not $PSBoundParameters.ContainsKey('DoneSymbol') -and $hasValue) {
                    $DoneSymbol = $flagValue
                }
            }
        }
    }
}

# 5. Build Title Command
# Automatically enable --no-prefix when WTProfile is used (since tab already displays the official Logo)
$effectiveNoPrefix = $NoPrefix.IsPresent -or ($WTProfile.IsPresent -and $Prefix -eq "agy:" -and -not $PSBoundParameters.ContainsKey('NoPrefix') -and -not $PSBoundParameters.ContainsKey('Prefix'))

$scriptArgs = @()
if ($ShowPrefix) { $effectiveNoPrefix = $false }
if ($effectiveNoPrefix) { $scriptArgs += "--no-prefix" }
if ($Text) { $scriptArgs += "--text" }
if ($Animate -and -not $NoAnimate) { $scriptArgs += "--animate" }
if (-not $effectiveNoPrefix -and $Prefix -ne "agy:") {
    $cleanPrefix = Clean-SymbolValue $Prefix
    $prefixArg = Format-SymbolArg "--prefix" $cleanPrefix
    if ($prefixArg) { $scriptArgs += $prefixArg }
}
if ($MaxLength -ne 35) { $scriptArgs += "--max-length $MaxLength" }
if ($TaskTimeout -ne 60) { $scriptArgs += "--task-timeout $TaskTimeout" }

if ($ReadySymbol) {
    $arg = Format-SymbolArg "--ready" $ReadySymbol
    if ($arg) { $scriptArgs += $arg }
}
if ($WorkingSymbol) {
    $arg = Format-SymbolArg "--working" $WorkingSymbol
    if ($arg) { $scriptArgs += $arg }
}
if ($WaitingSymbol) {
    $arg = Format-SymbolArg "--waiting" $WaitingSymbol
    if ($arg) { $scriptArgs += $arg }
}
if ($DoneSymbol) {
    $arg = Format-SymbolArg "--done" $DoneSymbol
    if ($arg) { $scriptArgs += $arg }
}

$argsPart = if ($scriptArgs.Count -gt 0) { " " + ($scriptArgs -join " ") } else { "" }
$destExeNormalized = $destExe -replace '\\', '/'
$titleCmd = if ($destExeNormalized -match '\s') { "`"$destExeNormalized`"$argsPart" } else { "$destExeNormalized$argsPart" }

# Add or update "title" block (Native white-listed structure to survive agy restarts)
$titleConfig = [ordered]@{
    "type" = "command"
    "command" = $titleCmd
    "enabled" = $true
}

if ($settingsObj -is [PSCustomObject] -or $settingsObj -is [System.Collections.IDictionary]) {
    $settingsObj | Add-Member -NotePropertyName "title" -NotePropertyValue $titleConfig -Force
} else {
    $settingsObj = [ordered]@{
        "title" = $titleConfig
    }
}

function Update-TitleJson([string]$raw, [string]$value, [switch]$Remove) {
    if ([string]::IsNullOrWhiteSpace($raw)) { $raw = '{}' }
    $newline = if ($raw.Contains("`r`n")) { "`r`n" } elseif ($raw.Contains("`n")) { "`n" } else { [Environment]::NewLine }
    $indent = '  '
    $rootIndent = $null
    $formatValue = {
        param([string]$baseIndent)
        # Format this value alone; leave every unrelated JSON literal untouched.
        $formatted = Format-CleanJson $value 1
        $lines = [regex]::Split($formatted, "\r?\n")
        for ($lineIndex = 1; $lineIndex -lt $lines.Length; $lineIndex++) {
            $spaces = [regex]::Match($lines[$lineIndex], '^ *').Length
            $lines[$lineIndex] = $baseIndent + ($indent * $spaces) + $lines[$lineIndex].Substring($spaces)
        }
        return $lines -join $newline
    }
    $depth = 0; $inString = $false; $escape = $false; $start = 0; $close = -1
    $titleStart = -1; $titleEnd = -1; $titleHead = ''; $titleIndent = $null; $titleTail = ''
    for ($i = 0; $i -lt $raw.Length; $i++) {
        $c = $raw[$i]
        if ($inString) {
            if ($escape) { $escape = $false }
            elseif ($c -eq [char]92) { $escape = $true }
            elseif ($c -eq [char]34) { $inString = $false }
            continue
        }
        if ($c -eq [char]34) { $inString = $true; continue }
        if ($depth -eq 1 -and ($c -eq ',' -or $c -eq '}')) {
            $segment = $raw.Substring($start, $i - $start)
            $match = [regex]::Match($segment, '^(\s*)("(?:\\.|[^"\\])*")\s*:')
            if ($match.Success) {
                $leading = $match.Groups[1].Value
                $keyIndent = $null
                if ($leading.Contains("`n")) {
                    $keyIndent = $leading.Substring($leading.LastIndexOf("`n")+1)
                    if ($rootIndent -eq $null -and $keyIndent.Length -gt 0) { $rootIndent = $keyIndent; $indent = $keyIndent }
                }
                if ((ConvertFrom-Json $match.Groups[2].Value) -eq 'title') {
                    if ($Remove) {
                        if ($c -eq ',') { return $raw.Substring(0,$start) + $raw.Substring($i+1) }
                        if ($start -gt 0 -and $raw[$start-1] -eq ',') { $start-- }
                        return $raw.Substring(0,$start) + $raw.Substring($i)
                    }
                    $titleStart = $start; $titleEnd = $i
                    $titleHead = $segment.Substring(0,$match.Length)
                    $titleIndent = $keyIndent
                    $titleTail = [regex]::Match($segment,'\s*$').Value
                }
            }
            if ($c -eq ',') { $start = $i + 1 } else { $close = $i }
        }
        if ($c -eq '{' -or $c -eq '[') { $depth++; if ($depth -eq 1) { $start = $i + 1 } }
        elseif ($c -eq '}' -or $c -eq ']') { $depth-- }
    }
    if ($Remove) { return $raw }
    if ($titleStart -ge 0) {
        $baseIndent = if ($titleIndent -ne $null) { $titleIndent } else { $indent }
        return $raw.Substring(0,$titleStart) + $titleHead + ' ' + (& $formatValue $baseIndent) + $titleTail + $raw.Substring($titleEnd)
    }
    if ($close -lt 0) { throw 'Expected a root JSON object' }
    $prefix = $raw.Substring(0,$close).TrimEnd()
    $comma = if ($prefix.EndsWith('{')) { '' } else { ',' }
    return $prefix + $comma + $newline + $indent + '"title": ' + (& $formatValue $indent) + $newline + $raw.Substring($close)
}

function Format-CleanJson([string]$json, [int]$indent = 2) {
    $indentStr = " " * $indent
    $level = 0
    $inString = $false
    $escape = $false
    $sb = [System.Text.StringBuilder]::new()

    for ($i = 0; $i -lt $json.Length; $i++) {
        $c = $json[$i]
        if ($escape) {
            [void]$sb.Append($c)
            $escape = $false
            continue
        }
        if ($c -eq [char]92) {
            [void]$sb.Append($c)
            if ($inString) { $escape = $true }
            continue
        }
        if ($c -eq [char]34) {
            [void]$sb.Append($c)
            $inString = -not $inString
            continue
        }
        if ($inString) {
            [void]$sb.Append($c)
            continue
        }
        if ($c -eq ' ' -or $c -eq "`t" -or $c -eq "`r" -or $c -eq "`n") { continue }

        if ($c -eq '{' -or $c -eq '[') {
            $closer = if ($c -eq '{') { '}' } else { ']' }
            $peek = $i + 1
            while ($peek -lt $json.Length -and ($json[$peek] -eq ' ' -or $json[$peek] -eq "`t" -or $json[$peek] -eq "`r" -or $json[$peek] -eq "`n")) {
                $peek++
            }
            if ($peek -lt $json.Length -and $json[$peek] -eq $closer) {
                [void]$sb.Append($c)
                [void]$sb.Append($closer)
                $i = $peek
                continue
            }
            [void]$sb.Append($c)
            $level++
            [void]$sb.AppendLine()
            [void]$sb.Append($indentStr * $level)
        }
        elseif ($c -eq '}' -or $c -eq ']') {
            $level = [Math]::Max(0, $level - 1)
            [void]$sb.AppendLine()
            [void]$sb.Append($indentStr * $level)
            [void]$sb.Append($c)
        }
        elseif ($c -eq ',') {
            [void]$sb.Append($c)
            [void]$sb.AppendLine()
            [void]$sb.Append($indentStr * $level)
        }
        elseif ($c -eq ':') {
            [void]$sb.Append(': ')
        }
        else {
            [void]$sb.Append($c)
        }
    }
    return $sb.ToString()
}

# Timestamped incremental backup before write
if (Test-Path $settingsPath) {
    $ts = (Get-Date).ToString("yyyyMMddHHmmss")
    $incBak = Join-Path $settingsDir "settings.json.$ts.bak"
    [System.IO.File]::WriteAllText($incBak, $raw, $utf8NoBom)
    Write-Host "[+] Incremental backup saved to: $incBak" -ForegroundColor Gray
    Get-ChildItem -Path $settingsDir -Filter "settings.json.*.bak" -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^settings\.json\.\d{14}\.bak$' } |
        Sort-Object LastWriteTime -Descending |
        Select-Object -Skip 5 |
        ForEach-Object { Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue }
}

$updatedJson = Update-TitleJson $raw (Format-CleanJson ($titleConfig | ConvertTo-Json -Depth 100) 2)
[System.IO.File]::WriteAllText($settingsPath, $updatedJson, $utf8NoBom)
Write-Host "[+] Successfully configured 'title' in: $settingsPath" -ForegroundColor Green

# 6. Windows Terminal Profile & Smart Launcher Integration
if ($WTProfile) {
    $wtPaths = @(
        (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json'),
        (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json'),
        (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows Terminal\settings.json')
    )
    $foundWt = $null
    foreach ($p in $wtPaths) {
        if (Test-Path $p) { $foundWt = $p; break }
    }
    $wtProfileSuccess = $false
    if ($foundWt) {
        try {
            $rawWt = Get-Content $foundWt -Raw -Encoding UTF8
            $wtObj = ConvertFrom-JsonC $rawWt
            $wtBak = $foundWt + '.pre-title.bak'
            if (-not (Test-Path $wtBak)) {
                [System.IO.File]::WriteAllText($wtBak, $rawWt, $utf8NoBom)
                Write-Host "[+] Initial Windows Terminal backup saved to: $wtBak" -ForegroundColor Gray
            }

            $iconPathNorm = ((Join-Path $assetsDir "antigravity.png") -replace '\\', '/')
            $pwshCmd = if (Get-Command pwsh -ErrorAction SilentlyContinue) { "pwsh.exe" } else { "powershell.exe" }
            $profileCmd = "$pwshCmd -NoExit -Command `"`$env:AGY_ACTIVE='1'; agy`""
            $agyGuid = "{a9191a91-a919-4a91-a919-a9191a91a919}"

            $hasComments = Test-HasJsonComments $rawWt
            $alreadyHasAgy = @(@($wtObj.profiles.list) + @($wtObj.profiles) | Where-Object { $_.guid -eq $agyGuid }).Count -gt 0

            $written = $false
            if ($hasComments -and -not $alreadyHasAgy) {
                $escapedCmd = $profileCmd.Replace('\', '\\').Replace('"', '\"')
                $profileText = @"
            {
                "commandline": "$escapedCmd",
                "guid": "$agyGuid",
                "hidden": false,
                "icon": "$iconPathNorm",
                "name": "Google Antigravity",
                "tabTitle": "Google Antigravity"
            }
"@
                $updatedWt = $null
                if ($rawWt -match '(?s)("list"\s*:\s*\[\s*)(\])') {
                        $updatedWt = [regex]::Replace($rawWt, '(?s)("list"\s*:\s*\[\s*)(\])', ('$1' + [Environment]::NewLine + $profileText + [Environment]::NewLine + '        $2'), 1)
                    } elseif ($rawWt -match '("list"\s*:\s*\[\s*)') {
                        $updatedWt = [regex]::Replace($rawWt, '("list"\s*:\s*\[\s*)', ('$1' + [Environment]::NewLine + $profileText + ',' + [Environment]::NewLine), 1)
                    } elseif ($rawWt -match '(?s)("profiles"\s*:\s*\[\s*)(\])') {
                        $updatedWt = [regex]::Replace($rawWt, '(?s)("profiles"\s*:\s*\[\s*)(\])', ('$1' + [Environment]::NewLine + $profileText + [Environment]::NewLine + '        $2'), 1)
                    } elseif ($rawWt -match '("profiles"\s*:\s*\[\s*)') {
                        $updatedWt = [regex]::Replace($rawWt, '("profiles"\s*:\s*\[\s*)', ('$1' + [Environment]::NewLine + $profileText + ',' + [Environment]::NewLine), 1)
                    }
                if ($updatedWt) {
                    $verifyObj = ConvertFrom-JsonC $updatedWt
                    if ($verifyObj) {
                        [System.IO.File]::WriteAllText($foundWt, $updatedWt, $utf8NoBom)
                        $written = $true
                        $wtProfileSuccess = $true
                        Write-Host "[+] Configured 'Google Antigravity' profile in Windows Terminal (JSONC comments preserved): $foundWt" -ForegroundColor Green
                    }
                }
            }

            if (-not $written) {
                if (-not $wtObj.PSObject.Properties['profiles']) {
                    $wtObj | Add-Member -NotePropertyName profiles -NotePropertyValue ([pscustomobject]@{list=@()})
                }
                $targetList = $null
                $isNestedList = $false
                if ($wtObj.profiles -is [System.Management.Automation.PSCustomObject]) {
                    if (-not $wtObj.profiles.PSObject.Properties['list']) {
                        $wtObj.profiles | Add-Member -NotePropertyName list -NotePropertyValue @()
                    }
                    $targetList = $wtObj.profiles.list
                    $isNestedList = $true
                } elseif ($wtObj.profiles -is [System.Collections.IEnumerable]) {
                    $targetList = $wtObj.profiles
                    $isNestedList = $false
                }
                $existingProfile = $null
                if ($targetList) {
                    foreach ($pr in $targetList) {
                        if ($pr.guid -eq $agyGuid) {
                            $existingProfile = $pr
                            break
                        }
                    }
                }
                if ($existingProfile) {
                    foreach ($pair in @{icon=$iconPathNorm; commandline=$profileCmd; tabTitle="Google Antigravity"; hidden=$false}.GetEnumerator()) {
                        $existingProfile | Add-Member -NotePropertyName $pair.Key -NotePropertyValue $pair.Value -Force
                    }
                } else {
                    $newPr = [ordered]@{
                        "guid" = $agyGuid
                        "name" = "Google Antigravity"
                        "tabTitle" = "Google Antigravity"
                        "commandline" = $profileCmd
                        "icon" = $iconPathNorm
                        "hidden" = $false
                    }
                    $list = [System.Collections.ArrayList]@($targetList | Where-Object { $null -ne $_ })
                    [void]$list.Add($newPr)
                    if ($isNestedList) {
                        $wtObj.profiles.list = $list
                    } else {
                        $wtObj.profiles = $list
                    }
                }
                $cleanWtJson = Format-CleanJson ($wtObj | ConvertTo-Json -Depth 100) 4
                [System.IO.File]::WriteAllText($foundWt, $cleanWtJson, $utf8NoBom)
                $wtProfileSuccess = $true
                Write-Host "[+] Configured 'Google Antigravity' profile in Windows Terminal: $foundWt" -ForegroundColor Green
            }
        } catch {
            Write-Warning "[-] Failed to configure Windows Terminal profile: $_"
        }
    }

    if ($wtProfileSuccess) {
        # 7. Configure PowerShell Profile Router (Dual-Compatibility: PS7 & Windows PowerShell 5.1)
        $rawBaseDirs = @(
            [Environment]::GetFolderPath('MyDocuments'),
            (Join-Path $homeDir 'Documents'),
        (Join-Path $homeDir 'OneDrive\Documents'),
        (Join-Path $homeDir "OneDrive\$([char]0x6587)$([char]0x6863)")
    )
    $validBaseDirs = @($rawBaseDirs | Where-Object { -not [string]::IsNullOrWhiteSpace($_) -and (Test-Path $_) } | Select-Object -Unique)

    $hasPwsh = [bool](Get-Command pwsh.exe -ErrorAction SilentlyContinue)
    $candidateProfiles = [System.Collections.Generic.List[string]]::new()
    if ($PROFILE) {
        $candidateProfiles.Add([System.IO.Path]::GetFullPath($PROFILE))
    }

    foreach ($baseDir in $validBaseDirs) {
        $candidateProfiles.Add([System.IO.Path]::GetFullPath((Join-Path $baseDir 'PowerShell\Microsoft.PowerShell_profile.ps1')))
        $candidateProfiles.Add([System.IO.Path]::GetFullPath((Join-Path $baseDir 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')))
    }

    $uniqueProfiles = @($candidateProfiles | Select-Object -Unique)

    $routerBlock = @"
# >>> antigravity-cli-title profile router >>>
`$makeAliasWrapper = {
    param(`$command, [string]`$fallback)
    `$path = if (`$command.CommandType -eq "Application" -or `$command.CommandType -eq "ExternalScript") { `$command.Source } else { `$fallback }
    `$quotedPath = "'" + ([regex]::Replace(`$path, "['`$([char]0x2018)`$([char]0x2019)`$([char]0x201A)`$([char]0x201B)]", '`$0`$0')) + "'"
    `$invoke = '& ' + `$quotedPath + ' @args'
    [scriptblock]::Create('if (`$MyInvocation.ExpectingInput) { `$input | ' + `$invoke + ' } else { ' + `$invoke + ' }')
}
if (Test-Path Alias:agy) {
    `$aliasTarget = (Get-Alias agy).Definition
    if (`$aliasTarget -ne 'agy' -and -not (Test-Path Function:\__agy_original)) {
        `$aliasCommand = Get-Command `$aliasTarget -ErrorAction SilentlyContinue
        if (`$aliasCommand) {
            Set-Item Function:\__agy_original -Value (& `$makeAliasWrapper `$aliasCommand `$aliasTarget)
        }
    }
    Remove-Item Alias:agy -Force -ErrorAction SilentlyContinue
}
if (Test-Path Function:\agy) {
    if (-not (Test-Path Function:\__agy_original)) {
        `$existingDef = (Get-Command -CommandType Function agy -ErrorAction SilentlyContinue).Definition
        if (`$existingDef -notmatch '__agy_original' -and `$existingDef -notmatch 'Google Antigravity') {
            Copy-Item Function:\agy Function:\__agy_original -Force -ErrorAction SilentlyContinue
        }
    }
}
function agy {
    `$isIdeTerminal = (`$env:SSH_CONNECTION -or `$env:SSH_TTY -or (`$env:TERM_PROGRAM -and `$env:TERM_PROGRAM -ne "Windows_Terminal")) -or (`$env:TERM_PROGRAM -eq 'vscode') -or (`$env:TERMINAL_EMULATOR -match 'JetBrains') -or (`$env:INTELLIJ_ENVIRONMENT_READER)
    `$isNonInteractive = (`$args.Count -gt 0 -and ((`$args[0] -match '^(acp|agent|agents|changelog|help|install|mcp|mic-serve|models|plugin|plugins|remote-control|update)`$') -or [bool](`$args -match '^(--?(print|prompt|help|version|output-format|input-format)|-p|-h|-v)(|=.*)`$')))
    `$isRedirected = [Console]::IsOutputRedirected -or [Console]::IsInputRedirected -or (`$MyInvocation.PipelineLength -gt 1)
    `$wtCmd = if (Get-Command wt.exe -ErrorAction SilentlyContinue) { "wt.exe" } elseif (Test-Path "`$env:LOCALAPPDATA\Microsoft\WindowsApps\wt.exe") { "`$env:LOCALAPPDATA\Microsoft\WindowsApps\wt.exe" } else { `$null }
    if (-not `$isIdeTerminal -and -not `$isNonInteractive -and -not `$isRedirected -and `$env:AGY_ACTIVE -ne '1' -and `$wtCmd) {
        `$wArg = if (`$env:WT_SESSION) { @("-w", "0") } else { @() }
        `$workDir = if (`$PWD.Provider.Name -eq 'FileSystem') { `$PWD.ProviderPath } else { [Environment]::GetFolderPath('UserProfile') }
        if (`$args.Count -gt 0) {
            `$quotedArgs = (`$args | ForEach-Object { "'" + ([regex]::Replace([string]`$_, "['`$([char]0x2018)`$([char]0x2019)`$([char]0x201A)`$([char]0x201B)]", '`$0`$0')) + "'" }) -join ' '
            `$pwsh = if (Get-Command pwsh.exe -ErrorAction SilentlyContinue) { "pwsh.exe" } else { "powershell.exe" }
            `$rawCmd = '`$env:AGY_ACTIVE=''1''; agy ' + `$quotedArgs
            `$enc = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes(`$rawCmd))
            & `$wtCmd @wArg nt -p "Google Antigravity" -d "`$workDir" `$pwsh -NoExit -EncodedCommand `$enc
        } else {
            & `$wtCmd @wArg nt -p "Google Antigravity" -d "`$workDir"
        }
        return
    }
    if (Get-Command __agy_original -ErrorAction SilentlyContinue) {
        if (`$MyInvocation.ExpectingInput) { `$input | & __agy_original @args } else { & __agy_original @args }
    } else {
        `$realAgy = "`$env:LOCALAPPDATA\agy\bin\agy.exe"
        if (-not (Test-Path `$realAgy)) {
            `$found = Get-Command agy.exe -ErrorAction SilentlyContinue
            if (`$found) { `$realAgy = `$found.Source }
        }
        if (`$MyInvocation.ExpectingInput) { `$input | & `$realAgy @args } else { & `$realAgy @args }
    }
}
# <<< antigravity-cli-title profile router <<<
"@

    function Get-EffectiveProfilePolicy([string]$profPath) {
        $isPs51 = ($profPath -match 'WindowsPowerShell')
        if ($isPs51) {
            foreach ($rk in @(
                'HKLM:\Software\Policies\Microsoft\Windows\PowerShell',
                'HKCU:\Software\Policies\Microsoft\Windows\PowerShell',
                'HKCU:\Software\Microsoft\PowerShell\1\ShellIds\Microsoft.PowerShell',
                'HKLM:\Software\Microsoft\PowerShell\1\ShellIds\Microsoft.PowerShell'
            )) {
                $val = (Get-ItemProperty -Path $rk -Name "ExecutionPolicy" -ErrorAction SilentlyContinue).ExecutionPolicy
                if ($val -and [string]$val -ne 'Undefined') { return [string]$val }
            }
            $installationType = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -Name InstallationType -ErrorAction SilentlyContinue).InstallationType
            if ($installationType -match 'Server') { return 'RemoteSigned' }
            return 'Restricted'
        }

        if ($PSVersionTable.PSVersion.Major -le 5) {
            if (Get-Command pwsh.exe -ErrorAction SilentlyContinue) {
                try {
                    $pwshScript = '$list = Get-ExecutionPolicy -List; foreach ($s in @("MachinePolicy","UserPolicy","CurrentUser","LocalMachine")) { $item = $list | Where-Object { $_.Scope -eq $s }; if ($item -and $item.ExecutionPolicy -ne "Undefined") { [string]$item.ExecutionPolicy; break } }'
                    $enc = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($pwshScript))
                    $pOut = (& pwsh.exe -NoProfile -EncodedCommand $enc 2>$null)
                    if ($pOut) {
                        $pPolicy = ([string]($pOut | Select-Object -Last 1)).Trim()
                        if ($pPolicy) { return $pPolicy }
                    }
                } catch {}
            }
            return 'RemoteSigned'
        }

        try {
            $list = Get-ExecutionPolicy -List -ErrorAction Stop
            foreach ($scope in @('MachinePolicy', 'UserPolicy', 'CurrentUser', 'LocalMachine')) {
                $item = $list | Where-Object { $_.Scope -eq $scope }
                if ($item -and $item.ExecutionPolicy -ne 'Undefined') {
                    return [string]$item.ExecutionPolicy
                }
            }
            return 'RemoteSigned'
        } catch {
            return 'RemoteSigned'
        }
    }

    foreach ($profPath in $uniqueProfiles) {
        $parentDir = Split-Path $profPath
        $isCurrentProfile = ($PROFILE -and ([System.IO.Path]::GetFullPath($profPath) -eq [System.IO.Path]::GetFullPath($PROFILE)))

        $shouldConfigure = (Test-Path $profPath) -or $isCurrentProfile -or ($hasPwsh -and $profPath -match '[\\/]PowerShell[\\/]')
        if ($shouldConfigure) {
            $policy = Get-EffectiveProfilePolicy $profPath
            if ($policy -eq 'Restricted' -or $policy -eq 'AllSigned') {
                if (Test-Path $profPath) {
                    try {
                        $pBytes = [System.IO.File]::ReadAllBytes($profPath)
                        $parsed = Get-FileEncodingAndContent $pBytes
                        $pText = $parsed.Content
                        $pEnc = $parsed.Encoding
                        if ($pText -match '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<') {
                            $cleaned = ($pText -replace '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<', '').Trim()
                            $profBak = $profPath + ".pre-title.bak"
                            $nl = if ($pText -match "`r`n") { "`r`n" } elseif ($pText -match "`n") { "`n" } else { [Environment]::NewLine }
                            $profileText = if ([string]::IsNullOrWhiteSpace($cleaned)) { "" } else { $cleaned + $nl }
                            [System.IO.File]::WriteAllText($profPath, $profileText, $pEnc)
                            Write-Host "[+] Removed profile router; current file and backup preserved: $profPath" -ForegroundColor Yellow
                        }
                    } catch {}
                }
                Write-Warning "[-] Skipping profile configuration at '$profPath': ExecutionPolicy is $policy. Run 'Set-ExecutionPolicy RemoteSigned -Scope CurrentUser' to enable profiles."
                continue
            }
            $profEncoding = $utf8NoBom
            $existing = ""
            if (Test-Path $profPath) {
                $bytes = [System.IO.File]::ReadAllBytes($profPath)
                $profBak = $profPath + ".pre-title.bak"
                if (-not (Test-Path $profBak)) {
                    [System.IO.File]::WriteAllBytes($profBak, $bytes)
                    Write-Host "[+] Initial profile backup saved to: $profBak" -ForegroundColor Gray
                }
                $parsed = Get-FileEncodingAndContent $bytes
                $profEncoding = $parsed.Encoding
                $existing = $parsed.Content
            }

            if (-not (Test-Path $parentDir)) { New-Item -ItemType Directory -Path $parentDir -Force | Out-Null }
            $nl = if ($existing -match "`r`n") { "`r`n" } elseif ($existing -match "`n") { "`n" } else { [Environment]::NewLine }
            $normRouterBlock = if ($nl -eq "`n") { $routerBlock.Replace("`r`n", "`n") } else { $routerBlock.Replace("`r`n", "`n").Replace("`n", "`r`n") }
            if ($existing -match '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<') {
                $newContent = [regex]::Replace($existing, '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<', { $normRouterBlock.Trim() })
                [System.IO.File]::WriteAllText($profPath, $newContent.Trim() + $nl, $profEncoding)
                Write-Host "[+] Updated Antigravity profile router in: $profPath" -ForegroundColor Green
            } else {
                $hasCustomAgy = ($existing -and ($existing -match 'function\s+agy\b'))
                $existingText = if ([string]::IsNullOrWhiteSpace($existing)) { "" } else { $existing.TrimEnd() }
                $separator = if ([string]::IsNullOrWhiteSpace($existingText)) { "" } else { $nl + $nl }
                $newContent = $existingText + $separator + $normRouterBlock.Trim() + $nl
                [System.IO.File]::WriteAllText($profPath, $newContent, $profEncoding)
                if ($hasCustomAgy) {
                    Write-Host "[+] Injected Antigravity profile router into: $profPath (seamlessly wrapped your custom agy/proxy function)" -ForegroundColor Green
                } else {
                    Write-Host "[+] Injected Antigravity profile router into: $profPath" -ForegroundColor Green
                }
            }
        }
    }
} else {
    Write-Warning "[-] Skipping profile router configuration because Windows Terminal profile was not configured."
}
}

Write-Host "`n$eSparkles Installation complete!" -ForegroundColor Cyan
Write-Host "   Command configured: $titleCmd" -ForegroundColor Gray
Write-Host "   Installed directory: $targetDir" -ForegroundColor Gray
Write-Host "   Engine: Native Compiled Executable (optional Windows animation worker)" -ForegroundColor Green
Write-Host "   To uninstall later, run:" -ForegroundColor Gray
Write-Host "     powershell.exe -ExecutionPolicy Bypass -File `"$destUninstall`"" -ForegroundColor Gray
Write-Host "   Try asking Antigravity CLI a question to see the 4-state indicator in action:`n"
Write-Host "   $eReady Ready     -> Initial state / re-entering history" -ForegroundColor Gray
Write-Host "   $eWorking  Working   -> Thinking / background task execution" -ForegroundColor Yellow
Write-Host "   $eWaiting  Waiting   -> Prompting user (ask_question) or permission confirmation" -ForegroundColor Red
Write-Host "   $eDone Done      -> Turn completed; remains done until new input" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan

# Prevent window from closing instantly when launched directly from Windows Explorer
try {
    $ppid = (Get-CimInstance Win32_Process -Filter ("ProcessId = " + $PID) -ErrorAction SilentlyContinue).ParentProcessId
    $parentName = if ($ppid) { (Get-Process -Id $ppid -ErrorAction SilentlyContinue).ProcessName } else { "" }
    $effectiveScriptPath = if ($InvokedCommandPath) { $InvokedCommandPath } elseif ($PSCommandPath) { $PSCommandPath } else { "" }
    $isScriptInvokedDirectly = (-not $isRemotePipeline -and $effectiveScriptPath -and (Test-Path -Path $effectiveScriptPath -PathType Leaf) -and ([Environment]::CommandLine -match '(?i)[/-](file|command)\b'))
    if ($isScriptInvokedDirectly -and $parentName -match 'explorer') {
        Write-Host "`nPress Enter to exit..." -ForegroundColor Gray
        [void][System.Console]::ReadLine()
    }
} catch {}
    } finally {
        $OutputEncoding = $origOutputEncoding
        [Console]::OutputEncoding = $origConsoleOutputEncoding
    }
} @callArgs
Remove-Variable callArgs -ErrorAction SilentlyContinue
