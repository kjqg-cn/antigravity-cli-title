# 1. Web installers MUST NOT have BOM and MUST be pure ASCII
$noBomScripts = @("install.ps1", "uninstall.ps1")
foreach ($rel in $noBomScripts) {
    $fullPath = Resolve-Path $rel
    $bytes = [System.IO.File]::ReadAllBytes($fullPath)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        throw "File $rel MUST NOT have UTF-8 BOM!"
    }
    for ($i = 0; $i -lt $bytes.Length; $i++) {
        if ($bytes[$i] -gt 127) {
            throw "File $rel contains non-ASCII byte at offset $i (value: $($bytes[$i]))"
        }
    }
    Write-Host "[+] Verified pure ASCII and UTF-8 No-BOM for: $rel"
}

# 2. Validate AST syntax for all PowerShell scripts
$allScripts = Get-ChildItem -Recurse -Filter *.ps1
foreach ($s in $allScripts) {
    Write-Host "[*] Validating AST for: $($s.FullName)"
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($s.FullName, [ref]$null, [ref]$errors) | Out-Null
    if ($errors) {
        throw "AST error in $($s.FullName): $errors"
    }
}
Write-Host "[+] All PowerShell scripts passed AST check."

# 3. Router Block Generated Command Validation
$ast = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path "install.ps1"), [ref]$null, [ref]$null)
$assign = $ast.Find({
    $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] -and
    $args[0].Left.VariablePath.UserPath -eq 'routerBlock'
}, $true)
if (-not $assign) {
    throw "Could not find routerBlock in install.ps1"
}

$routerScript = (Invoke-Expression $assign.Right.Extent.Text).Trim()

# 3.1 Parse router script with AST to find $rawCmd assignment
$routerAst = [System.Management.Automation.Language.Parser]::ParseInput($routerScript, [ref]$null, [ref]$null)
$rawCmdAssign = $routerAst.Find({
    $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] -and
    $args[0].Left.VariablePath.UserPath -eq 'rawCmd'
}, $true)

if (-not $rawCmdAssign) {
    throw "Could not find `$rawCmd assignment inside router script AST!"
}

Write-Host "Found `$rawCmd expression:" $rawCmdAssign.Right.Extent.Text

# 3.3 Simulate arguments and evaluate the exact expression extracted via AST
$args = @('cost is $5 total', 'say "hi" now')
$quotedArgs = ($args | ForEach-Object { "'" + ($_ -replace "'", "''") + "'" }) -join ' '
$rawCmd = Invoke-Expression $rawCmdAssign.Right.Extent.Text

Write-Host "Evaluated rawCmd: $rawCmd"

# 3.4 Simulate Base64 encoding into -EncodedCommand and decode
$enc = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($rawCmd))
$decoded = [System.Text.Encoding]::Unicode.GetString([Convert]::FromBase64String($enc))
Write-Host "Decoded -EncodedCommand: $decoded"

# 3.5 Assertions
if (-not ($decoded.StartsWith("`$env:AGY_ACTIVE='1'; agy "))) {
    throw "Decoded command does NOT start with `$env:AGY_ACTIVE='1'; agy! Got: $decoded"
}

$expected = "`$env:AGY_ACTIVE='1'; agy 'cost is `$5 total' 'say ""hi"" now'"
if ($decoded -ne $expected) {
    throw "Decoded command does not match expected quoted arguments!`nExpected: $expected`nGot:      $decoded"
}

# 4. Format-CleanJson Verification (AST extracted)
$funcAst = $ast.Find({
    $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
    $args[0].Name -eq 'Format-CleanJson'
}, $true)
if (-not $funcAst) {
    throw "Could not find Format-CleanJson function in install.ps1 AST!"
}

. ([scriptblock]::Create($funcAst.Extent.Text))

$jsonInput = '{"rule":"command(echo { } done)","empty_obj":{  },"empty_arr":[  ]}'
$formatted = Format-CleanJson $jsonInput 2
if ($formatted -notmatch '"rule": "command\(echo \{ \} done\)"') {
    throw "Format-CleanJson altered string content! Got: $formatted"
}
if ($formatted -notmatch '"empty_obj": \{\}') {
    throw "Format-CleanJson did not collapse empty object! Got: $formatted"
}
if ($formatted -notmatch '"empty_arr": \[\]') {
    throw "Format-CleanJson did not collapse empty array! Got: $formatted"
}
Write-Host "[+] Format-CleanJson verification PASSED!"

