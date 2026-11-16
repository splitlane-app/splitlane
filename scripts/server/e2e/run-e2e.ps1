# SplitLane: the end-to-end check of "Your own VPS" (spec of stage 3 part 2, section 9) on Docker Desktop.
# Builds a test server image (Dockerfile next to this file: systemd, sshd, root and the sudo user kit) for
# each base image; then, one system at a time, starts fresh containers and runs the Go test
# helper\internal\serversetup\e2e_test.go
# (serversetup.Run over real SSH: root with a password, kit with sudo, kit with a key, busy 443, a second
# run, a server with the kit installed before; fail2ban counts a wrong password) and one smoke run of the
# built splitlane-helper.exe server-setup over stdin and stdout, then removes the containers, the network
# and the test images. Only for a local run; never during network checks of the PC.
#
# Safety: inside the containers api.ipify.org answers with this PC's exit address, so the kit takes that
# address for its links and its subscription name. Every container therefore resolves the ACME hosts of
# Let's Encrypt to 127.0.0.1 (no certificate is ever requested), the Go test fakes the subscription
# download, and only the smoke run tries the subscription address for real (90 s, then sub_unreachable).
# The passwords are made up at each run, the key of kit is generated in a temporary folder.
#
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts\server\e2e\run-e2e.ps1
#          [-Images debian:12,ubuntu:24.04] [-SmokeImage debian:12] [-NoSmoke] [-Parallel 5] [-MinFreeMB 2048] [-Keep]
# Before each system and before the smoke run it checks the free memory of the PC and stops there when
# less than -MinFreeMB is left (the systems before it keep their results).
# Needs: Docker Desktop (Linux containers), Go, ssh-keygen (the OpenSSH client of Windows).
# Result: last line E2E-OK or E2E-FAILED <what>; exit code 0 or 1.
param(
    [string[]]$Images = @('debian:12', 'ubuntu:22.04', 'ubuntu:24.04', 'debian:13', 'ubuntu:26.04'),
    [string]$SmokeImage = 'debian:12',
    [switch]$NoSmoke,
    [int]$Parallel = 5,
    [int]$BasePort = 22100,
    [int]$MinFreeMB = 2048,
    [switch]$Keep
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# -File passes "-Images a,b" as one string.
$Images = @($Images | ForEach-Object { $_ -split ',' } | ForEach-Object { $_.Trim() } | Where-Object { $_ })
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
$label = 'splitlane-e2e=1'
$network = 'splitlane-e2e'
$acmeHosts = @('acme-v02.api.letsencrypt.org', 'acme-staging-v02.api.letsencrypt.org')

function Get-Tag([string]$image) { 'splitlane-e2e:' + ($image -replace '[:/]', '-') }

function Get-Short([string]$image) { $image -replace '[:/]', '-' }

# The free physical memory of the PC in MB.
function Get-FreeMB { [int]((Get-CimInstance Win32_OperatingSystem).FreePhysicalMemory / 1024) }

# Throws when less than -MinFreeMB is free, so that the run stops before $what.
function Assert-Memory([string]$what) {
    $free = Get-FreeMB
    Write-Host "== free memory: $free MB (at least $MinFreeMB MB needed for $what)"
    if ($free -lt $MinFreeMB) { throw "stopped before ${what}: $free MB of memory free, less than $MinFreeMB MB" }
}

# Letters and digits from a cryptographic generator.
function New-Secret([int]$length) {
    $chars = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    $bytes = New-Object byte[] $length
    $rng = New-Object System.Security.Cryptography.RNGCryptoServiceProvider
    $rng.GetBytes($bytes)
    $rng.Dispose()
    -join ($bytes | ForEach-Object { $chars[$_ % $chars.Length] })
}

# Removes the containers and the network of this check (and, with -Images, its images).
function Remove-E2E([switch]$Images) {
    $ids = @(& docker ps -aq --filter "label=$label")
    if ($ids.Count -gt 0) { & docker rm -f $ids | Out-Null }
    $nets = @(& docker network ls -q --filter "label=$label")
    if ($nets.Count -gt 0) { & docker network rm $nets | Out-Null }
    if ($Images) {
        $tags = @(& docker images --format '{{.Repository}}:{{.Tag}}' splitlane-e2e)
        if ($tags.Count -gt 0) { & docker rmi $tags | Out-Null }
    }
}

function Start-Server([string]$name, [string]$image, [int]$port, [string]$rootPw, [string]$kitPw, [string]$kitKey) {
    $a = @('run', '-d', '--name', $name, '--label', $label, '--network', $network,
        '--privileged', '--cgroupns=host', '-p', "127.0.0.1:${port}:22")
    foreach ($h in $acmeHosts) { $a += @('--add-host', "${h}:127.0.0.1") }
    $a += @('-e', "E2E_ROOT_PASSWORD=$rootPw", '-e', "E2E_KIT_PASSWORD=$kitPw", '-e', "E2E_KIT_KEY=$kitKey", (Get-Tag $image))
    & docker @a | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "docker run of $name failed" }
}

