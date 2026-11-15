# SplitLane: the check of the installer on a CI runner (spec 7 of stage 2). In this order: the stage 1 layout
# (the six files of the helper at the root of %ProgramFiles%\SplitLane); the silent install over it, then the
# files, the task and its command, setup.json, the access list of the data folder, the Electron fuses and the
# version info of the installed SplitLane.exe; the install over the installed product, with marker files in
# %ProgramData%\SplitLane and %AppData%\SplitLane that must stay; the silent uninstall without /PURGE (the
# data stays); the install and the silent uninstall with /PURGE (the data goes). The helper is never started
# and the network is not touched.
# It changes the machine: %ProgramFiles%\SplitLane, %ProgramData%\SplitLane, %AppData%\SplitLane of this user,
# the task "SplitLane Helper", the uninstall entry and the shortcuts. So it runs only as an administrator on a
# machine without SplitLane and without Tailscale (the runner of GitHub Actions); anywhere else it stops with
# TEST-REFUSED before it changes anything. Never run it on a PC where SplitLane is installed.
# Needs what scripts\build-installer.ps1 leaves: dist\helper (the six files) and app\node_modules (the order
# of the fuses is that of @electron/fuses as electron-builder resolves it), and node in PATH.
# The NSIS uninstaller copies itself to %TEMP% and starts that copy, which removes the files; the exit code
# of the copy is not seen, so the test waits until the folder, the uninstall entry and the copy are gone.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\test-installer.ps1 -Installer <path>
# Result: last line TEST-OK, TEST-FAIL <step>: <what> (exit code 1) or TEST-REFUSED: <reason> (exit code 2).
param([string]$Installer = '')
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root 'dist\helper'
$taskName = 'SplitLane Helper'
$product = Join-Path $env:ProgramFiles 'SplitLane'
$helper = Join-Path $product 'helper'
$data = Join-Path $env:ProgramData 'SplitLane'
$userData = Join-Path $env:APPDATA 'SplitLane'
$marker = 'splitlane-test-marker.txt'
# install.Files: the helper's files, in helper\ now and at the root in the stage 1 layout
$six = 'splitlane-helper.exe', 'sing-box.exe', 'libcronet.dll', 'sing-box-LICENSE', 'LICENSE', 'THIRD-PARTY.md'
$taskCommand = Join-Path $helper 'splitlane-helper.exe'
$sentinel = 'dL7pKGdnNz796PbbjQWNKmHXBZaB9tsX'
# Global Constraints: the fuses of electron-builder.yml by their names in @electron/fuses; '0' off, '1' on
$fuses = [ordered]@{
    RunAsNode                            = '0'
    EnableNodeOptionsEnvironmentVariable = '0'
    EnableNodeCliInspectArguments        = '0'
    OnlyLoadAppFromAsar                  = '1'
}
$script:step = 'start'

function Say([string]$text) { Write-Host "  ok: $text" }

# A native command with its output (stderr too) as lines and its exit code; stderr is no error here.
function Invoke-Native([string]$exe, [string[]]$arguments) {
    $ErrorActionPreference = 'Continue'
    $lines = @(& $exe @arguments 2>&1 | ForEach-Object { "$_" })
    return [pscustomobject]@{ Code = $LASTEXITCODE; Lines = $lines }
}

