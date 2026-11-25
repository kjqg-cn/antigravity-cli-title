# uninstall.ps1 - Uninstaller for Antigravity CLI Window Title (Windows/PowerShell)
param(
    [switch]$Purge
)

$callArgs = @{}
foreach ($k in $PSBoundParameters.Keys) { $callArgs[$k] = $PSBoundParameters[$k] }
$callArgs['InvokedScriptRoot'] = $PSScriptRoot
$callArgs['InvokedCommandPath'] = $PSCommandPath

& {
    [CmdletBinding()]
    param(
        [switch]$Purge,
        [string]$InvokedScriptRoot = "",
        [string]$InvokedCommandPath = ""
    )

    $origOutputEncoding = $OutputEncoding
    $origConsoleOutputEncoding = [Console]::OutputEncoding
    try {
        $OutputEncoding = [System.Text.Encoding]::UTF8
        [Console]::OutputEncoding = [System.Text.Encoding]::UTF8

    Write-Host "==========================================================" -ForegroundColor Blue
    $eTrash    = [char]::ConvertFromUtf32(0x1F5D1) + [char]0xFE0F
    $eSparkles = [char]::ConvertFromUtf32(0x2728)

    Write-Host "  $eTrash Uninstalling Antigravity CLI Window Title (Windows) " -ForegroundColor Yellow
    Write-Host "==========================================================" -ForegroundColor Blue

    $homeDir = $env:USERPROFILE
    if (-not $homeDir) { $homeDir = $env:HOME }
    $effectiveScriptRoot = if ($InvokedScriptRoot) { $InvokedScriptRoot } elseif ($PSScriptRoot) { $PSScriptRoot } else { "" }
    $isRemotePipeline = [string]::IsNullOrEmpty($effectiveScriptRoot)

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
    $settingsDir = Join-Path $homeDir ".gemini\antigravity-cli"
    $settingsPath = Join-Path $settingsDir "settings.json"

    function ConvertFrom-JsonC([string]$jsonc) {
        $sb = [System.Text.StringBuilder]::new()
        $inString = $false; $escape = $false; $len = $jsonc.Length
        for ($i = 0; $i -lt $len; $i++) {
            $c = $jsonc[$i]
            if ($inString) {
                [void]$sb.Append($c)
                if ($escape) { $escape = $false }
                elseif ($c -eq [char]92) { $escape = $true }
                elseif ($c -eq [char]34) { $inString = $false }
                continue
            }
            if ($c -eq [char]34) { $inString = $true; [void]$sb.Append($c); continue }
            if ($c -eq '/' -and $i + 1 -lt $len -and $jsonc[$i + 1] -eq '/') {
                $i += 2; while ($i -lt $len -and $jsonc[$i] -ne "`n") { $i++ }
                if ($i -lt $len) { [void]$sb.Append($jsonc[$i]) }
                continue
            }
            if ($c -eq '/' -and $i + 1 -lt $len -and $jsonc[$i + 1] -eq '*') {
                $i += 2; while ($i + 1 -lt $len -and -not ($jsonc[$i] -eq '*' -and $jsonc[$i + 1] -eq '/')) { $i++ }
                $i++; continue
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

$bakPath = Join-Path $settingsDir "settings.json.pre-title.bak"
$settingsUpdateFailed = $false

if (Test-Path $settingsPath) {
    try {
        $raw = (Get-FileEncodingAndContent ([System.IO.File]::ReadAllBytes($settingsPath))).Content
        if ([string]::IsNullOrWhiteSpace($raw)) {
            $settingsObj = [ordered]@{}
        } else {
            if ($raw.Trim().StartsWith('[')) {
                throw "Root element in $settingsPath is not an object (expected JSON object)."
            }
            $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop
            if ($settingsObj -is [System.Array] -or ($settingsObj -isnot [System.Management.Automation.PSCustomObject] -and $settingsObj -isnot [System.Collections.IDictionary])) {
                throw "Root element in $settingsPath is not an object (expected JSON object)."
            }
        }

        $currentCmd = if ($settingsObj.title -is [string]) { [string]$settingsObj.title } else { [string]$settingsObj.title.command }
        if ($currentCmd -notmatch 'antigravity-cli-title') {
            Write-Host '[i] Current title belongs to another configuration; settings and backup preserved.' -ForegroundColor Gray
        } else {
        $restoredOldTitle = $false
        if (Test-Path $bakPath) {
            try {
                $rawBak = Get-Content -Raw -Path $bakPath -Encoding UTF8
                $bakObj = ConvertFrom-Json $rawBak -ErrorAction SilentlyContinue
                if ($bakObj -and $bakObj.PSObject.Properties['title'] -and $bakObj.title) {
                    $bakCmd = if ($bakObj.title.PSObject.Properties['command']) { [string]$bakObj.title.command } else { [string]$bakObj.title }
                    if ($bakCmd -and $bakCmd -notmatch 'antigravity-cli-title') {
                        $settingsObj | Add-Member -NotePropertyName "title" -NotePropertyValue $bakObj.title -Force
                        $updatedJson = Update-TitleJson $raw (Format-CleanJson ($bakObj.title | ConvertTo-Json -Depth 100) 2)
                        [System.IO.File]::WriteAllText($settingsPath, $updatedJson, $utf8NoBom)
                        Write-Host "[+] Restored previous 'title' configuration from backup: $bakPath" -ForegroundColor Green
                        $restoredOldTitle = $true
                    } else {
                        Write-Host "[i] Backup 'title' points to antigravity-cli-title, skipping restore." -ForegroundColor Gray
                    }
                }
            } catch {}
        }

        if (-not $restoredOldTitle) {
            if ($settingsObj.PSObject.Properties['title']) {
                $settingsObj.PSObject.Properties.Remove('title')
                $updatedJson = Update-TitleJson $raw "" -Remove
                [System.IO.File]::WriteAllText($settingsPath, $updatedJson, $utf8NoBom)
                Write-Host "[+] Removed 'title' section from: $settingsPath" -ForegroundColor Green
            } else {
                Write-Host "[i] No 'title' section found in settings.json." -ForegroundColor Gray
            }
        }

        if (Test-Path $bakPath) { Remove-Item $bakPath -Force -ErrorAction SilentlyContinue }
        }
    } catch {
        $settingsUpdateFailed = $true
        Write-Warning "[-] Failed to parse settings.json: $_"
    }
}

$targetDir = Join-Path $homeDir ".gemini\antigravity-cli-title"

if ($Purge -and -not $settingsUpdateFailed) {
    Push-Location $homeDir
    try {
        $targetExe = Join-Path $targetDir "title.exe"
        Get-Process -Name title -ErrorAction SilentlyContinue | Where-Object {
            try {
                $_.Path -and ([System.IO.Path]::GetFullPath($_.Path) -eq [System.IO.Path]::GetFullPath($targetExe))
            } catch { $false }
        } | Stop-Process -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 200
        if (Test-Path $targetDir) {
            try {
                Remove-Item -Path $targetDir -Recurse -Force -ErrorAction Stop
                Write-Host "[+] Purged installation directory: $targetDir" -ForegroundColor Green
            } catch {
                Write-Warning "[-] Could not completely remove ${targetDir}: $_"
            }
        }
        if (-not $settingsUpdateFailed) {
            Get-ChildItem -Path $settingsDir -Filter "settings.json.*.bak" -File -ErrorAction SilentlyContinue |
                Where-Object { $_.Name -match '^settings\.json\.\d{14}\.bak$' } |
                ForEach-Object { Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue }
        }
    } finally {
        Pop-Location -ErrorAction SilentlyContinue
    }
} else {
    if (Test-Path $targetDir) {
        Write-Host "[i] Binary files preserved at: $targetDir (Use -Purge to completely remove directory)" -ForegroundColor Gray
    }
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
            $hasComments = Test-HasJsonComments $rawWt
            $agyGuid = "{a9191a91-a919-4a91-a919-a9191a91a919}"
            $agyPattern = '(?s)(,\s*)?\{\s*[^{}]*?(?:"guid"\s*:\s*"(?i:\{a9191a91-a919-4a91-a919-a9191a91a919\})")[^{}]*?\}\s*(?(1)|,?)'
            $removed = $false
            if ($hasComments) {
                if ($rawWt -match $agyPattern) {
                    $cleanedWt = [regex]::Replace($rawWt, $agyPattern, '')
                    $testObj = ConvertFrom-JsonC $cleanedWt
                    if ($testObj) {
                        [System.IO.File]::WriteAllText($p, $cleanedWt, $utf8NoBom)
                        Write-Host "[+] Removed 'Google Antigravity' profile from: $p (JSONC comments preserved)" -ForegroundColor Green
                        $removed = $true
                    }
                }
            }
            if (-not $removed) {
                try {
                    $wtObj = ConvertFrom-JsonC $rawWt -ErrorAction Stop
                    $targetList = $null
                    $isNestedList = $false
                    if ($wtObj.profiles -is [System.Management.Automation.PSCustomObject] -and $wtObj.profiles.PSObject.Properties['list']) {
                        $targetList = $wtObj.profiles.list
                        $isNestedList = $true
                    } elseif ($wtObj.profiles -is [System.Collections.IEnumerable]) {
                        $targetList = $wtObj.profiles
                        $isNestedList = $false
                    }
                    if ($targetList) {
                        $filtered = @($targetList | Where-Object { $_.guid -ne $agyGuid })
                        if ($filtered.Count -ne $targetList.Count) {
                            if ($isNestedList) {
                                $wtObj.profiles.list = $filtered
                            } else {
                                $wtObj.profiles = $filtered
                            }
                            $cleanWt = Format-CleanJson ($wtObj | ConvertTo-Json -Depth 100) 4
                            [System.IO.File]::WriteAllText($p, $cleanWt, $utf8NoBom)
                            Write-Host "[+] Removed 'Google Antigravity' profile from: $p" -ForegroundColor Green
                        }
                    }
                    $removed = $true
                } catch {
                    Write-Warning "[-] Cannot safely update Windows Terminal settings at '$p'. File and backup preserved: $_"
                    $settingsUpdateFailed = $true
                }
            }
            $wtBak = $p + ".pre-title.bak"
            if ($removed -and (Test-Path $wtBak)) {
                Remove-Item $wtBak -Force -ErrorAction SilentlyContinue
            }
        } catch {}
    }
}

$rawBaseDirs = @(
    [Environment]::GetFolderPath('MyDocuments'),
    (Join-Path $homeDir 'Documents'),
    (Join-Path $homeDir 'OneDrive\Documents'),
    (Join-Path $homeDir "OneDrive\$([char]0x6587)$([char]0x6863)")
)
$validBaseDirs = @($rawBaseDirs | Where-Object { -not [string]::IsNullOrWhiteSpace($_) -and (Test-Path $_) } | Select-Object -Unique)

$candidateProfiles = [System.Collections.Generic.List[string]]::new()
if ($PROFILE) {
    $candidateProfiles.Add([System.IO.Path]::GetFullPath($PROFILE))
}

foreach ($baseDir in $validBaseDirs) {
    $candidateProfiles.Add([System.IO.Path]::GetFullPath((Join-Path $baseDir 'PowerShell\Microsoft.PowerShell_profile.ps1')))
    $candidateProfiles.Add([System.IO.Path]::GetFullPath((Join-Path $baseDir 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')))
}

$uniqueProfiles = @($candidateProfiles | Select-Object -Unique)

foreach ($profPath in $uniqueProfiles) {
    if (Test-Path $profPath) {
        try {
            $bytes = [System.IO.File]::ReadAllBytes($profPath)
            $parsed = Get-FileEncodingAndContent $bytes
            $profEncoding = $parsed.Encoding
            $content = $parsed.Content
            $profBak = $profPath + ".pre-title.bak"
            if ($content -match '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<') {
                $cleaned = ($content -replace '# >>> antigravity-cli-title profile router >>>[\s\S]*?# <<< antigravity-cli-title profile router <<<', '').Trim()
                if ([string]::IsNullOrWhiteSpace($cleaned)) {
                    [System.IO.File]::WriteAllText($profPath, "", $profEncoding)
                    Write-Host "[+] Removed profile router; preserved profile file: $profPath" -ForegroundColor Green
                } else {
                    $nl = if ($content -match "`r`n") { "`r`n" } elseif ($content -match "`n") { "`n" } else { [Environment]::NewLine }
                    $finalText = $cleaned + $nl
                    [System.IO.File]::WriteAllText($profPath, $finalText, $profEncoding)
                    Write-Host "[+] Removed Antigravity profile router from: $profPath" -ForegroundColor Green
                }
            }
            if (Test-Path $profBak) { Remove-Item $profBak -Force -ErrorAction SilentlyContinue }
        } catch {}
    }
}

if ($settingsUpdateFailed) {
    Write-Warning "`n[-] Warning: settings.json could not be updated. Please check manually."
    Write-Host "==========================================================" -ForegroundColor Blue
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
    if ($isRemotePipeline) { throw "Uninstallation incomplete; configuration was preserved." }
    exit 1
}

Write-Host "`n$eSparkles Uninstallation completed." -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Blue

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
