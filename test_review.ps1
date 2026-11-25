# Focused regressions run without changing installed settings or deleting fixtures.
$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$routerTests = [IO.File]::ReadAllText((Join-Path $root 'test_router.ps1'))
# Sections 11/12 delete their fixtures; run the non-deleting sections here.
$safeTests = $routerTests.Substring(0,$routerTests.IndexOf('# 11. Regression test:')) + $routerTests.Substring($routerTests.IndexOf('# 13. Regression test:'))
& ([scriptblock]::Create($safeTests))
$ast = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'install.ps1'), [ref]$null, [ref]$null)
foreach ($name in @('ConvertFrom-JsonC','Format-CleanJson','Get-FileEncodingAndContent','Test-HasJsonComments','Format-SymbolArg','Clean-SymbolValue','Update-TitleJson','Split-TitleCommand')) {
    $node = $ast.Find({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name }, $true)
    . ([scriptblock]::Create($node.Extent.Text))
}

# Exercise the installer's actual inheritance block with isolated parameter bindings.
$installSource = [IO.File]::ReadAllText((Join-Path $root 'install.ps1'))
$inheritStart = $installSource.IndexOf('if ($existingCmd -and')
$inheritEnd = $installSource.IndexOf('# 5. Build Title Command', $inheritStart)
$inheritBlock = [scriptblock]::Create($installSource.Substring($inheritStart,$inheritEnd-$inheritStart))
$inheritCases = @(
    @{ Flags='--animate=true'; Expected=$true },
    @{ Flags='--animate --animate=false'; Expected=$false },
    @{ Flags='--animate --no-animate'; Expected=$false },
    @{ Flags='--animate=false --animate'; Expected=$true },
    @{ Flags='--no-animate --animate=true'; Expected=$true },
    @{ Flags='--prefix "--animate=true"'; Expected=$false },
    @{ Flags='--prefix "--animate"'; Expected=$false; Bound=@{Prefix='override'} },
    @{ Flags='--working "--animate"'; Expected=$false; Bound=@{WorkingSymbol='override'} },
    @{ Flags='--prefix "--no-animate" --animate=true'; Expected=$true; Bound=@{Prefix='override'} },
    @{ Flags='--animate=false'; Expected=$true; Bound=@{Animate=$true} },
    @{ Flags='--animate=true'; Expected=$false; Bound=@{Animate=$false} },
    @{ Flags='--animate=true'; Expected=$false; NoAnimate=$true }
)
$inheritSetup = @'
param($case)
$existingCmd = 'C:/antigravity-cli-title/title.exe ' + $case.Flags
$PSBoundParameters = @{}
if ($case.Bound) { foreach ($key in $case.Bound.Keys) { $PSBoundParameters[$key]=$case.Bound[$key] } }
$Animate = [switch]($PSBoundParameters.ContainsKey('Animate') -and $PSBoundParameters['Animate'])
$NoAnimate = [switch][bool]$case.NoAnimate
'@
$inheritAssertion = 'if ($Animate.IsPresent -ne $case.Expected) { throw "Animation inheritance failed: $($case.Flags)" }'
$inheritTest = [scriptblock]::Create($inheritSetup + [Environment]::NewLine + $inheritBlock.ToString() + [Environment]::NewLine + $inheritAssertion)
foreach ($case in $inheritCases) { & $inheritTest $case }
Write-Host '[+] Boolean animation inheritance and explicit parameter precedence passed.'

$precise = '{"large":123456789012345678901234567890,"date":"2026-01-01T01:02:03Z","ratio":1.2345e-100,"title":{"command":"old"},"last":true}'
$updated = Update-TitleJson $precise '{"command":"new"}'
$prettyValue = '{' + [Environment]::NewLine + '    "command": "new"' + [Environment]::NewLine + '  }'
if ($updated -ne $precise.Replace('{"command":"old"}',' '+$prettyValue)) { throw 'Title update changed unrelated JSON bytes' }
foreach ($inputJson in @('{"title":1,"a":2}','{"a":2,"title":1}','{"title":1}')) {
    $removedTitle = Update-TitleJson $inputJson '' -Remove
    $check = ConvertFrom-Json $removedTitle
    if ($check.PSObject.Properties['title']) { throw 'Title removal failed' }
}

