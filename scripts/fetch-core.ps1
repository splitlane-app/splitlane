# SplitLane: get sing-box 1.14.4 windows-amd64 and check it against the SHA-256 pinned in core.sha256.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -ExecutionPolicy Bypass -File scripts\fetch-core.ps1 [-Zip <local copy of the archive>]
# Result: .cache\core\sing-box-1.14.4-windows-amd64\sing-box.exe; last line FETCH-OK <path> or FETCH-FAILED <reason>.
param([string]$Zip = '')
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$version = '1.14.4'
$name = "sing-box-$version-windows-amd64"
$url = "https://github.com/SagerNet/sing-box/releases/download/v$version/$name.zip"
$root = Split-Path -Parent $PSScriptRoot
$dir = Join-Path $root '.cache\core'

function Get-Pin([string]$file) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $PSScriptRoot 'core.sha256')) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $file) { return $parts[0].ToLower() }
    }
    throw "no pin for $file in core.sha256"
}

function Get-Sha256([string]$path) { (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLower() }

try {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $archive = Join-Path $dir "$name.zip"
    if ($Zip) {
        Copy-Item -LiteralPath $Zip -Destination $archive -Force
    } elseif (-not (Test-Path -LiteralPath $archive)) {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $url -OutFile "$archive.part" -UseBasicParsing
        Move-Item -LiteralPath "$archive.part" -Destination $archive -Force
    }
    $want = Get-Pin "$name.zip"
    $got = Get-Sha256 $archive
    if ($got -ne $want) {
        Remove-Item -LiteralPath $archive -Force
        Write-Host "FETCH-FAILED $name.zip has SHA-256 $got, the pin is $want; the archive was deleted"
        exit 1
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $dir -Force
    $exe = Join-Path $dir "$name\sing-box.exe"
    $wantExe = Get-Pin "$name/sing-box.exe"
    $gotExe = Get-Sha256 $exe
    if ($gotExe -ne $wantExe) {
        Write-Host "FETCH-FAILED sing-box.exe has SHA-256 $gotExe, the pin is $wantExe"
        exit 1
    }
    Write-Host "FETCH-OK $exe"
} catch {
    Write-Host "FETCH-FAILED $($_.Exception.Message)"
    exit 1
}
