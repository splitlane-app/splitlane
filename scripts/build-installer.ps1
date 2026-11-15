# SplitLane: build the installer app\release\SplitLane-Setup-<version>.exe (NSIS through electron-builder,
# spec 3 and 6 of stage 2) and next to it SplitLane-Setup-<version>.exe.sha256, one line
# "<sha256>  SplitLane-Setup-<version>.exe". The steps: scripts\build-helper.ps1 (dist\helper, the six
# files that the installer puts into helper\), then in app: the packages, the Electron binary, typecheck,
# tests, the build of the window and electron-builder --win nsis (app\build\installer.nsh adds the
# helper's prepare, install and uninstall). The installer is not signed. Stops on the first failure.
# The version is "version" of app\package.json. GitHub Actions runs the same script.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1
# Result: last line BUILD-OK <full path of the installer> or BUILD-FAILED <reason>.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$app = Join-Path $root 'app'

function Invoke-Step([string]$label, [scriptblock]$cmd) {
    Write-Host "== $label"
    & $cmd
    if ($LASTEXITCODE -ne 0) { throw "$label failed with exit code $LASTEXITCODE" }
}

try {
    $version = (Get-Content -LiteralPath (Join-Path $app 'package.json') -Raw | ConvertFrom-Json).version
    if ($version -isnot [string] -or $version -notmatch '^\d+\.\d+\.\d+$') {
        throw "version in app\package.json is not x.y.z: $version"
    }
    $name = "SplitLane-Setup-$version.exe"
    $installer = Join-Path $app "release\$name"
    $sum = "$installer.sha256"
    foreach ($old in $installer, $sum) {
        if (Test-Path -LiteralPath $old) { Remove-Item -LiteralPath $old -Force }
    }
    Invoke-Step 'build-helper' { powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'build-helper.ps1') }
    Push-Location $app
    try {
        Invoke-Step 'pnpm install' { pnpm install --frozen-lockfile }
        # electron 45 has no postinstall: its binary is downloaded on the first require('electron')
        Invoke-Step 'electron binary' { node -e "require('electron')" }
        Invoke-Step 'pnpm typecheck' { pnpm typecheck }
        Invoke-Step 'pnpm test' { pnpm test }
        Invoke-Step 'pnpm build' { pnpm build }
        Invoke-Step 'pnpm run dist' { pnpm run dist }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path -LiteralPath $installer)) { throw "electron-builder made no $installer" }
    $hash = (Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLower()
    [System.IO.File]::WriteAllText($sum, "$hash  $name`n", (New-Object System.Text.UTF8Encoding $false))
    Write-Host "== SHA-256 $hash"
    Write-Host "BUILD-OK $installer"
} catch {
    Write-Host "BUILD-FAILED $($_.Exception.Message)"
    exit 1
}
