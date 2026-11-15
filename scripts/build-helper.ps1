# SplitLane: build dist\helper\, the six files of the helper. The installer (NSIS, built with the window)
# puts them into %ProgramFiles%\SplitLane\helper\ next to the window at the root (spec 2, 4.2, 4.3, 14):
# splitlane-helper.exe without a console window, the unchanged sing-box.exe and libcronet.dll of the
# core, the core's licence as sing-box-LICENSE, LICENSE and THIRD-PARTY.md of SplitLane (it also carries
# the licence text of libcronet.dll, BSD-3-Clause of Chromium, that its binary form needs). The window runs
# dist\helper\splitlane-helper.exe install --user-sid <SID> with UAC. The test tools slctl and slguard are
# not part of it. The SHA-256 of sing-box.exe and libcronet.dll are checked against scripts\core.sha256
# before they are copied.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-helper.ps1 [-Version <x.y.z>]
# The version (x.y.z) is -Version, else "version" of app\package.json; the helper gets it as main.version.
# Needs: Go in %LOCALAPPDATA%\Programs\go\bin (else go from PATH), the core from scripts\fetch-core.ps1.
# Result: last line BUILD-OK dist\helper <version> or BUILD-FAILED <reason>.
param([string]$Version = '')
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$name = 'sing-box-1.14.4-windows-amd64'
$root = Split-Path -Parent $PSScriptRoot
$core = Join-Path $root ".cache\core\$name"
$out = Join-Path $root 'dist\helper'

function Get-Pin([string]$file) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $PSScriptRoot 'core.sha256')) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $file) { return $parts[0].ToLower() }
    }
    throw "no pin for $file in core.sha256"
}

function Get-Sha256([string]$path) { (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLower() }

try {
    $go = Join-Path $env:LOCALAPPDATA 'Programs\go\bin\go.exe'
    if (-not (Test-Path -LiteralPath $go)) {
        $found = Get-Command go -ErrorAction SilentlyContinue
        if (-not $found) {
            Write-Host "BUILD-FAILED no Go toolchain at $go or in PATH"
            exit 1
        }
        $go = $found.Source
    }
    $from = '-Version'
    if (-not $Version) {
        $Version = (Get-Content -LiteralPath (Join-Path $root 'app\package.json') -Raw | ConvertFrom-Json).version
        $from = 'version in app\package.json'
    }
    if ($Version -notmatch '^\d+\.\d+\.\d+$') {
        Write-Host "BUILD-FAILED $from is not x.y.z: $Version"
        exit 1
    }
    foreach ($file in 'sing-box.exe', 'libcronet.dll') {
        $path = Join-Path $core $file
        if (-not (Test-Path -LiteralPath $path)) {
            Write-Host "BUILD-FAILED no $path; run scripts\fetch-core.ps1 first"
            exit 1
        }
        $want = Get-Pin "$name/$file"
        $got = Get-Sha256 $path
        if ($got -ne $want) {
            Write-Host "BUILD-FAILED $file has SHA-256 $got, the pin is $want"
            exit 1
        }
    }
    if (Test-Path -LiteralPath $out) { Remove-Item -LiteralPath $out -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $out | Out-Null
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    Push-Location (Join-Path $root 'helper')
    try {
        $ErrorActionPreference = 'Continue'
        & $go build -ldflags "-H=windowsgui -X main.version=$Version" -o (Join-Path $out 'splitlane-helper.exe') ./cmd/splitlane-helper
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = 'Stop'
        Pop-Location
    }
    if ($code -ne 0) {
        Write-Host "BUILD-FAILED go build exit code $code"
        exit 1
    }
    Copy-Item -LiteralPath (Join-Path $core 'sing-box.exe') -Destination $out
    Copy-Item -LiteralPath (Join-Path $core 'libcronet.dll') -Destination $out
    Copy-Item -LiteralPath (Join-Path $core 'LICENSE') -Destination (Join-Path $out 'sing-box-LICENSE')
    Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination $out
    Copy-Item -LiteralPath (Join-Path $root 'THIRD-PARTY.md') -Destination $out
    Write-Host "BUILD-OK dist\helper $Version"
} catch {
    Write-Host "BUILD-FAILED $($_.Exception.Message)"
    exit 1
}