# Waits until systemd of the container has started, and makes sure it cannot reach Let's Encrypt.
function Wait-Server([string]$name) {
    $state = ''
    for ($i = 0; $i -lt 120; $i++) {
        $state = (& docker exec $name sh -c 'systemctl is-system-running 2>/dev/null') -join ''
        if ($state -eq 'running' -or $state -eq 'degraded') { break }
        Start-Sleep -Seconds 1
    }
    if ($state -ne 'running' -and $state -ne 'degraded') { throw "systemd of $name did not start (state: $state)" }
    foreach ($h in $acmeHosts) {
        $addrs = @(& docker exec $name getent ahosts $h | ForEach-Object { ($_ -split '\s+')[0] } | Sort-Object -Unique)
        if ($addrs.Count -ne 1 -or $addrs[0] -ne '127.0.0.1') { throw "$h resolves to $($addrs -join ', ') in $name" }
    }
    $state
}

# The links as "scheme:port ...", nothing secret.
function Get-LinkText([string]$links) {
    $parts = foreach ($l in ($links -split "`n")) {
        if ($l -match '^([a-z0-9+.-]+)://') {
            $scheme = $Matches[1]
            $rest = ($l.Substring($scheme.Length + 3) -split '[?#]', 2)[0]
            $rest = $rest.Substring($rest.LastIndexOf('@') + 1).TrimEnd('/')
            "${scheme}:" + $rest.Substring($rest.LastIndexOf(':') + 1)
        } elseif ($l) { '?' }
    }
    $parts -join ' '
}

# The smoke run: splitlane-helper.exe server-setup with the request on stdin, kept open until the end
# (closing it cancels). Prints the steps and a summary without secrets; returns an error text or ''.
function Invoke-Smoke([string]$exe, [int]$port, [string]$password, [bool]$sub) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $exe
    $psi.Arguments = 'server-setup'
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $utf8 = New-Object System.Text.UTF8Encoding $false
    $psi.StandardOutputEncoding = $utf8
    $psi.StandardErrorEncoding = $utf8
    # The request must start with '{': no byte order mark from the console's input encoding.
    $savedInput = $null
    if ($psi.PSObject.Properties['StandardInputEncoding']) {
        $psi.StandardInputEncoding = $utf8
    } else {
        try {
            if ([Console]::InputEncoding.GetPreamble().Length -gt 0) {
                $savedInput = [Console]::InputEncoding
                [Console]::InputEncoding = $utf8
            }
        } catch {
            Write-Host "  could not check the console's input encoding: $_"
        }
    }
    $start = Get-Date
    try {
        $proc = [System.Diagnostics.Process]::Start($psi)
    } finally {
        if ($savedInput) { [Console]::InputEncoding = $savedInput }
    }
    $stderr = $proc.StandardError.ReadToEndAsync()
    $request = @{ host = '127.0.0.1'; port = $port; user = 'root'; password = $password } | ConvertTo-Json -Compress
    $bytes = $utf8.GetBytes($request + "`n")
    $proc.StandardInput.BaseStream.Write($bytes, 0, $bytes.Length)
    $proc.StandardInput.BaseStream.Flush()
    [Array]::Clear($bytes, 0, $bytes.Length)
    $final = $null
    $leak = $false
    while ($null -ne ($line = $proc.StandardOutput.ReadLine())) {
        if ($line.Contains($password)) { $leak = $true }
        $o = $line | ConvertFrom-Json
        if ($o.PSObject.Properties['step']) {
            Write-Host ('  {0} {1}: {2}' -f $o.step, $o.state, $o.detail)
        } else {
            $final = $o
        }
    }
    $proc.WaitForExit()
    $proc.StandardInput.Close()
    $took = [int]((Get-Date) - $start).TotalSeconds
    $code = $proc.ExitCode
    if ($stderr.Result) { Write-Host "  stderr: $($stderr.Result.Length) characters (want none)" }
    if ($leak) { return 'the password is in the output of server-setup' }
    if (-not $final) { return "no final line (exit code $code)" }
    if ($final.PSObject.Properties['failed']) { return "failed $($final.failed.code) $($final.failed.detail) (exit code $code)" }
    $done = $final.done
    $links = Get-LinkText $done.links
    $warnings = @($done.warnings)
    $subText = 'null'
    if ($null -ne $done.subscription) { $subText = 'an address' }
    Write-Host "  done in $took s, exit code ${code}: links $links; subscription $subText; warnings $($warnings -join ', ')"
    $want = 'sub_unreachable'
    if (-not $sub) { $want = 'sub_skipped' }
    if ($code -ne 0) { return "exit code $code" }
    if ($links -notmatch '\bvless:\d+' -or $links -notmatch '\bhysteria2:\d+') { return "links $links" }
    if ($null -ne $done.subscription -or $warnings -notcontains $want) { return "subscription $subText, warnings $($warnings -join ', '), want null and $want" }
    if ($stderr.Result) { return 'server-setup wrote to stderr' }
    ''
}