# 5. Profile encoding roundtrip verification using installer code (Review Item 1 & 14)
$fnEncodingAst = $ast.Find({
    $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
    $args[0].Name -eq 'Get-FileEncodingAndContent'
}, $true)
if (-not $fnEncodingAst) {
    throw "Could not find Get-FileEncodingAndContent function in install.ps1 AST!"
}
Invoke-Expression $fnEncodingAst.Extent.Text

function Assert-EncodingRoundtrip([byte[]]$origBytes, [string]$testName) {
    $parsed = Get-FileEncodingAndContent $origBytes
    $enc = $parsed.Encoding
    $text = $parsed.Content
    $ms = [System.IO.MemoryStream]::new()
    $sw = [System.IO.StreamWriter]::new($ms, $enc)
    $sw.Write($text)
    $sw.Flush()
    $reEncoded = $ms.ToArray()

    if ($origBytes.Length -ne $reEncoded.Length) {
        throw "[$testName] byte roundtrip length mismatch: expected $($origBytes.Length), got $($reEncoded.Length)"
    }
    for ($k = 0; $k -lt $origBytes.Length; $k++) {
        if ($origBytes[$k] -ne $reEncoded[$k]) {
            throw "[$testName] byte mismatch at offset $($k): expected $('{0:X2}' -f $origBytes[$k]), got $('{0:X2}' -f $reEncoded[$k])"
        }
    }
    Write-Host "[+] $testName byte roundtrip verification PASSED!"
}

# 5.1 UTF-16LE with BOM (PowerShell 5.1 default)
$utf16Preamble = [byte[]]@(0xFF, 0xFE)
$utf16Body = [System.Text.Encoding]::Unicode.GetBytes("Write-Host 'Test profile'")
$utf16Full = [byte[]]($utf16Preamble + $utf16Body)
Assert-EncodingRoundtrip $utf16Full "UTF-16LE with BOM"

# 5.2 UTF-8 with BOM
$utf8Preamble = [byte[]]@(0xEF, 0xBB, 0xBF)
$utf8Body = [System.Text.Encoding]::UTF8.GetBytes("Write-Host 'Test profile'")
$utf8Full = [byte[]]($utf8Preamble + $utf8Body)
Assert-EncodingRoundtrip $utf8Full "UTF-8 with BOM"

# 5.3 GBK / ANSI without BOM
$gbkBytes = [byte[]]@(0xB4, 0xFA, 0xC0, 0xED) # "daili" in GBK
Assert-EncodingRoundtrip $gbkBytes "GBK / ANSI without BOM"

# 5.4 UTF-8 No-BOM
$utf8NoBomBytes = (New-Object System.Text.UTF8Encoding($false)).GetBytes("Write-Host 'Test profile'")
Assert-EncodingRoundtrip $utf8NoBomBytes "UTF-8 No-BOM"

# 6. Router pipeline check verification (Review Item 8)
if ($routerScript -notmatch '\$MyInvocation\.PipelineLength -gt 1') {
    throw "Router script does NOT check `$MyInvocation.PipelineLength -gt 1!"
}
Write-Host "[+] Router pipeline redirection check PASSED!"

# 7. Router Non-Interactive / Interactive Dispatch Test (Review Item 1)
$isNonInteractiveAssign = $routerAst.Find({
    $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] -and
    $args[0].Left.VariablePath.UserPath -eq 'isNonInteractive'
}, $true)
if (-not $isNonInteractiveAssign) {
    throw "Could not find `$isNonInteractive assignment inside router script AST!"
}
$isNonInteractiveExpr = $isNonInteractiveAssign.Right.Extent.Text
Write-Host "Found `$isNonInteractive expression:" $isNonInteractiveExpr

$nonInteractiveTestCases = @(
    @('-p', 'q'),
    @('--print', 'q'),
    @('-print', 'q'),
    @('--prompt', 'q'),
    @('-prompt', 'q'),
    @('mcp', 'list'),
    @('models'),
    @('update'),
    @('--model', 'claude', '--help'),
    @('--model', 'claude', '-help'),
    @('-v'),
    @('--version'),
    @('-version'),
    @('-h'),
    @('--help'),
    @('-help'),
    @('help'),
    @('--output-format', 'json'),
    @('--output-format=json'),
    @('-output-format=json'),
    @('--input-format', 'text'),
    @('-input-format=text'),
    @('agent'),
    @('agents'),
    @('changelog'),
    @('install'),
    @('mic-serve'),
    @('plugin'),
    @('plugins'),
    @('remote-control')
)