# The task, or $null when there is none; any other error of the task scheduler is thrown.
function Get-Task {
    $service = New-Object -ComObject Schedule.Service
    $service.Connect()
    try {
        return $service.GetFolder('\').GetTask($taskName)
    } catch {
        $e = $_.Exception
        while ($e.InnerException) { $e = $e.InnerException }
        if ($e.HResult -eq -2147024894) { return $null } # 0x80070002, ERROR_FILE_NOT_FOUND
        throw
    }
}

# The registry paths of the uninstall entries called SplitLane (both registry views).
function Get-UninstallEntries {
    $keys = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall'
    foreach ($key in $keys) {
        if (-not (Test-Path -LiteralPath $key)) { continue }
        foreach ($sub in Get-ChildItem -LiteralPath $key) {
            $p = Get-ItemProperty -LiteralPath $sub.PSPath -ErrorAction SilentlyContinue
            if ($p -and $p.DisplayName -eq 'SplitLane') { $sub.PSPath }
        }
    }
}

# The copy of the NSIS uninstaller that runs from %TEMP%\~nsu*.tmp (Un_A.exe; Au_.exe in NSIS 2).
function Get-UninstallerCopies { @(Get-Process | Where-Object { $_.ProcessName -match '^(Un_[A-Z]|Au_)$' }) }

# Empty when the machine may run the test, else why not. Nothing is changed before this passes.
function Get-Refusal {
    if (Test-Path -LiteralPath $product) { return "$product is there: SplitLane is installed here" }
    $me = New-Object System.Security.Principal.WindowsPrincipal([System.Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $me.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)) {
        return 'this process is not elevated: the test runs as an administrator'
    }
    if (Get-Task) { return "the task $taskName is there: SplitLane is installed here" }
    if (@(Get-UninstallEntries).Count -gt 0) { return 'an uninstall entry SplitLane is there' }
    if (Test-Path -LiteralPath $data) { return "$data is there; the purge step would remove it" }
    if (Test-Path -LiteralPath $userData) { return "$userData is there; the purge step would remove it" }
    if ((Test-Path -LiteralPath (Join-Path $env:ProgramFiles 'Tailscale')) -or (Get-Service -Name 'Tailscale' -ErrorAction SilentlyContinue)) {
        return 'Tailscale is installed: the test runs on a machine without it'
    }
    return ''
}

# install.DataSDDL from the helper's source, the access list of the data folder.
function Get-DataSddl {
    $src = Get-Content -LiteralPath (Join-Path $root 'helper\internal\install\install.go') -Raw
    if ($src -notmatch '(?m)^const DataSDDL\s*=\s*"([^"]+)"') { throw 'no const DataSDDL in helper\internal\install\install.go' }
    return $Matches[1]
}

# The fuse order: FuseV1Options of the @electron/fuses that app-builder-lib of electron-builder resolves
# (the one that wrote the fuses), and its version.
function Get-FuseOrder {
    $js = "const p=require('path');const dir=(n,d)=>p.dirname(require.resolve(n+'/package.json',{paths:[d]}));" +
        "const f=dir('@electron/fuses',dir('app-builder-lib',dir('electron-builder',process.cwd())));" +
        "console.log(JSON.stringify({version:require(p.join(f,'package.json')).version,options:require(f).FuseV1Options}))"
    Push-Location (Join-Path $root 'app')
    try { $r = Invoke-Native 'node' @('-e', $js) } finally { Pop-Location }
    $json = @($r.Lines | Where-Object { $_.StartsWith('{') })
    if ($r.Code -ne 0 -or $json.Count -ne 1) {
        throw "@electron/fuses not found from app\node_modules (run scripts\build-installer.ps1 first): $($r.Lines -join ' ')"
    }
    return $json[0] | ConvertFrom-Json
}

# The ACEs of an SDDL that matter for the comparison: type, flags without INHERITED_ACE, mask, SID; sorted.
function Get-AceKeys($descriptor) {
    $inherited = [int][System.Security.AccessControl.AceFlags]::Inherited
    @(foreach ($ace in $descriptor.DiscretionaryAcl) {
        '{0};0x{1:x2};0x{2:x8};{3}' -f $ace.AceType, ([int]$ace.AceFlags -band (-bnot $inherited)), $ace.AccessMask, $ace.SecurityIdentifier.Value
    }) | Sort-Object
}

# Empty when the access list $got (SDDL, as Get-Acl gives it) is $want, else why not. Compared: that the
# list is protected when $want is (P), no inherited entry, and the same entries in any order. Not compared:
# the owner and the group (not part of $want), AI (auto-inherited, set by Windows), the order of the entries.
function Compare-Dacl([string]$got, [string]$want) {
    $g = New-Object System.Security.AccessControl.RawSecurityDescriptor($got)
    $w = New-Object System.Security.AccessControl.RawSecurityDescriptor($want)
    if ($null -eq $g.DiscretionaryAcl) { return 'it has no access list' }
    $protected = [int][System.Security.AccessControl.ControlFlags]::DiscretionaryAclProtected
    if ((([int]$w.ControlFlags -band $protected) -ne 0) -and (([int]$g.ControlFlags -band $protected) -eq 0)) {
        return "it is not protected (inherits from its parent): $got"
    }
    foreach ($ace in $g.DiscretionaryAcl) {
        if ($ace.IsInherited) { return "it has an inherited entry: $got" }
    }
    $gk = (Get-AceKeys $g) -join ' '
    $wk = (Get-AceKeys $w) -join ' '
    if ($gk -ne $wk) { return "its entries are [$gk], want [$wk] ($want)" }
    return ''
}

# Runs a program and waits for it; its exit code.
function Invoke-Program([string]$exe, [string]$arguments, [int]$seconds) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $exe
    $psi.Arguments = $arguments
    $psi.UseShellExecute = $false
    $p = [System.Diagnostics.Process]::Start($psi)
    try {
        if (-not $p.WaitForExit($seconds * 1000)) {
            try { $p.Kill() } catch { }
            throw "$(Split-Path -Leaf $exe) $arguments did not end within $seconds s"
        }
        return $p.ExitCode
    } finally {
        $p.Dispose()
    }
}

