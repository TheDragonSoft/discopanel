# DiscoPanel Proxmox LXC Deployment Script
# Builds the frontend + cross-compiles the Linux/AMD64 binary and deploys it to the Proxmox LXC container.

[CmdletBinding()]
param(
    [string]$ProxmoxHost = "192.168.1.200",
    [string]$ProxmoxUser = "root",
    [string]$ProxmoxPass = "#1@14proxmox",
    [int]$ContainerId = 104,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = (Resolve-Path "$ScriptDir\..").Path
$BuildDir = "$ProjectRoot\build"
$BinaryPath = "$BuildDir\discopanel-linux-amd64"

Write-Host "===============================================" -ForegroundColor Cyan
Write-Host " DiscoPanel Proxmox LXC Deployment" -ForegroundColor Cyan
Write-Host " Target Host: $ProxmoxHost (LXC $ContainerId)" -ForegroundColor Cyan
Write-Host "===============================================" -ForegroundColor Cyan

# ---------------------------------------------------------
# Step 1: Build Frontend and Cross-Compile Go Binary
# ---------------------------------------------------------
if (-not $SkipBuild) {
    Write-Host "`n[1/5] Building Svelte 5 Frontend..." -ForegroundColor Yellow
    Push-Location "$ProjectRoot\web\discopanel"
    try {
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }

    Write-Host "`n[2/5] Cross-compiling Go backend for Linux AMD64..." -ForegroundColor Yellow
    Push-Location "$ProjectRoot"
    try {
        if (-not (Test-Path $BuildDir)) {
            New-Item -ItemType Directory -Path $BuildDir | Out-Null
        }
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        & go build -ldflags="-s -w" -o "$BinaryPath" ./cmd/discopanel
        if ($LASTEXITCODE -ne 0) {
            throw "Go compilation failed with exit code $LASTEXITCODE"
        }
        $binSize = (Get-Item $BinaryPath).Length / 1MB
        Write-Host ("Compiled binary size: {0:N2} MB" -f $binSize) -ForegroundColor Green
    } finally {
        Pop-Location
    }
} else {
    Write-Host "`n[1/5 & 2/5] Skipping build as requested (-SkipBuild)" -ForegroundColor DarkGray
    if (-not (Test-Path $BinaryPath)) {
        throw "Binary not found at $BinaryPath. Run without -SkipBuild."
    }
}

# ---------------------------------------------------------
# Step 2: Upload binary to Proxmox host
# ---------------------------------------------------------
Write-Host "`n[3/5] Uploading binary to Proxmox host ($ProxmoxHost)..." -ForegroundColor Yellow
Import-Module Posh-SSH -ErrorAction SilentlyContinue
$secPass = ConvertTo-SecureString $ProxmoxPass -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential($ProxmoxUser, $secPass)

Set-SCPItem -ComputerName $ProxmoxHost -Credential $cred -Path $BinaryPath -Destination "/tmp/" -AcceptKey -Force
Write-Host "Upload to Proxmox host /tmp/ successful." -ForegroundColor Green

# ---------------------------------------------------------
# Step 3: Push into LXC and Swap Binary
# ---------------------------------------------------------
Write-Host "`n[4/5] Pushing binary into LXC $ContainerId and swapping service..." -ForegroundColor Yellow
$session = New-SSHSession -ComputerName $ProxmoxHost -Credential $cred -AcceptKey -Force

try {
    # Push into container
    $pushCmd = "pct push $ContainerId /tmp/discopanel-linux-amd64 /opt/discopanel/discopanel-linux-amd64.new"
    Write-Host ">> $pushCmd" -ForegroundColor DarkGray
    $pushRes = Invoke-SSHCommand -SessionId $session.SessionId -Command $pushCmd
    if ($pushRes.ExitStatus -ne 0) {
        throw "Failed to push binary into LXC: $($pushRes.Error)"
    }

    # Execute swap inside container
    $swapCmd = @"
pct exec $ContainerId -- bash -c '
  set -e
  echo "--- Stopping discopanel service ---"
  systemctl stop discopanel

  echo "--- Backing up previous binary ---"
  if [ -f /opt/discopanel/discopanel-linux-amd64 ]; then
    cp /opt/discopanel/discopanel-linux-amd64 /opt/discopanel/discopanel-linux-amd64.bak
  fi

  echo "--- Installing new binary ---"
  mv /opt/discopanel/discopanel-linux-amd64.new /opt/discopanel/discopanel-linux-amd64
  chmod +x /opt/discopanel/discopanel-linux-amd64
  ln -sf /opt/discopanel/discopanel-linux-amd64 /opt/discopanel/discopanel

  echo "--- Starting discopanel service ---"
  systemctl start discopanel
  sleep 3

  echo "--- Service status ---"
  systemctl is-active discopanel
'
"@
    Write-Host "Executing swap inside LXC..." -ForegroundColor DarkGray
    $swapRes = Invoke-SSHCommand -SessionId $session.SessionId -Command $swapCmd -TimeOut 60
    if ($swapRes.Output) { Write-Host $swapRes.Output }
    if ($swapRes.Error) { Write-Host $swapRes.Error -ForegroundColor Yellow }
    if ($swapRes.ExitStatus -ne 0) {
        throw "Service swap failed with exit code $($swapRes.ExitStatus)"
    }

    # ---------------------------------------------------------
    # Step 4: Health Check & Verification
    # ---------------------------------------------------------
    Write-Host "`n[5/5] Performing health check..." -ForegroundColor Yellow
    $checkCmd = @"
pct exec $ContainerId -- bash -c '
  echo "--- Binary details ---"
  ls -lh /opt/discopanel/discopanel-linux-amd64

  echo "--- HTTP Port 8080 Check ---"
  curl -s -o /dev/null -w "HTTP Response Code: %{http_code}\n" http://localhost:8080 || echo "Curl failed"

  echo "--- Listening Sockets ---"
  ss -tlnp | grep 8080 || echo "Port 8080 not listening"

  echo "--- Recent Service Logs ---"
  journalctl -u discopanel --no-pager -n 12
'
"@
    $checkRes = Invoke-SSHCommand -SessionId $session.SessionId -Command $checkCmd -TimeOut 30
    if ($checkRes.Output) { Write-Host $checkRes.Output }
    if ($checkRes.Error) { Write-Host $checkRes.Error -ForegroundColor Yellow }

    # Step 5: Cleanup host temp file
    Invoke-SSHCommand -SessionId $session.SessionId -Command "rm -f /tmp/discopanel-linux-amd64" | Out-Null

} finally {
    Remove-SSHSession -SessionId $session.SessionId | Out-Null
}

Write-Host "`n===============================================" -ForegroundColor Green
Write-Host " Deployment Complete!" -ForegroundColor Green
Write-Host " DiscoPanel is running in LXC $ContainerId" -ForegroundColor Green
Write-Host " Access Web UI at: http://192.168.1.204:8080" -ForegroundColor Green
Write-Host "===============================================" -ForegroundColor Green