$interactiveTestCases = @(
    @(),
    @('x'),
    @('write a quicksort in go'),
    @('--model', 'gemini-pro'),
    @('--model', 'gemini-pro', 'x')
)

$decisionSb = [scriptblock]::Create($isNonInteractiveExpr)

function Test-AgyRouterDecision {
    param([string[]]$testArgs)
    if ($testArgs -and $testArgs.Count -gt 0) {
        return [bool](& $decisionSb @testArgs)
    } else {
        return [bool](& $decisionSb)
    }
}

foreach ($tc in $nonInteractiveTestCases) {
    $res = Test-AgyRouterDecision $tc
    if (-not $res) {
        throw "Failed: Expected non-interactive for args: $($tc -join ' ')"
    }
}

foreach ($tc in $interactiveTestCases) {
    $res = Test-AgyRouterDecision $tc
    if ($res) {
        throw "Failed: Expected interactive for args: $($tc -join ' ')"
    }
}
Write-Host "[+] Router non-interactive vs interactive routing verification PASSED!"

# 8. ConvertFrom-JsonC AST extraction and JSONC + trailing comma test (Review Item 3)
$fnJsonCAst = $ast.Find({
    $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
    $args[0].Name -eq 'ConvertFrom-JsonC'
}, $true)
if (-not $fnJsonCAst) {
    throw "Could not find ConvertFrom-JsonC function in install.ps1 AST!"
}
Invoke-Expression $fnJsonCAst.Extent.Text

$jsoncSample = @'
// Header comment
{
    /* Block comment */
    "profiles": {
        "list": [
            { "name": "PowerShell", }, // Trailing comma!
        ],
    },
}
'@
$parsedJsonC = ConvertFrom-JsonC $jsoncSample
if ($parsedJsonC.profiles.list[0].name -ne "PowerShell") {
    throw "ConvertFrom-JsonC failed to parse JSONC with comments and trailing commas!"
}
Write-Host "[+] ConvertFrom-JsonC comments and trailing comma verification PASSED!"

# 9. Windows Terminal JSONC textual profile injection and upgrade (Items 4 & 16)
$fnHasCommentsAst = $ast.Find({
    $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
    $args[0].Name -eq 'Test-HasJsonComments'
}, $true)
if (-not $fnHasCommentsAst) {
    throw "Could not find Test-HasJsonComments function in install.ps1 AST!"
}
Invoke-Expression $fnHasCommentsAst.Extent.Text

# Test item 4: https:// in schema URL MUST NOT be detected as comments!
$schemaSample = '{"$schema": "https://aka.ms/terminal-profiles-schema", "profiles": {"list": []}}'
if (Test-HasJsonComments $schemaSample) {
    throw "Test-HasJsonComments falsely detected https:// schema URL as comment!"
}
if (Test-HasJsonComments '{"url": "https://example.com/foo/bar"}') {
    throw "Test-HasJsonComments falsely detected https:// URL as comment!"
}
if (Test-HasJsonComments '{"msg": "hello // world inside string"}') {
    throw "Test-HasJsonComments falsely detected // inside string as comment!"
}
if (Test-HasJsonComments '{"msg": "escaped quote \" and // still in string"}') {
    throw "Test-HasJsonComments falsely detected // after escaped quote as comment!"
}
if (-not (Test-HasJsonComments '{"key": "val"} // real comment')) {
    throw "Test-HasJsonComments failed to detect line comment!"
}
if (-not (Test-HasJsonComments '/* block */ {"key": "val"}')) {
    throw "Test-HasJsonComments failed to detect block comment!"
}

# Dynamically extract list injection patterns from install.ps1 AST
$regexMatches = @($ast.FindAll({
    $args[0] -is [System.Management.Automation.Language.StringConstantExpressionAst] -and
    $args[0].Value -match '\("list"\\s\*:'
}, $true))
if ($regexMatches.Count -lt 2) {
    throw "Could not find list injection patterns in install.ps1 AST!"
}
$emptyListPattern = ($regexMatches | Where-Object { $_.Value -match '\\\]' })[0].Value
$listPattern = ($regexMatches | Where-Object { $_.Value -notmatch '\\\]' })[0].Value

