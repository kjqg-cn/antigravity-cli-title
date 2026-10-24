# uninstall.ps1 - Uninstaller for Antigravity CLI Window Title (Windows/PowerShell)
param(
    [switch]$Purge
)

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "==========================================================" -ForegroundColor Blue
$eTrash    = [char]::ConvertFromUtf32(0x1F5D1) + [char]0xFE0F
$eSparkles = [char]::ConvertFromUtf32(0x2728)

Write-Host "  $eTrash Uninstalling Antigravity CLI Window Title (Windows) " -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Blue

$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$settingsPath = Join-Path $homeDir ".gemini\antigravity-cli\settings.json"

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

if (Test-Path $settingsPath) {
    try {
        $raw = Get-Content -Raw -Path $settingsPath -Encoding UTF8
        $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop

        if ($settingsObj.PSObject.Properties['title']) {
            $settingsObj.PSObject.Properties.Remove('title')
            $updatedJson = Format-CleanJson ($settingsObj | ConvertTo-Json -Depth 10) 2
            $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
            [System.IO.File]::WriteAllText($settingsPath, $updatedJson, $utf8NoBom)
            Write-Host "[+] Removed 'title' section from: $settingsPath" -ForegroundColor Green
        } else {
            Write-Host "[i] No 'title' section found in settings.json." -ForegroundColor Gray
        }
    } catch {
        Write-Warning "[-] Failed to parse settings.json: $_"
    }
}

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"

if ($Purge) {
    Set-Location $homeDir
    if (Test-Path $targetDir) {
        Remove-Item -Path $targetDir -Recurse -Force
        Write-Host "[+] Purged installation directory: $targetDir" -ForegroundColor Green
    }

    $wtPaths = @(
        (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json'),
        (Join-Path $env:LOCALAPPDATA 'Packages\Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe\LocalState\settings.json'),
        (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows Terminal\settings.json')
    )
    foreach ($p in $wtPaths) {
        if (Test-Path $p) {
            try {
                $rawWt = Get-Content $p -Raw -Encoding UTF8
                $wtObj = ConvertFrom-Json $rawWt -ErrorAction Stop
                $filtered = @($wtObj.profiles.list | Where-Object { $_.guid -ne "{a9191a91-a919-4a91-a919-a9191a91a919}" -and $_.name -ne "Google Antigravity" })
                if ($filtered.Count -ne $wtObj.profiles.list.Count) {
                    $wtObj.profiles.list = $filtered
                    $cleanWt = Format-CleanJson ($wtObj | ConvertTo-Json -Depth 10) 4
                    [System.IO.File]::WriteAllText($p, $cleanWt, $utf8NoBom)
                    Write-Host "[+] Removed 'Google Antigravity' profile from: $p" -ForegroundColor Green
                }
            } catch {}
        }
    }

    $docsDir = [Environment]::GetFolderPath('MyDocuments')
    $candidateProfiles = @(
        (Join-Path $docsDir 'PowerShell\Microsoft.PowerShell_profile.ps1'),
        (Join-Path $docsDir 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')
    )
    foreach ($profPath in $candidateProfiles) {
        if (Test-Path $profPath) {
            try {
                $content = Get-Content $profPath -Raw -Encoding UTF8
                if ($content -match '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<') {
                    $cleaned = ($content -replace '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<', '').Trim()
                    $finalText = if ([string]::IsNullOrWhiteSpace($cleaned)) { "" } else { $cleaned + [Environment]::NewLine }
                    [System.IO.File]::WriteAllText($profPath, $finalText, $utf8NoBom)
                    Write-Host "[+] Removed Antigravity profile router from: $profPath" -ForegroundColor Green
                }
            } catch {}
        }
    }
} else {
    if (Test-Path $targetDir) {
        Write-Host "[i] Files preserved at: $targetDir (Use -Purge to completely remove directory)" -ForegroundColor Gray
    }
}

Write-Host "`n$eSparkles Uninstallation completed." -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Blue