# After a failed setup only: what the helper says when its step runs again (the silent installer shows
# no message box, so its reason is not seen otherwise).
function Show-HelperOutput([string]$exe, [string]$arguments) {
    try {
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $exe
        $psi.Arguments = $arguments
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $p = [System.Diagnostics.Process]::Start($psi)
        $out = $p.StandardOutput.ReadToEnd() + $p.StandardError.ReadToEnd()
        $p.WaitForExit()
        foreach ($line in $out -split "`r?`n") {
            if ($line) { Write-Host "  diag: splitlane-helper.exe ${arguments}: $line" }
        }
        Write-Host "  diag: splitlane-helper.exe ${arguments}: exit code $($p.ExitCode)"
    } catch {
        Write-Host "  diag: splitlane-helper.exe ${arguments}: $($_.Exception.Message)"
    }
}

function Install-Product {
    $code = Invoke-Program $Installer '/S' 600
    if ($code -ne 0) {
        Show-HelperOutput (Join-Path $dist 'splitlane-helper.exe') 'prepare --session-user'
        if (Test-Path -LiteralPath $taskCommand) { Show-HelperOutput $taskCommand 'install --session-user' }
        throw "the installer /S ended with exit code $code"
    }
    Say 'the installer /S ended with exit code 0'
}

function Uninstall-Product([string]$arguments) {
    $uninstaller = Join-Path $product 'Uninstall SplitLane.exe'
    if (-not (Test-Path -LiteralPath $uninstaller -PathType Leaf)) { throw "no $uninstaller" }
    $entries = @(Get-UninstallEntries)
    if ($entries.Count -eq 0) { throw 'no uninstall entry SplitLane before the uninstall' }
    $deadline = (Get-Date).AddSeconds(120)
    $code = Invoke-Program $uninstaller $arguments 120
    if ($code -ne 0) { throw "Uninstall SplitLane.exe $arguments ended with exit code $code" }
    while ($true) {
        $left = @()
        if (Test-Path -LiteralPath $product) { $left += $product }
        if (@($entries | Where-Object { Test-Path -LiteralPath $_ }).Count -gt 0) { $left += 'the uninstall entry SplitLane' }
        if ((Get-UninstallerCopies).Count -gt 0) { $left += 'the uninstaller copy in %TEMP%' }
        if ($left.Count -eq 0) { break }
        if ((Get-Date) -gt $deadline) {
            $files = ''
            if (Test-Path -LiteralPath $product) {
                $files = '; left in it: ' + ((@(Get-ChildItem -LiteralPath $product -Recurse -Force |
                    ForEach-Object { $_.FullName.Substring($product.Length + 1) }) | Select-Object -First 20) -join ', ')
            }
            $task = 'gone'
            if (Get-Task) { $task = 'still there' }
            throw "Uninstall SplitLane.exe ${arguments}: after 120 s still there: $($left -join ', '); the task is $task$files"
        }
        Start-Sleep -Milliseconds 500
    }
    if (@(Get-UninstallEntries).Count -gt 0) { throw 'an uninstall entry SplitLane is still there' }
    Say "Uninstall SplitLane.exe $arguments ended (exit code 0) and its copy removed $product and the uninstall entry"
}