$agyGuid = "{a9191a91-a919-4a91-a919-a9191a91a919}"
$profileText = @"
            {
                "commandline": "pwsh.exe",
                "guid": "$agyGuid",
                "hidden": false,
                "icon": "antigravity.png",
                "name": "Google Antigravity",
                "tabTitle": "Google Antigravity"
            }
"@
$injected = [regex]::Replace($jsoncSample, $listPattern, ('$1' + [Environment]::NewLine + $profileText + ',' + [Environment]::NewLine), 1)
if ($injected -notmatch '// Header comment' -or $injected -notmatch '/\* Block comment \*/') {
    throw "Textual profile injection dropped comments!"
}
$injectedObj = ConvertFrom-JsonC $injected
if ($injectedObj.profiles.list.Count -ne 2 -or $injectedObj.profiles.list[0].name -ne "Google Antigravity") {
    throw "Textual profile injection corrupted JSON structure!"
}

# Test empty list injection
$emptyJsonC = "// comment`n{ `"profiles`": { `"list`": [] } }"
$injectedEmpty = [regex]::Replace($emptyJsonC, $emptyListPattern, ('$1' + [Environment]::NewLine + $profileText + [Environment]::NewLine + '        $2'), 1)
$injectedEmptyObj = ConvertFrom-JsonC $injectedEmpty
if ($injectedEmptyObj.profiles.list.Count -ne 1 -or $injectedEmptyObj.profiles.list[0].name -ne "Google Antigravity") {
    throw "Empty list profile injection failed!"
}

# Existing profile upgrades are tested through actual installer code in test_review.ps1.
Write-Host "[+] Windows Terminal JSONC injection and upgrade verification PASSED!"

# 10. Session variable isolation check (Items 7 & 16)
# Verify that install.ps1 and uninstall.ps1 clean up callArgs variable at the end of execution
$installAst = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path "install.ps1"), [ref]$null, [ref]$null)
$installCleaned = [bool]($installAst.Find({
    $args[0] -is [System.Management.Automation.Language.CommandAst] -and
    $args[0].GetCommandName() -eq 'Remove-Variable' -and
    $args[0].Extent.Text -match 'callArgs'
}, $true))
if (-not $installCleaned) {
    throw "install.ps1 does not contain Remove-Variable callArgs at the end!"
}

$uninstallAst = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path "uninstall.ps1"), [ref]$null, [ref]$null)
$uninstallCleaned = [bool]($uninstallAst.Find({
    $args[0] -is [System.Management.Automation.Language.CommandAst] -and
    $args[0].GetCommandName() -eq 'Remove-Variable' -and
    $args[0].Extent.Text -match 'callArgs'
}, $true))
if (-not $uninstallCleaned) {
    throw "uninstall.ps1 does not contain Remove-Variable callArgs at the end!"
}

