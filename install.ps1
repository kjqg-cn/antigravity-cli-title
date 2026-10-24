# install.ps1 - Native Installer for Antigravity CLI Window Title (Windows Native Engine)
param(
    [switch]$NoPrefix,
    [switch]$Text,
    [string]$Prefix = "agy:",
    [int]$MaxLength = 35,
    [int]$TaskTimeout = 60,
    [switch]$WTProfile
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

Write-Host "   $eRocket Antigravity CLI Window Title Installer (Windows Native)    " -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan

$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"
if (-not (Test-Path $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    Write-Host "[+] Created target directory: $targetDir" -ForegroundColor Gray
}

# 1. Install uninstaller (uninstall.ps1)
$repoBaseUrl = "https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main"
$localUninstall = if ($PSScriptRoot) { Join-Path $PSScriptRoot "uninstall.ps1" } else { "uninstall.ps1" }
$destUninstall = Join-Path $targetDir "uninstall.ps1"

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

# 2. Install Native Fast Execution Engine (title.exe)
$localExe = if ($PSScriptRoot) { Join-Path $PSScriptRoot "title.exe" } else { "title.exe" }
$destExe = Join-Path $targetDir "title.exe"
$hasNativeExe = $false

if (Test-Path $localExe) {
    Copy-Item -Path $localExe -Destination $destExe -Force
    Write-Host "[+] Installed native fast execution engine: $destExe" -ForegroundColor Green
    $hasNativeExe = $true
} else {
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    $goSourceDir = if ($PSScriptRoot) { Join-Path $PSScriptRoot "cmd\title" } else { "cmd\title" }
    if ($goCmd -and (Test-Path $goSourceDir)) {
        Write-Host "[*] Compiling native execution engine (title.exe) via local Go..." -ForegroundColor Gray
        try {
            & go build -ldflags="-s -w" -o $destExe "$goSourceDir"
            if (Test-Path $destExe) {
                Write-Host "[+] Compiled native execution engine: $destExe" -ForegroundColor Green
                $hasNativeExe = $true
            }
        } catch {}
    }
}

if (-not $hasNativeExe) {
    try {
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
        Write-Error "[-] Failed to obtain title.exe: $_`nPlease install Go (https://go.dev) or compile manually: go build -o title.exe ./cmd/title"
        exit 1
    }
}

# 3. Install Antigravity Icon Assets
$assetsDir = Join-Path $targetDir "assets"
if (-not (Test-Path $assetsDir)) { New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null }
$localAssets = if ($PSScriptRoot) { Join-Path $PSScriptRoot "assets" } else { "assets" }
if (Test-Path $localAssets) {
    Copy-Item -Path "$localAssets\*" -Destination $assetsDir -Force -Recurse
    Write-Host "[+] Installed Antigravity icon assets to: $assetsDir" -ForegroundColor Green
} else {
    try {
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.png" -OutFile (Join-Path $assetsDir "antigravity.png")
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.ico" -OutFile (Join-Path $assetsDir "antigravity.ico")
        Invoke-RestMethod -Uri "$repoBaseUrl/assets/antigravity.svg" -OutFile (Join-Path $assetsDir "antigravity.svg")
        Write-Host "[+] Downloaded Antigravity icon assets to: $assetsDir" -ForegroundColor Green
    } catch {}
}

# 4. Build Title Command
$scriptArgs = @()
if ($NoPrefix) { $scriptArgs += "--no-prefix" }
if ($Text) { $scriptArgs += "--text" }
if ($Prefix -ne "agy:") { $scriptArgs += "--prefix `"$Prefix`"" }
if ($MaxLength -ne 35) { $scriptArgs += "--max-length $MaxLength" }
if ($TaskTimeout -ne 60) { $scriptArgs += "--task-timeout $TaskTimeout" }

$argsPart = if ($scriptArgs.Count -gt 0) { " " + ($scriptArgs -join " ") } else { "" }
$destExeNormalized = $destExe -replace '\\', '/'
$titleCmd = "$destExeNormalized$argsPart"

# 5. Update settings.json
$settingsDir = Join-Path $homeDir ".gemini\antigravity-cli"
$settingsPath = Join-Path $settingsDir "settings.json"

if (-not (Test-Path $settingsDir)) {
    New-Item -ItemType Directory -Path $settingsDir -Force | Out-Null
}

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$settingsObj = [ordered]@{}
if (Test-Path $settingsPath) {
    try {
        $raw = Get-Content -Raw -Path $settingsPath -Encoding UTF8
        # Backup original
        $bakPath = Join-Path $settingsDir "settings.json.bak"
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
    if ($foundWt) {
        try {
            $rawWt = Get-Content $foundWt -Raw -Encoding UTF8
            $wtObj = ConvertFrom-Json $rawWt -ErrorAction Stop
            [System.IO.File]::WriteAllText(($foundWt + '.bak'), $rawWt, $utf8NoBom)

            $iconPathNorm = ((Join-Path $assetsDir "antigravity.png") -replace '\\', '/')
            $pwshCmd = if (Get-Command pwsh -ErrorAction SilentlyContinue) { "pwsh.exe" } else { "powershell.exe" }
            $profileCmd = "$pwshCmd -NoExit -Command `"`$env:AGY_ACTIVE='1'; agy`""
            $agyGuid = "{a9191a91-a919-4a91-a919-a9191a91a919}"

            $existingProfile = $null
            if ($wtObj.profiles.list) {
                foreach ($pr in $wtObj.profiles.list) {
                    if ($pr.guid -eq $agyGuid -or $pr.name -eq "Google Antigravity") {
                        $existingProfile = $pr
                        break
                    }
                }
            }
            if ($existingProfile) {
                $existingProfile.icon = $iconPathNorm
                $existingProfile.commandline = $profileCmd
                $existingProfile.tabTitle = "Google Antigravity"
                $existingProfile.hidden = $false
                if ($existingProfile.PSObject.Properties['startingDirectory']) {
                    $existingProfile.PSObject.Properties.Remove('startingDirectory')
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
                $list = [System.Collections.ArrayList]@($wtObj.profiles.list)
                [void]$list.Add($newPr)
                $wtObj.profiles.list = $list
            }
            $cleanWtJson = Format-CleanJson ($wtObj | ConvertTo-Json -Depth 10) 4
            [System.IO.File]::WriteAllText($foundWt, $cleanWtJson, $utf8NoBom)
            Write-Host "[+] Configured 'Google Antigravity' profile in Windows Terminal: $foundWt" -ForegroundColor Green
        } catch {
            Write-Warning "[-] Failed to configure Windows Terminal profile: $_"
        }
    }

    # 7. Configure PowerShell Profile Router (Clean & Proxy-Free)
    $docsDir = [Environment]::GetFolderPath('MyDocuments')
    $candidateProfiles = @(
        (Join-Path $docsDir 'PowerShell\Microsoft.PowerShell_profile.ps1'),
        (Join-Path $docsDir 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')
    )

    $routerBlock = @"
# >>> antigravity-cli-title profile router >>>
function agy {
    if (`$env:AGY_ACTIVE -ne '1' -and (Get-Command wt.exe -ErrorAction SilentlyContinue)) {
        `$wArg = if (`$env:WT_SESSION) { @("-w", "0") } else { @() }
        if (`$args.Count -gt 0) {
            `$argStr = (`$args | ForEach-Object { if (`$_ -match '\s') { "`"`$_`"" } else { `$_ } }) -join ' '
            `$argStr = `$argStr -replace ';', '\;'
            `$pwsh = if (Get-Command pwsh -ErrorAction SilentlyContinue) { "pwsh.exe" } else { "powershell.exe" }
            wt.exe @wArg nt -p "{a9191a91-a919-4a91-a919-a9191a91a919}" -d "`$PWD" `$pwsh -NoExit -Command "if (`$env:AGY_ACTIVE = '1') { agy `$argStr }"
        } else {
            wt.exe @wArg nt -p "{a9191a91-a919-4a91-a919-a9191a91a919}" -d "`$PWD"
        }
        return
    }
    `$realAgy = "`$env:LOCALAPPDATA\agy\bin\agy.exe"
    if (-not (Test-Path `$realAgy)) {
        `$found = Get-Command agy.exe -ErrorAction SilentlyContinue
        if (`$found) { `$realAgy = `$found.Source }
    }
    & `$realAgy @args
}
# <<< antigravity-cli-title profile router <<<
"@

    foreach ($profPath in $candidateProfiles) {
        $parentDir = Split-Path $profPath
        if ((Test-Path $profPath) -or (Test-Path $parentDir)) {
            if (-not (Test-Path $parentDir)) { New-Item -ItemType Directory -Path $parentDir -Force | Out-Null }
            $existing = if (Test-Path $profPath) { Get-Content $profPath -Raw -Encoding UTF8 } else { "" }
            if ($existing -match '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<') {
                $newContent = $existing -replace '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<', $routerBlock.Trim()
                [System.IO.File]::WriteAllText($profPath, $newContent.Trim() + [Environment]::NewLine, $utf8NoBom)
                Write-Host "[+] Updated Antigravity profile router in: $profPath" -ForegroundColor Green
            } elseif ($existing -match 'function\s+agy\b') {
                Write-Host "[i] Custom 'function agy' already exists in: $profPath (preserved)" -ForegroundColor Gray
            } else {
                $separator = if ([string]::IsNullOrWhiteSpace($existing)) { "" } else { [Environment]::NewLine + [Environment]::NewLine }
                $newContent = $existing.TrimEnd() + $separator + $routerBlock.Trim() + [Environment]::NewLine
                [System.IO.File]::WriteAllText($profPath, $newContent, $utf8NoBom)
                Write-Host "[+] Injected Antigravity profile router into: $profPath" -ForegroundColor Green
            }
        }
    }
}

Write-Host "`n$eSparkles Installation complete!" -ForegroundColor Cyan
Write-Host "   Command configured: $titleCmd" -ForegroundColor Gray
Write-Host "   Installed directory: $targetDir" -ForegroundColor Gray
Write-Host "   Engine: Native Compiled Executable (< 3ms latency)" -ForegroundColor Green
Write-Host "   To uninstall later, run:" -ForegroundColor Gray
Write-Host "     powershell.exe -File `"$destUninstall`"" -ForegroundColor Gray
Write-Host "   Try asking Antigravity CLI a question to see the 4-state indicator in action:`n"
Write-Host "   $eReady Ready     -> Initial state / re-entering history" -ForegroundColor Gray
Write-Host "   $eWorking  Working   -> Thinking / background task execution" -ForegroundColor Yellow
Write-Host "   $eWaiting  Waiting   -> Prompting user (ask_question) or permission confirmation" -ForegroundColor Red
Write-Host "   $eDone Done      -> Turn completed; remains done until new input" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan
