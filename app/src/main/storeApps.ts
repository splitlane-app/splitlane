import { packageValue } from '../shared/protocol'
import type { Program } from '../shared/programs'

/**
 * One PowerShell call: Start-menu entries of Store apps, with the executable
 * taken from the package manifest. ASCII only. Apps whose executable cannot
 * be found are left out.
 */
export const STORE_APPS_SCRIPT = [
  "$ErrorActionPreference = 'SilentlyContinue'",
  '$packages = @{}',
  'foreach ($p in Get-AppxPackage -PackageTypeFilter Main) { $packages[$p.PackageFamilyName] = $p }',
  '$list = @()',
  'foreach ($app in Get-StartApps) {',
  '  try {',
  "    $parts = ([string]$app.AppID).Split('!')",
  '    if ($parts.Length -ne 2) { continue }',
  '    $pkg = $packages[$parts[0]]',
  '    if ($null -eq $pkg) { continue }',
  "    $manifest = Join-Path $pkg.InstallLocation 'AppxManifest.xml'",
  '    if (-not (Test-Path -LiteralPath $manifest)) { continue }',
  '    [xml]$xml = Get-Content -LiteralPath $manifest -Raw',
  '    $node = $xml.Package.Applications.Application | Where-Object { $_.Id -eq $parts[1] } | Select-Object -First 1',
  '    if ($null -eq $node -or [string]::IsNullOrEmpty($node.Executable)) { continue }',
  '    $exe = Join-Path $pkg.InstallLocation $node.Executable',
  '    if (Test-Path -LiteralPath $exe) { $list += [pscustomobject]@{ name = [string]$app.Name; exe = $exe } }',
  '  } catch { }',
  '}',
  'ConvertTo-Json -Compress -InputObject @($list)',
].join('\r\n')

export function parseStoreApps(json: string): Program[] {
  if (json.trim() === '') return []
  let data: unknown
  try {
    data = JSON.parse(json)
  } catch {
    return []
  }
  const items = Array.isArray(data) ? data : [data]
  const seen = new Set<string>()
  const out: Program[] = []
  for (const item of items) {
    if (typeof item !== 'object' || item === null) continue
    const { name, exe } = item as { name?: unknown; exe?: unknown }
    if (typeof name !== 'string' || name === '' || typeof exe !== 'string' || exe === '') continue
    if (packageValue(exe) === null) continue
    const key = exe.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ name, exe })
  }
  return out
}

/** Runs the script once per window run; a failure gives an empty list (cached too). */
export class StoreAppsCache {
  private pending: Promise<Program[]> | null = null

  constructor(private readonly load: () => Promise<string>) {}

  get(): Promise<Program[]> {
    this.pending ??= this.load().then(parseStoreApps, () => [])
    return this.pending
  }
}