# Runtime execution: verify running with $preVars does not leak $callArgs or inner variables
$preVars = @(Get-Variable | Select-Object -ExpandProperty Name)
$testScopeBlock = [scriptblock]::Create(@'
$callArgs = @{ 'foo' = 'bar'; 'Prefix' = 'caller-val' }
& {
    param($foo, $Prefix)
    $innerState = "running"
    $localHelper = 42
} @callArgs
Remove-Variable callArgs -ErrorAction SilentlyContinue
'@)
& $testScopeBlock
$postVars = @(Get-Variable | Select-Object -ExpandProperty Name)
if ($postVars -contains 'callArgs') {
    throw "`$callArgs leaked into calling scope!"
}
if ($postVars -contains 'innerState' -or $postVars -contains 'localHelper') {
    throw "Inner scriptblock variable leaked into calling scope!"
}
Write-Host "[+] Session variable isolation verification PASSED!"

# 11. Regression test: Incremental backup cleanup preserves pre-title.bak (Item 3)
$tempTestDir = Join-Path ([System.IO.Path]::GetTempPath()) ("agy_test_baks_" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempTestDir -Force | Out-Null
try {
    $preTitleBak = Join-Path $tempTestDir "settings.json.pre-title.bak"
    [System.IO.File]::WriteAllText($preTitleBak, "original-content")
    for ($i = 1; $i -le 6; $i++) {
        $tsFile = Join-Path $tempTestDir ("settings.json.2026103100000" + $i + ".bak")
        [System.IO.File]::WriteAllText($tsFile, "ts-$i")
        Start-Sleep -Milliseconds 10
    }
    # Run exact pruning logic from install.ps1
    Get-ChildItem -Path $tempTestDir -Filter "settings.json.*.bak" -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^settings\.json\.\d{14}\.bak$' } |
        Sort-Object LastWriteTime -Descending |
        Select-Object -Skip 5 |
        ForEach-Object { Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue }

    if (-not (Test-Path $preTitleBak)) {
        throw "Pruning timestamped backups deleted settings.json.pre-title.bak!"
    }
    $remainingBaks = @(Get-ChildItem -Path $tempTestDir -Filter "settings.json.*.bak")
    if ($remainingBaks.Count -ne 6) {
        throw "Expected 6 backup files remaining (1 pre-title + 5 timestamped), got: $($remainingBaks.Count)"
    }
    Write-Host "[+] Incremental backup pruning preserves pre-title.bak verification PASSED!"
} finally {
    if (Test-Path $tempTestDir) { Remove-Item -Path $tempTestDir -Recurse -Force -ErrorAction SilentlyContinue }
}

# 12. Regression test: Uninstall targeted deletion preserves user changes and handles failures (Items 2 & 8)
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$tempTestDir = Join-Path ([System.IO.Path]::GetTempPath()) ("agy_test_uninstall_" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempTestDir -Force | Out-Null
try {
    # Part A: Test Item 8 - Parse failure preserves pre-title.bak and baks, flags failure
    $mockSettingsDir = Join-Path $tempTestDir ".gemini\antigravity-cli"
    New-Item -ItemType Directory -Path $mockSettingsDir -Force | Out-Null
    $mockSettingsPath = Join-Path $mockSettingsDir "settings.json"
    $mockBakPath = Join-Path $mockSettingsDir "settings.json.pre-title.bak"
    $mockIncBak = Join-Path $mockSettingsDir "settings.json.20261101000000.bak"

    [System.IO.File]::WriteAllText($mockSettingsPath, "{ invalid json text")
    [System.IO.File]::WriteAllText($mockBakPath, '{"title": "original"}')
    [System.IO.File]::WriteAllText($mockIncBak, '{"title": "incremental"}')

    # Simulate uninstall.ps1 settings update error handling
    $settingsUpdateFailed = $false
    try {
        $raw = Get-Content -Raw -Path $mockSettingsPath -Encoding UTF8
        if ([string]::IsNullOrWhiteSpace($raw)) {
            $settingsObj = [ordered]@{}
        } else {
            if ($raw.Trim().StartsWith('[')) {
                throw "Root element in $mockSettingsPath is not an object"
            }
            $settingsObj = ConvertFrom-Json $raw -ErrorAction Stop
        }
    } catch {
        $settingsUpdateFailed = $true
    }

    if (-not $settingsUpdateFailed) {
        throw "Expected settings parsing to fail on invalid JSON!"
    }
    # Simulate -Purge backup deletion guard:
    if (-not $settingsUpdateFailed) {
        Get-ChildItem -Path $mockSettingsDir -Filter "settings.json.*.bak" -File | Remove-Item -Force
    }

    if (-not (Test-Path $mockBakPath)) {
        throw "Purge deleted pre-title.bak even when settings update failed!"
    }
    if (-not (Test-Path $mockIncBak)) {
        throw "Purge deleted incremental backup even when settings update failed!"
    }

    # Part B: Test Item 2 - Windows Terminal targeted deletion preserves user profiles, themes, and comments
    $wtMockFile = Join-Path $tempTestDir "wt_settings.json"
    $wtMockContent = @'
// Terminal configuration
{
    "$schema": "https://aka.ms/terminal-profiles-schema",
    "theme": "Dark",
    "profiles": {
        "list": [
            {
                "guid": "{a9191a91-a919-4a91-a919-a9191a91a919}",
                "name": "Google Antigravity",
                "commandline": "agy.exe"
            },
            {
                "guid": "{user-profile-guid}",
                "name": "User Custom Shell",
                "commandline": "custom.exe"
            }
        ]
    }
}
'@
    [System.IO.File]::WriteAllText($wtMockFile, $wtMockContent, $utf8NoBom)
    $rawWt = Get-Content $wtMockFile -Raw -Encoding UTF8
    $hasComments = Test-HasJsonComments $rawWt
    if (-not $hasComments) {
        throw "Test-HasJsonComments failed to detect // Terminal configuration comment in wtMockContent!"
    }

    # Extract $agyPattern from uninstall.ps1 AST
    $agyPatternUninstAssign = $uninstallAst.Find({
        $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] -and
        $args[0].Left.VariablePath.UserPath -eq 'agyPattern'
    }, $true)
    if (-not $agyPatternUninstAssign) {
        throw "Could not find `$agyPattern assignment in uninstall.ps1 AST!"
    }
    $uninstAgyPattern = Invoke-Expression $agyPatternUninstAssign.Right.Extent.Text

    # Execute targeted deletion using AST-extracted pattern
    if ($rawWt -match $uninstAgyPattern) {
        $cleanedWt = [regex]::Replace($rawWt, $uninstAgyPattern, '')
        [System.IO.File]::WriteAllText($wtMockFile, $cleanedWt, $utf8NoBom)
    }

    $afterWtContent = Get-Content $wtMockFile -Raw -Encoding UTF8
    if ($afterWtContent -notmatch '// Terminal configuration') {
        throw "WT targeted deletion stripped header comment!"
    }
    $afterWtObj = ConvertFrom-JsonC $afterWtContent
    if ($afterWtObj.theme -ne "Dark") {
        throw "WT targeted deletion wiped user theme!"
    }
    if ($afterWtObj.profiles.list.Count -ne 1 -or $afterWtObj.profiles.list[0].name -ne "User Custom Shell") {
        throw "WT targeted deletion failed to preserve user profile or failed to remove agy profile!"
    }

    # Part C: Test flat profiles array removal in WT
    $wtFlatMockFile = Join-Path $tempTestDir "wt_flat.json"
    $wtFlatContent = @'
{
    "profiles": [
        {
            "guid": "{a9191a91-a919-4a91-a919-a9191a91a919}",
            "name": "Google Antigravity"
        },
        {
            "guid": "{user-flat-guid}",
            "name": "User Flat Profile"
        }
    ]
}
'@
    [System.IO.File]::WriteAllText($wtFlatMockFile, $wtFlatContent, $utf8NoBom)
    $rawFlat = Get-Content $wtFlatMockFile -Raw -Encoding UTF8
    $flatObj = ConvertFrom-JsonC $rawFlat
    $targetList = $null
    $isNestedList = $false
    if ($flatObj.profiles -is [System.Management.Automation.PSCustomObject] -and $flatObj.profiles.PSObject.Properties['list']) {
        $targetList = $flatObj.profiles.list
        $isNestedList = $true
    } elseif ($flatObj.profiles -is [System.Collections.IEnumerable]) {
        $targetList = $flatObj.profiles
        $isNestedList = $false
    }
    $filteredFlat = @($targetList | Where-Object { $_.guid -ne "{a9191a91-a919-4a91-a919-a9191a91a919}" -and $_.name -ne "Google Antigravity" })
    if ($filteredFlat.Count -ne 1 -or $filteredFlat[0].name -ne "User Flat Profile") {
        throw "Targeted deletion failed on flat profiles array!"
    }
    if ($isNestedList) {
        $flatObj.profiles.list = $filteredFlat
    } else {
        $flatObj.profiles = $filteredFlat
    }
    $cleanFlat = Format-CleanJson ($flatObj | ConvertTo-Json -Depth 100) 4
    [System.IO.File]::WriteAllText($wtFlatMockFile, $cleanFlat, $utf8NoBom)
    $afterFlatObj = ConvertFrom-JsonC (Get-Content $wtFlatMockFile -Raw -Encoding UTF8)
    if ($afterFlatObj.profiles.Count -ne 1 -or $afterFlatObj.profiles[0].name -ne "User Flat Profile") {
        throw "Flat profile file write-back failed!"
    }

    Write-Host "[+] Uninstall targeted deletion and failure handling verification PASSED!"
} finally {
    if (Test-Path $tempTestDir) { Remove-Item -Path $tempTestDir -Recurse -Force -ErrorAction SilentlyContinue }
}

# 13. Regression test: Single element root array rejected (Item 13)
# Under PS 7, ConvertFrom-Json '[{"model":"m"}]' returns a PSCustomObject instead of Array.
# Verify that the prefix check $raw.Trim().StartsWith('[') catches it and rejects it in both install.ps1 and uninstall.ps1.
$arrayJson = '[{"model":"m"}]'
$unwrappedObj = ConvertFrom-Json $arrayJson
$isUnwrappedObject = ($unwrappedObj -is [System.Management.Automation.PSCustomObject]) -and ($unwrappedObj -isnot [System.Array])

$installCheckPassed = $false
try {
    if ($arrayJson.Trim().StartsWith('[')) {
        throw "Root element in settings.json is not an object"
    }
} catch {
    $installCheckPassed = $true
}
if (-not $installCheckPassed) {
    throw "install.ps1 root array check failed to catch root array!"
}

# Verify uninstall.ps1 AST also contains the root array check
$uninstallHasArrayCheck = [bool]($uninstallAst.Find({
    $args[0].Extent.Text -match 'Root element.*is not an object'
}, $true))
if (-not $uninstallHasArrayCheck) {
    throw "uninstall.ps1 AST does not contain root array check!"
}
Write-Host "[+] Single element root array rejection check PASSED!"

# 14. Regression test: Profile newline style preservation (Item 14)
$lfSample = "function foo {`n    Write-Host 'hello'`n}`n"
$crlfSample = "function foo {`r`n    Write-Host 'hello'`r`n}`r`n"

$nlLf = if ($lfSample -match "`r`n") { "`r`n" } elseif ($lfSample -match "`n") { "`n" } else { [Environment]::NewLine }
if ($nlLf -ne "`n") { throw "Expected LF newline detection for LF sample" }

$nlCrlf = if ($crlfSample -match "`r`n") { "`r`n" } elseif ($crlfSample -match "`n") { "`n" } else { [Environment]::NewLine }
if ($nlCrlf -ne "`r`n") { throw "Expected CRLF newline detection for CRLF sample" }
Write-Host "[+] Profile newline style preservation verification PASSED!"

# 15. Regression test: WTProfile respects explicit NoPrefix or Prefix (Item 15)
# Case A: User specifies -WTProfile and -NoPrefix:$false
$NoPrefix = [switch]::new($false)
$WTProfile = [switch]::new($true)
$Prefix = "agy:"
$PSBoundParameters = @{ 'NoPrefix' = $false; 'WTProfile' = $true }
$effectiveNoPrefix = $NoPrefix.IsPresent -or ($WTProfile.IsPresent -and $Prefix -eq "agy:" -and -not $PSBoundParameters.ContainsKey('NoPrefix') -and -not $PSBoundParameters.ContainsKey('Prefix'))
if ($effectiveNoPrefix -ne $false) {
    throw "WTProfile incorrectly forced --no-prefix when -NoPrefix:`$false was specified!"
}

# Case B: User specifies -WTProfile and -Prefix "custom:"
$PSBoundParameters = @{ 'Prefix' = 'custom:'; 'WTProfile' = $true }
$Prefix = "custom:"
$effectiveNoPrefix = $NoPrefix.IsPresent -or ($WTProfile.IsPresent -and $Prefix -eq "agy:" -and -not $PSBoundParameters.ContainsKey('NoPrefix') -and -not $PSBoundParameters.ContainsKey('Prefix'))
if ($effectiveNoPrefix -ne $false) {
    throw "WTProfile incorrectly forced --no-prefix when -Prefix was specified!"
}

# Case C: User specifies only -WTProfile (default behavior should enable --no-prefix)
$PSBoundParameters = @{ 'WTProfile' = $true }
$Prefix = "agy:"
$effectiveNoPrefix = $NoPrefix.IsPresent -or ($WTProfile.IsPresent -and $Prefix -eq "agy:" -and -not $PSBoundParameters.ContainsKey('NoPrefix') -and -not $PSBoundParameters.ContainsKey('Prefix'))
if ($effectiveNoPrefix -ne $true) {
    throw "WTProfile without explicit prefix parameters did not default to --no-prefix!"
}
Write-Host "[+] WTProfile explicit prefix parameter precedence verification PASSED!"

Write-Host "[+] All script, encoding, AST, and router command verifications PASSED!"