$failed = @()
$tmp = Join-Path $env:TEMP ('splitlane-e2e-' + (New-Secret 8))
try {
    foreach ($tool in 'docker', 'go', 'ssh-keygen') {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "$tool is not in PATH" }
    }
    $server = (& docker version --format '{{.Server.Os}}') -join ''
    if ($LASTEXITCODE -ne 0 -or $server -ne 'linux') { throw 'Docker is not running with Linux containers' }
    Remove-E2E -Images
    New-Item -ItemType Directory -Path $tmp | Out-Null

    # Made-up passwords with a space, a quote and a dollar: none of them may pass through a shell.
    $rootPw = 'Root e2e ' + (New-Secret 12) + ' $&''!'
    $kitPw = 'Kit e2e ' + (New-Secret 12) + ' $&''!'
    $keyFile = Join-Path $tmp 'kit_ed25519'
    $kg = Start-Process -FilePath (Get-Command ssh-keygen).Source -NoNewWindow -Wait -PassThru `
        -ArgumentList "-q -t ed25519 -N `"`" -C splitlane-e2e -f `"$keyFile`""
    if ($kg.ExitCode -ne 0) { throw 'ssh-keygen failed' }
    $kitKey = (Get-Content -LiteralPath "$keyFile.pub" -Raw).Trim()

    $build = @($Images)
    if (-not $NoSmoke -and $build -notcontains $SmokeImage) { $build += $SmokeImage }
    foreach ($image in $build) {
        Write-Host "== image $(Get-Tag $image) from $image"
        & docker build -q --label $label --build-arg "BASE=$image" -t (Get-Tag $image) $PSScriptRoot | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "docker build from $image failed" }
    }
    & docker network create --label $label $network | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "docker network create $network failed" }

    $roles = @(@{ role = 'root'; user = 'root' }, @{ role = 'sudo'; user = 'kit' }, @{ role = 'busy'; user = 'root' }, @{ role = 'preinstalled'; user = 'root' })
    $clientPort = $BasePort + 90
    $client = @{ name = 'client'; scenario = 'client'; host = '127.0.0.1'; port = $clientPort; user = 'root'; password = $rootPw }
    Start-Server 'splitlane-e2e-client' $Images[0] $clientPort $rootPw $kitPw $kitKey
    $state = Wait-Server 'splitlane-e2e-client'
    Write-Host "== splitlane-e2e-client ($($Images[0])) on 127.0.0.1:${clientPort}: systemd $state, ACME hosts at 127.0.0.1"

    # One system at a time, so that Docker never holds more than a few servers: root (and key) on one
    # fresh server, sudo, busy and preinstalled on one each. Their containers go when the test is over.
    $i = 0
    foreach ($image in $Images) {
        Assert-Memory $image
        $short = Get-Short $image
        $targets = New-Object System.Collections.ArrayList
        $started = @()
        for ($j = 0; $j -lt $roles.Count; $j++) {
            $r = $roles[$j]
            $p = [int]($BasePort + 10 * $i + $j)
            $pw = $rootPw
            if ($r.user -eq 'kit') { $pw = $kitPw }
            $name = "splitlane-e2e-$short-$($r.role)"
            Start-Server $name $image $p $rootPw $kitPw $kitKey
            $started += @{ name = $name; port = $p }
            [void]$targets.Add(@{ name = "$short/$($r.role)"; scenario = $r.role; host = '127.0.0.1'; port = $p; user = $r.user; password = $pw })
            if ($r.role -eq 'root') {
                [void]$targets.Add(@{ name = "$short/key"; scenario = 'key'; host = '127.0.0.1'; port = $p; user = 'kit'; keyfile = $keyFile })
            }
        }
        [void]$targets.Add($client)
        foreach ($s in $started) {
            $state = Wait-Server $s.name
            Write-Host "== $($s.name) ($image) on 127.0.0.1:$($s.port): systemd $state, ACME hosts at 127.0.0.1"
        }

        Write-Host "== go test -tags e2e: $image"
        $env:SPLITLANE_E2E = ConvertTo-Json -InputObject $targets -Compress -Depth 3
        Push-Location (Join-Path $repo 'helper')
        try {
            & go test -tags e2e -run '^TestE2E$' -count=1 -v -timeout 45m -parallel $Parallel ./internal/serversetup/
            if ($LASTEXITCODE -ne 0) { $failed += "go test on $image" }
        } finally {
            Pop-Location
            Remove-Item Env:SPLITLANE_E2E
        }
        if (-not $Keep) {
            & docker rm -f ($started | ForEach-Object { $_.name }) | Out-Null
        }
        $i++
    }

    if (-not $NoSmoke) {
        Assert-Memory 'the smoke run'
        $smokePort = $BasePort + 91
        Start-Server 'splitlane-e2e-smoke' $SmokeImage $smokePort $rootPw $kitPw $kitKey
        $state = Wait-Server 'splitlane-e2e-smoke'
        Write-Host "== smoke run: splitlane-helper.exe server-setup, root on splitlane-e2e-smoke ($SmokeImage, systemd $state)"
        $version = (Get-Content -LiteralPath (Join-Path $repo 'app\package.json') -Raw | ConvertFrom-Json).version
        $exe = Join-Path $tmp 'splitlane-helper.exe'
        Push-Location (Join-Path $repo 'helper')
        try {
            & go build -ldflags "-H=windowsgui -X main.version=$version" -o $exe ./cmd/splitlane-helper
            if ($LASTEXITCODE -ne 0) { throw 'go build of splitlane-helper.exe failed' }
        } finally {
            Pop-Location
        }
        $smoke = Invoke-Smoke $exe $smokePort $rootPw ($SmokeImage -ne 'ubuntu:22.04')
        if ($smoke) {
            Write-Host "  SMOKE FAILED: $smoke"
            $failed += 'smoke'
        } else {
            Write-Host '  SMOKE OK'
        }
    }
} catch {
    $failed += "$_"
} finally {
    if ($Keep) {
        Write-Host "== kept: the containers (label $label), the network $network, the images splitlane-e2e:*, $tmp"
        Write-Host '   root and kit passwords are in docker inspect; remove all with: docker rm -f $(docker ps -aq --filter label=splitlane-e2e=1)'
    } else {
        Remove-E2E -Images
        if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Recurse -Force }
        Write-Host '== removed the containers, the network and the images of the check'
    }
}
if ($failed.Count -gt 0) {
    Write-Host "E2E-FAILED $($failed -join '; ')"
    exit 1
}
Write-Host 'E2E-OK'
exit 0