function Assert-Task {
    $task = Get-Task
    if (-not $task) { throw "no task $taskName" }
    $actions = @(foreach ($a in $task.Definition.Actions) { $a })
    if ($actions.Count -ne 1 -or $actions[0].Type -ne 0) { throw "the task $taskName has $($actions.Count) actions, want one program" }
    if ($actions[0].Path -ne $taskCommand) { throw "the task $taskName runs '$($actions[0].Path)', want '$taskCommand'" }
    if ($actions[0].Arguments) { throw "the task $taskName runs with arguments '$($actions[0].Arguments)', want none" }
    Say "the task $taskName runs $taskCommand"
}

function Assert-Fuses([string]$exe, $order) {
    $bytes = [System.IO.File]::ReadAllBytes($exe)
    $text = [System.Text.Encoding]::GetEncoding(28591).GetString($bytes) # one char per byte
    $at = $text.IndexOf($sentinel, [System.StringComparison]::Ordinal)
    if ($at -lt 0) { throw "SplitLane.exe has no fuse sentinel $sentinel" }
    if ($text.LastIndexOf($sentinel, [System.StringComparison]::Ordinal) -ne $at) { throw 'SplitLane.exe has more than one fuse sentinel' }
    $pos = $at + $sentinel.Length
    if ($pos + 2 -gt $bytes.Length) { throw 'the fuse wire runs past the end of SplitLane.exe' }
    $version = $bytes[$pos]
    $length = $bytes[$pos + 1]
    if ($version -ne 1) { throw "the fuse wire has version $version; @electron/fuses $($order.version) writes version 1" }
    if ($pos + 2 + $length -gt $bytes.Length) { throw 'the fuse wire runs past the end of SplitLane.exe' }
    $wire = $text.Substring($pos + 2, $length)
    $shown = -join @($wire.ToCharArray() | ForEach-Object { if ('0', '1', 'r' -ccontains [string]$_) { [string]$_ } else { '?' } })
    $states = @()
    foreach ($name in $fuses.Keys) {
        $index = $order.options.$name
        if ($index -isnot [int] -and $index -isnot [long]) { throw "@electron/fuses $($order.version) has no fuse $name" }
        if ($index -ge $length) { throw "the fuse wire has $length fuses; $name is fuse $index" }
        if ([string]$wire[[int]$index] -cne $fuses[$name]) {
            throw "fuse $name (index $index) is '$($shown[[int]$index])', want '$($fuses[$name])' (wire $shown)"
        }
        $states += "$name[$index]=$($fuses[$name])"
    }
    Say "fuses of SplitLane.exe: one sentinel, wire version 1, $length fuses, wire $shown; $($states -join ', ') (order of @electron/fuses $($order.version))"
}

function Assert-Installed($dataSddl, $order) {
    foreach ($f in 'SplitLane.exe', 'Uninstall SplitLane.exe') {
        if (-not (Test-Path -LiteralPath (Join-Path $product $f) -PathType Leaf)) { throw "no $f in $product" }
    }
    $stage1 = @($six | Where-Object { Test-Path -LiteralPath (Join-Path $product $_) })
    if ($stage1.Count -gt 0) { throw "the stage 1 files are still at the root of ${product}: $($stage1 -join ', ')" }
    $inHelper = @(Get-ChildItem -LiteralPath $helper -Force | ForEach-Object { $_.Name } | Sort-Object)
    $want = @($six | Sort-Object)
    if (($inHelper -join '|') -ne ($want -join '|')) { throw "helper\ has [$($inHelper -join ', ')], want [$($want -join ', ')]" }
    foreach ($f in $six) {
        $a = (Get-FileHash -LiteralPath (Join-Path $helper $f) -Algorithm SHA256).Hash
        $b = (Get-FileHash -LiteralPath (Join-Path $dist $f) -Algorithm SHA256).Hash
        if ($a -ne $b) { throw "helper\$f differs from dist\helper\$f" }
    }
    Say "$product has SplitLane.exe and Uninstall SplitLane.exe, none of the six stage 1 files; helper\ has the six files of dist\helper"
    $entries = @(Get-UninstallEntries)
    if ($entries.Count -ne 1) { throw "$($entries.Count) uninstall entries SplitLane, want one" }
    Say 'one uninstall entry SplitLane'
    Assert-Task
    $setupFile = Join-Path $data 'setup.json'
    if (-not (Test-Path -LiteralPath $setupFile -PathType Leaf)) { throw "no $setupFile" }
    $setup = Get-Content -LiteralPath $setupFile -Raw | ConvertFrom-Json
    $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    if ($setup.userSid -ne $sid) { throw "setup.json has userSid '$($setup.userSid)', want the SID of this user, $sid" }
    Say 'setup.json names this user'
    $why = Compare-Dacl (Get-Acl -LiteralPath $data).Sddl $dataSddl
    if ($why) { throw "the access list of ${data}: $why" }
    Say "the access list of $data is install.DataSDDL $dataSddl"
    $exe = Join-Path $product 'SplitLane.exe'
    Assert-Fuses $exe $order
    $description = (Get-Item -LiteralPath $exe).VersionInfo.FileDescription
    if ($description -cne 'SplitLane') { throw "the FileDescription of SplitLane.exe is '$description', want 'SplitLane'" }
    Say 'the FileDescription of SplitLane.exe is SplitLane'
}

