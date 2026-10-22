# uninstall.ps1 - Uninstaller for Antigravity CLI Window Title (Windows/PowerShell)
param(
    [switch]$Purge
)

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "==========================================================" -ForegroundColor Blue
Write-Host "  🗑️ Uninstalling Antigravity CLI Window Title (Windows) " -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Blue

$homeDir = $env:USERPROFILE
if (-not $homeDir) { $homeDir = $env:HOME }

$settingsPath = Join-Path $homeDir ".gemini\antigravity-cli\settings.json"

if (Test-Path $settingsPath) {
    try {
        $raw = Get-Content -Raw -Path $settingsPath -Encoding UTF8
        $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop

        if ($settingsObj.PSObject.Properties['title']) {
            $settingsObj.PSObject.Properties.Remove('title')
            $updatedJson = $settingsObj | ConvertTo-Json -Depth 10
            [System.IO.File]::WriteAllText($settingsPath, $updatedJson, [System.Text.Encoding]::UTF8)
            Write-Host "[+] Removed 'title' section from: $settingsPath" -ForegroundColor Green
        } else {
            Write-Host "[i] No 'title' section found in settings.json." -ForegroundColor Gray
        }
    } catch {
        Write-Warning "[-] Failed to parse settings.json: $_"
    }
}

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"
$legacyDir = Join-Path $homeDir ".antigravity"

if ($Purge) {
    Set-Location $homeDir
    if (Test-Path $targetDir) {
        Remove-Item -Path $targetDir -Recurse -Force
        Write-Host "[+] Purged installation directory: $targetDir" -ForegroundColor Green
    }
    $legacyFile = Join-Path $legacyDir "title.ps1"
    if (Test-Path $legacyFile) {
        Remove-Item -Path $legacyFile -Force
        Write-Host "[+] Purged legacy file: $legacyFile" -ForegroundColor Gray
    }
} else {
    if (Test-Path $targetDir) {
        Write-Host "[i] Scripts preserved at: $targetDir (Use -Purge to completely remove directory)" -ForegroundColor Gray
    }
}

Write-Host "`n✨ Uninstallation completed." -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Blue
