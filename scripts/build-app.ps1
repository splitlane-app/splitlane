# SplitLane: build the unpacked, unsigned Windows build of the window into app\release\win-unpacked, with
# the six files of dist\helper in helper\ next to SplitLane.exe, as the installer lays them out (spec 2 of
# stage 2). For development; scripts\build-installer.ps1 builds the installer. Stops on the first failure.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-app.ps1
# Result: last line BUILD-OK app\release\win-unpacked or BUILD-FAILED <reason>.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$app = Join-Path $root 'app'

function Invoke-Step([string]$label, [scriptblock]$cmd) {
    Write-Host "== $label"
    & $cmd
    if ($LASTEXITCODE -ne 0) { throw "$label failed with exit code $LASTEXITCODE" }
}

try {
    Invoke-Step 'build-helper' { powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'build-helper.ps1') }
    Push-Location $app
    try {
        Invoke-Step 'pnpm install' { pnpm install --frozen-lockfile }
        # electron 45 has no postinstall: its binary is downloaded on the first require('electron')
        Invoke-Step 'electron binary' { node -e "require('electron')" }
        Invoke-Step 'pnpm typecheck' { pnpm typecheck }
        Invoke-Step 'pnpm test' { pnpm test }
        Invoke-Step 'pnpm build' { pnpm build }
        Invoke-Step 'pnpm run pack' { pnpm run pack }
    } finally {
        Pop-Location
    }
    $dir = Join-Path $app 'release\win-unpacked\helper'
    foreach ($f in 'splitlane-helper.exe', 'sing-box.exe', 'libcronet.dll', 'sing-box-LICENSE', 'LICENSE', 'THIRD-PARTY.md') {
        if (-not (Test-Path -LiteralPath (Join-Path $dir $f))) { throw "missing $f in $dir" }
    }
    Write-Host 'BUILD-OK app\release\win-unpacked'
} catch {
    Write-Host "BUILD-FAILED $($_.Exception.Message)"
    exit 1
}