# Both install and uninstall restoration use the same nested indentation rules.
foreach ($scriptName in @('install.ps1','uninstall.ps1')) {
    $scriptAst = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path $root $scriptName),[ref]$null,[ref]$null)
    $updateAst = $scriptAst.Find({param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Update-TitleJson'},$true)
    & {
        param($updateAst,$scriptName)
        . ([scriptblock]::Create($updateAst.Extent.Text))
        foreach ($style in @(@{Indent='  ';NL="`n"},@{Indent='    ';NL="`r`n"},@{Indent="`t";NL="`n"})) {
            $indent=$style.Indent; $nl=$style.NL
            $raw='{'+$nl+$indent+'"other": 123456789012345678901234567890'+$nl+'}'+$nl
            $value='{"type":"command","nested":{"text":"literal } , \\"}}'
            $expectedBlock='"title": {'+$nl+($indent*2)+'"type": "command",'+$nl+($indent*2)+'"nested": {'+$nl+($indent*3)+'"text": "literal } , \\"'+$nl+($indent*2)+'}'+$nl+$indent+'}'
            $result=Update-TitleJson $raw $value
            if (-not $result.Contains($expectedBlock)) {throw "$scriptName insertion indentation mismatch: $result"}
            if (-not $result.StartsWith('{'+$nl+$indent+'"other": 123456789012345678901234567890,')) {throw 'Unrelated number or formatting changed'}
            $parsed=ConvertFrom-Json $result -ErrorAction Stop
            if ($parsed.title.nested.text -ne 'literal } , \') {throw 'String value changed'}
            $result=Update-TitleJson $result $value
            if (-not $result.Contains($expectedBlock) -or -not $result.EndsWith($nl+'}'+$nl)) {throw "$scriptName reinstall layout mismatch"}
        }
        $fresh=Update-TitleJson '' '{"type":"command"}'
        if (-not $fresh.Contains([Environment]::NewLine+'    "type": "command"'+[Environment]::NewLine+'  }')) {throw 'New settings indentation mismatch'}
    } $updateAst $scriptName
}
Write-Host '[+] Title indentation, line endings, nested values and restoration regressions passed.'

$sample = '{"profiles":{"defaults":{"font":{"face":"Custom Font"}}},"keybindings":[{"command":{"action":"sendInput","input":",} , ]"}}],}'
$parsed = ConvertFrom-JsonC $sample
if ($parsed.keybindings[0].command.input -ne ',} , ]') { throw 'JSONC changed a string literal' }
$symbol = Format-SymbolArg '--working' 'A"B''C'
if ($symbol -ne '--working "A\"B''C"') { throw "Symbol quoting failed: $symbol" }

# Evaluate the generated router, not a hand-copied quoting/pipeline implementation.
$assignment = $ast.Find({param($n) $n -is [System.Management.Automation.Language.AssignmentStatementAst] -and $n.Left.VariablePath.UserPath -eq 'routerBlock'},$true)
$router = Invoke-Expression $assignment.Right.Extent.Text
if (@($router.ToCharArray() | Where-Object { [int]$_ -gt 127 }).Count -ne 0) { throw 'Generated router must be ASCII for ANSI profiles' }
foreach ($codepage in @(936,1252)) {
    $encoding = [Text.Encoding]::GetEncoding($codepage)
    if ($encoding.GetString($encoding.GetBytes($router)) -ne $router) { throw "Router changed under code page $codepage" }
}
$routerAst = [System.Management.Automation.Language.Parser]::ParseInput($router,[ref]$null,[ref]$null)
$quoted = $routerAst.Find({param($n) $n -is [System.Management.Automation.Language.AssignmentStatementAst] -and $n.Left.VariablePath.UserPath -eq 'quotedArgs'},$true)
$quoteScript = [scriptblock]::Create($quoted.Right.Extent.Text)
$smart = 'it' + [char]0x2019 + 's valid'
$quotedText = & $quoteScript $smart
$value = & ([scriptblock]::Create('Write-Output '+$quotedText))
if ($value -ne $smart) { throw 'Smart quote argument did not round trip' }
$question = & $quoteScript 'what?'
if ((& ([scriptblock]::Create('Write-Output '+$question))) -ne 'what?') { throw 'Question mark argument was corrupted' }
$agyFunction = $routerAst.Find({param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'agy'},$true)
. ([scriptblock]::Create($agyFunction.Extent.Text))
function __agy_original { $input | ForEach-Object { 'forwarded:'+$_ } }
$forwarded = @('hello','world') | agy --print
if (($forwarded -join ',') -ne 'forwarded:hello,forwarded:world') { throw 'Router lost piped input' }

$fixture = Join-Path ([IO.Path]::GetTempPath()) ('agy_review_' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixture | Out-Null
$fakeAlias = Join-Path $fixture 'only-at-this-path.ps1'
[IO.File]::WriteAllText($fakeAlias, '$input | ForEach-Object { "alias:"+$_ }; "args:"+($args -join ",")', (New-Object Text.UTF8Encoding($false)))
$aliasBuilder = $routerAst.Find({param($n) $n -is [System.Management.Automation.Language.AssignmentStatementAst] -and $n.Left.VariablePath.UserPath -eq 'makeAliasWrapper'},$true)
$makeWrapper = Invoke-Expression $aliasBuilder.Right.Extent.Text
$wrapped = & $makeWrapper (Get-Command $fakeAlias) $fakeAlias
if ($wrapped.ToString() -notlike ('*'+$fakeAlias+'*')) { throw 'Alias wrapper dropped absolute path' }
$aliasResult = @('hello','world') | & $wrapped -p
if (($aliasResult -join ',') -ne 'alias:hello,alias:world,args:-p') { throw "Alias wrapper lost stdin or arguments: $aliasResult" }
$utf8NoBom = New-Object Text.UTF8Encoding($false)
$envRoot = Join-Path $fixture 'local'
$wtPath = Join-Path $envRoot 'Microsoft\Windows Terminal\settings.json'
New-Item -ItemType Directory -Path (Split-Path $wtPath) -Force | Out-Null
[IO.File]::WriteAllText($wtPath, $sample, $utf8NoBom)

# Execute the actual installer's WT section, stopping before profile router writes.
$source = [IO.File]::ReadAllText((Join-Path $root 'install.ps1'))
$start = $source.IndexOf('# 6. Windows Terminal Profile')
$end = $source.IndexOf('    if ($wtProfileSuccess)', $start)
$wtSection = $source.Substring($start, $end-$start) + "`n}"
$previousLocal = $env:LOCALAPPDATA
try {
    $env:LOCALAPPDATA = $envRoot
    $WTProfile = $true
    $assetsDir = Join-Path $fixture 'assets'
    & ([scriptblock]::Create($wtSection))
    $after = ConvertFrom-JsonC ([IO.File]::ReadAllText($wtPath))
    if ($after.profiles.defaults.font.face -ne 'Custom Font' -or @($after.profiles.list).Count -ne 1) { throw 'WT defaults-only object was damaged' }
    if ($after.keybindings[0].command.input -ne ',} , ]') { throw 'WT write changed user input' }

    # Updating must preserve custom fields and an unrelated same-name profile.
    $after.profiles.list[0] | Add-Member -NotePropertyName opacity -NotePropertyValue 81
    $after.profiles.list += [pscustomobject]@{guid='{other-user-guid}';name='Google Antigravity';commandline='custom.exe'}
    [IO.File]::WriteAllText($wtPath, (ConvertTo-Json $after -Depth 100), $utf8NoBom)
    & ([scriptblock]::Create($wtSection))
    $after = ConvertFrom-JsonC ([IO.File]::ReadAllText($wtPath))
    if ($after.profiles.list[0].opacity -ne 81 -or $after.profiles.list[1].commandline -ne 'custom.exe') { throw 'WT update lost user fields' }

    # Execute actual uninstall WT cleanup on invalid JSON: preserve both files.
    $uninstall = [IO.File]::ReadAllText((Join-Path $root 'uninstall.ps1'))
    $start = $uninstall.IndexOf('foreach ($p in $wtPaths)')
    $end = $uninstall.IndexOf('$rawBaseDirs =', $start)
    [IO.File]::WriteAllText($wtPath, '{ broken JSON', $utf8NoBom)
    $beforeBackup = [IO.File]::ReadAllText($wtPath+'.pre-title.bak')
    $wtPaths = @($wtPath)
    & ([scriptblock]::Create($uninstall.Substring($start,$end-$start)))
    if ([IO.File]::ReadAllText($wtPath) -ne '{ broken JSON' -or [IO.File]::ReadAllText($wtPath+'.pre-title.bak') -ne $beforeBackup) { throw 'Failed uninstall overwrote user data' }
} finally { $env:LOCALAPPDATA = $previousLocal }

Write-Host '[+] Review JSONC, profile preservation, symbol quoting and failure regressions passed.'

# Run the real non-WT installer in a child process with an isolated home directory.
$previousHome = $env:USERPROFILE
try {
    $env:USERPROFILE = Join-Path $fixture 'home'
    $shell = (Get-Process -Id $PID).Path
    & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'install.ps1') -Animate -NoPrefix
    if ($LASTEXITCODE -ne 0) { throw 'Isolated installer failed' }
    $settings = Join-Path $env:USERPROFILE '.gemini\antigravity-cli\settings.json'
    $installed = ConvertFrom-Json ([IO.File]::ReadAllText($settings))
    if ($installed.title.command -notmatch '--animate' -or $installed.title.command -notmatch '--no-prefix') { throw 'Animation installer flags were lost' }
    & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'install.ps1') -ShowPrefix
    if ($LASTEXITCODE -ne 0) { throw 'Isolated reinstall failed' }
    $installed = ConvertFrom-Json ([IO.File]::ReadAllText($settings))
    if ($installed.title.command -match '--no-prefix' -or $installed.title.command -notmatch '--animate') { throw 'ShowPrefix or inherited animation failed' }
    # Seed the same escaped/equals syntax that the renderer accepts, then reinstall.
    $installed.title.command = $installed.title.command + ' --working="A\"B''C" --prefix="My Bot"'
    [IO.File]::WriteAllText($settings, (ConvertTo-Json $installed -Depth 100), $utf8NoBom)
    & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'install.ps1') -NoAnimate
    if ($LASTEXITCODE -ne 0) { throw 'NoAnimate reinstall failed' }
    $installed = ConvertFrom-Json ([IO.File]::ReadAllText($settings))
    $tokens = @(Split-TitleCommand $installed.title.command)
    if ($installed.title.command -match '--animate' -or $tokens[-1] -ne 'A"B''C' -or $tokens -notcontains 'My Bot') { throw "Escaped symbol/prefix was not preserved: $($tokens -join '|')" }
    foreach ($case in @(
        @{Flags='--animate=true'; Expected=$true},
        @{Flags='--animate --animate=false'; Expected=$false},
        @{Flags='--animate --no-animate'; Expected=$false}
    )) {
        $installed.title.command = 'C:/antigravity-cli-title/title.exe ' + $case.Flags
        [IO.File]::WriteAllText($settings,(ConvertTo-Json $installed -Depth 100),$utf8NoBom)
        & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'install.ps1')
        if ($LASTEXITCODE -ne 0) { throw 'Boolean animation reinstall failed' }
        $installed = ConvertFrom-Json ([IO.File]::ReadAllText($settings))
        $tokens = @(Split-TitleCommand $installed.title.command)
        if (($tokens -contains '--animate') -ne $case.Expected) { throw "Reinstall changed animation setting: $($case.Flags)" }
    }
    # Evaluate actual uninstall settings logic without touching real profiles or deleting files.
    $installed.title.command = 'my-new-custom-title'
    [IO.File]::WriteAllText($settings,(ConvertTo-Json $installed -Depth 100),$utf8NoBom)
    $backupPath = Join-Path (Split-Path $settings) 'settings.json.pre-title.bak'
    [IO.File]::WriteAllText($backupPath,'{"title":{"command":"original-title"}}',$utf8NoBom)
    $before = [IO.File]::ReadAllText($settings)
    $settingsDir = Split-Path $settings
    $settingsPath = $settings
    $start = $uninstall.IndexOf('$bakPath = Join-Path $settingsDir')
    $end = $uninstall.IndexOf('$targetDir =', $start)
    & ([scriptblock]::Create($uninstall.Substring($start,$end-$start)))
    if ([IO.File]::ReadAllText($settings) -ne $before -or -not (Test-Path $backupPath)) { throw 'Uninstall overwrote a new user title' }
} finally { $env:USERPROFILE = $previousHome }
Write-Host "Fixtures preserved at: $fixture"
