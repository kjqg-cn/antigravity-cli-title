# install.ps1 - Installer for Antigravity CLI Window Title (Windows/PowerShell)
param(
    [switch]$NoPrefix,
    [switch]$Text,
    [string]$Prefix = "agy:",
    [int]$MaxLength = 35
)

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "==========================================================" -ForegroundColor Cyan
$eRocket   = [char]::ConvertFromUtf32(0x1F680)
$eSparkles = [char]::ConvertFromUtf32(0x2728)
$eReady    = [char]::ConvertFromUtf32(0x26AA)
$eWorking  = [char]::ConvertFromUtf32(0x2699) + [char]0xFE0F
$eWaiting  = [char]::ConvertFromUtf32(0x26A0) + [char]0xFE0F
$eDone     = [char]::ConvertFromUtf32(0x2705)

Write-Host "   $eRocket Antigravity CLI Window Title Installer (Windows)    " -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan

$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"
if (-not (Test-Path $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    Write-Host "[+] Created target directory: $targetDir" -ForegroundColor Gray
}

# 1. Install title.ps1 and uninstall.ps1
$repoBaseUrl = "https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main"

$localTitle = if ($PSScriptRoot) { Join-Path $PSScriptRoot "scripts\title.ps1" } else { "scripts\title.ps1" }
$localUninstall = if ($PSScriptRoot) { Join-Path $PSScriptRoot "uninstall.ps1" } else { "uninstall.ps1" }

$destFile = Join-Path $targetDir "title.ps1"
$destUninstall = Join-Path $targetDir "uninstall.ps1"

if (Test-Path $localTitle) {
    Copy-Item -Path $localTitle -Destination $destFile -Force
    Write-Host "[+] Installed renderer to: $destFile" -ForegroundColor Green
} else {
    Write-Host "[*] Fetching renderer from GitHub..." -ForegroundColor Gray
    try {
        Invoke-RestMethod -Uri "$repoBaseUrl/scripts/title.ps1" -OutFile $destFile
        Write-Host "[+] Downloaded renderer to: $destFile" -ForegroundColor Green
    } catch {
        Write-Error "[-] Failed to download renderer from GitHub: $_"
        exit 1
    }
}

if (Test-Path $localUninstall) {
    Copy-Item -Path $localUninstall -Destination $destUninstall -Force
    Write-Host "[+] Installed uninstaller to: $destUninstall" -ForegroundColor Green
} else {
    Write-Host "[*] Fetching uninstaller from GitHub..." -ForegroundColor Gray
    try {
        Invoke-RestMethod -Uri "$repoBaseUrl/uninstall.ps1" -OutFile $destUninstall
        Write-Host "[+] Downloaded uninstaller to: $destUninstall" -ForegroundColor Green
    } catch {
        Write-Warning "[-] Could not fetch uninstall.ps1 from remote: $_"
    }
}

# 2. Build Title Command
$scriptArgs = @()
if ($NoPrefix) { $scriptArgs += "--no-prefix" }
if ($Text) { $scriptArgs += "--text" }
if ($Prefix -ne "agy:") { $scriptArgs += "--prefix `"$Prefix`"" }
if ($MaxLength -ne 35) { $scriptArgs += "--max-length $MaxLength" }

$argsPart = if ($scriptArgs.Count -gt 0) { " " + ($scriptArgs -join " ") } else { "" }
$destPathNormalized = $destFile -replace '\\', '/'
$titleCmd = "powershell.exe -NoProfile -ExecutionPolicy Bypass -File $destPathNormalized$argsPart"

# 3. Update settings.json
$settingsDir = Join-Path $homeDir ".gemini\antigravity-cli"
$settingsPath = Join-Path $settingsDir "settings.json"

if (-not (Test-Path $settingsDir)) {
    New-Item -ItemType Directory -Path $settingsDir -Force | Out-Null
}

$settingsObj = [ordered]@{}
if (Test-Path $settingsPath) {
    try {
        $raw = Get-Content -Raw -Path $settingsPath -Encoding UTF8
        # Backup original
        $bakPath = Join-Path $settingsDir "settings.json.bak"
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($bakPath, $raw, $utf8NoBom)
        Write-Host "[+] Backup saved to: $bakPath" -ForegroundColor Gray
        
        $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop
    } catch {
        Write-Warning "[!] Existing settings.json had invalid JSON. A fresh file will be created."
        $settingsObj = [ordered]@{}
    }
}

# Add or update "title" block
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
    $res = $sb.ToString()
    return ($res -replace '\{\s*\}', '{}' -replace '\[\s*\]', '[]' -replace '\\u0027', "'" -replace '\\u003c', '<' -replace '\\u003e', '>' -replace '\\u0026', '&')
}

$updatedJson = Format-CleanJson ($settingsObj | ConvertTo-Json -Depth 10) 2
if (-not $utf8NoBom) { $utf8NoBom = New-Object System.Text.UTF8Encoding($false) }
[System.IO.File]::WriteAllText($settingsPath, $updatedJson, $utf8NoBom)
Write-Host "[+] Successfully configured 'title' in: $settingsPath" -ForegroundColor Green

Write-Host "`n$eSparkles Installation complete!" -ForegroundColor Cyan
Write-Host "   Command configured: $titleCmd" -ForegroundColor Gray
Write-Host "   Installed directory: $targetDir" -ForegroundColor Gray
Write-Host "   To uninstall later, run:" -ForegroundColor Gray
Write-Host "     powershell.exe -File `"$destUninstall`"" -ForegroundColor Gray
Write-Host "   Try asking Antigravity CLI a question to see the 4-state indicator in action:`n"
Write-Host "   $eReady Ready     -> Initial state / re-entering history" -ForegroundColor Gray
Write-Host "   $eWorking  Working   -> Thinking / background task execution" -ForegroundColor Yellow
Write-Host "   $eWaiting  Waiting   -> Prompting user (ask_question) or permission confirmation" -ForegroundColor Red
Write-Host "   $eDone Done      -> Turn completed; remains done until new input" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan
