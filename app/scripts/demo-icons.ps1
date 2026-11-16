# Extracts the icons of the demo programs from this PC for the screenshots of the window (the demo mode, app\src\renderer\demo).
# Writes app\src\renderer\demo\icons.local.json {"<file name of the exe, lower case>": "data:image/png;base64,..."}, a git-ignored
# file that is never committed or shipped. A program that is not installed is skipped: the demo then shows a neutral monogram.
# It only reads the registry, the folders of the launchers and the exe files; it prints no path.
#   powershell -NoProfile -ExecutionPolicy Bypass -File app\scripts\demo-icons.ps1
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type -Namespace Demo -Name Native -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
public static extern uint PrivateExtractIcons(string file, int index, int cx, int cy, System.IntPtr[] icons, uint[] ids, uint count, uint flags);
[System.Runtime.InteropServices.DllImport("user32.dll")]
public static extern bool DestroyIcon(System.IntPtr icon);
'@

$out = Join-Path $PSScriptRoot '..\src\renderer\demo\icons.local.json'
$pf86 = ${env:ProgramFiles(x86)}
$pf = $env:ProgramFiles

function First-Existing([string[]]$paths) {
  foreach ($p in $paths) { if ($p -and (Test-Path -LiteralPath $p -PathType Leaf)) { return $p } }
  return $null
}

function App-Path([string]$exe) {
  foreach ($hive in 'HKLM:', 'HKCU:') {
    $key = "$hive\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$exe"
    $v = (Get-ItemProperty -LiteralPath $key -ErrorAction SilentlyContinue).'(default)'
    if ($v) { return $v.Trim('"') }
  }
  return $null
}

# the folders of the Steam libraries, from libraryfolders.vdf
function Steam-Libraries {
  $steam = (Get-ItemProperty 'HKCU:\Software\Valve\Steam' -ErrorAction SilentlyContinue).SteamPath
  if (-not $steam) { $steam = "$pf86\Steam" }
  $dirs = @($steam)
  $vdf = Join-Path $steam 'steamapps\libraryfolders.vdf'
  if (Test-Path -LiteralPath $vdf) {
    foreach ($m in [regex]::Matches((Get-Content -LiteralPath $vdf -Raw), '"path"\s+"([^"]+)"')) {
      $dirs += $m.Groups[1].Value.Replace('\\', '\')
    }
  }
  return $dirs
}

# the install folders of the Epic games, from the launcher's manifests
function Epic-Installs {
  $dir = Join-Path $env:ProgramData 'Epic\EpicGamesLauncher\Data\Manifests'
  if (-not (Test-Path -LiteralPath $dir)) { return @() }
  Get-ChildItem -LiteralPath $dir -Filter *.item | ForEach-Object {
    try { Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json } catch { }
  }
}

function Find-Steam { First-Existing @((Get-ItemProperty 'HKCU:\Software\Valve\Steam' -ErrorAction SilentlyContinue).SteamExe, "$pf86\Steam\steam.exe", "$pf\Steam\steam.exe") }
function Find-Epic {
  First-Existing @("$pf86\Epic Games\Launcher\Portal\Binaries\Win64\EpicGamesLauncher.exe", "$pf86\Epic Games\Launcher\Portal\Binaries\Win32\EpicGamesLauncher.exe", "$pf\Epic Games\Launcher\Portal\Binaries\Win64\EpicGamesLauncher.exe")
}
function Find-Discord {
  $root = Join-Path $env:LOCALAPPDATA 'Discord'
  if (-not (Test-Path -LiteralPath $root)) { return $null }
  $app = Get-ChildItem -LiteralPath $root -Directory -Filter 'app-*' | Sort-Object Name -Descending | Select-Object -First 1
  if ($app) { return First-Existing @((Join-Path $app.FullName 'Discord.exe')) }
  return $null
}
function Find-Firefox { First-Existing @((App-Path 'firefox.exe'), "$pf\Mozilla Firefox\firefox.exe", "$pf86\Mozilla Firefox\firefox.exe") }
function Find-Edge { First-Existing @((App-Path 'msedge.exe'), "$pf86\Microsoft\Edge\Application\msedge.exe", "$pf\Microsoft\Edge\Application\msedge.exe") }
function Find-Cs2 {
  foreach ($lib in Steam-Libraries) {
    $p = First-Existing @((Join-Path $lib 'steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe'))
    if ($p) { return $p }
  }
  return $null
}
function Find-Fortnite {
  foreach ($m in Epic-Installs) {
    if ($m.AppName -eq 'Fortnite' -or $m.DisplayName -eq 'Fortnite') {
      $p = First-Existing @((Join-Path $m.InstallLocation 'FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe'))
      if ($p) { return $p }
    }
  }
  return First-Existing @("$pf\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe")
}

# 64x64 icon of an exe as a PNG data URL; the large icon of the file, else the one Windows shows
function Icon-Url([string]$path) {
  $icon = $null
  $handles = New-Object 'System.IntPtr[]' 1
  $ids = New-Object 'uint32[]' 1
  if ([Demo.Native]::PrivateExtractIcons($path, 0, 64, 64, $handles, $ids, 1, 0) -ge 1 -and $handles[0] -ne [IntPtr]::Zero) {
    $icon = [System.Drawing.Icon]::FromHandle($handles[0]).Clone()
    [Demo.Native]::DestroyIcon($handles[0]) | Out-Null
  } else {
    $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($path)
  }
  $bmp = New-Object System.Drawing.Bitmap 64, 64
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.InterpolationMode = 'HighQualityBicubic'
  $g.DrawImage($icon.ToBitmap(), 0, 0, 64, 64)
  $g.Dispose()
  $ms = New-Object System.IO.MemoryStream
  $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
  $bmp.Dispose()
  return 'data:image/png;base64,' + [Convert]::ToBase64String($ms.ToArray())
}

$programs = [ordered]@{
  'Steam'               = { Find-Steam }
  'Epic Games Launcher' = { Find-Epic }
  'Discord'             = { Find-Discord }
  'Firefox'             = { Find-Firefox }
  'Microsoft Edge'      = { Find-Edge }
  'Counter-Strike 2'    = { Find-Cs2 }
  'Fortnite'            = { Find-Fortnite }
}

$icons = [ordered]@{}
foreach ($name in $programs.Keys) {
  $exe = & $programs[$name]
  if ($exe) {
    $icons[(Split-Path -Leaf $exe).ToLowerInvariant()] = Icon-Url $exe
    Write-Host ("{0,-20} found" -f $name)
  } else {
    Write-Host ("{0,-20} not installed: monogram" -f $name)
  }
}
[System.IO.File]::WriteAllText([System.IO.Path]::GetFullPath($out), ($icons | ConvertTo-Json -Compress), (New-Object System.Text.UTF8Encoding($false)))
Write-Host ("{0} icon(s) written" -f $icons.Count)