function Assert-Markers {
    foreach ($dir in $data, $userData) {
        if (-not (Test-Path -LiteralPath (Join-Path $dir $marker) -PathType Leaf)) { throw "the marker file in $dir is gone" }
    }
    Say "the marker files in $data and $userData are there"
}

try {
    $refusal = Get-Refusal
} catch {
    $refusal = "the machine could not be checked: $($_.Exception.Message)"
}
if ($refusal) {
    Write-Host "TEST-REFUSED: $refusal"
    exit 2
}

try {
    if (-not $Installer) { throw '-Installer <path of SplitLane-Setup-<version>.exe> is required' }
    if (-not (Test-Path -LiteralPath $Installer -PathType Leaf)) { throw "no installer $Installer" }
    $Installer = (Resolve-Path -LiteralPath $Installer).Path
    foreach ($f in $six) {
        if (-not (Test-Path -LiteralPath (Join-Path $dist $f) -PathType Leaf)) { throw "no dist\helper\$f; run scripts\build-installer.ps1 first" }
    }
    $dataSddl = Get-DataSddl
    $order = Get-FuseOrder
    Write-Host "== $Installer; install.DataSDDL $dataSddl; @electron/fuses $($order.version)"

    $script:step = 'layout'
    Write-Host "== 1. the stage 1 layout: the six files of the helper at the root of $product"
    New-Item -ItemType Directory -Path $product | Out-Null
    foreach ($f in $six) { Copy-Item -LiteralPath (Join-Path $dist $f) -Destination $product }
    Say "$product has the six files"

    $script:step = 'install'
    Write-Host '== 2. silent install over the stage 1 layout'
    Install-Product
    Assert-Installed $dataSddl $order

    $script:step = 'reinstall'
    Write-Host '== 3. silent install over the installed product'
    Set-Content -LiteralPath (Join-Path $data $marker) -Value 'data folder'
    New-Item -ItemType Directory -Force -Path $userData | Out-Null
    Set-Content -LiteralPath (Join-Path $userData $marker) -Value 'settings folder'
    Install-Product
    Assert-Markers
    Assert-Installed $dataSddl $order

    $script:step = 'uninstall'
    Write-Host '== 4. silent uninstall without /PURGE'
    Uninstall-Product '/S'
    if (Get-Task) { throw "the task $taskName is still there" }
    Say "no task $taskName, no $product"
    if (-not (Test-Path -LiteralPath $data -PathType Container)) { throw "$data is gone" }
    Assert-Markers

    $script:step = 'purge'
    Write-Host '== 5. silent install, then silent uninstall with /PURGE'
    Install-Product
    Assert-Task
    Uninstall-Product '/S /PURGE'
    if (Get-Task) { throw "the task $taskName is still there" }
    foreach ($dir in $data, $userData) {
        if (Test-Path -LiteralPath $dir) { throw "$dir is still there" }
    }
    Say "no task $taskName, no $product, no $data, no $userData"
} catch {
    Write-Host "TEST-FAIL $($script:step): $(($_.Exception.Message -replace '\s*\r?\n\s*', ' ').Trim())"
    exit 1
}
Write-Host 'TEST-OK'
