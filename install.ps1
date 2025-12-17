<#
.SYNOPSIS
    llm-mux Windows Installer
    Installs llm-mux binary and sets up a background task.

.DESCRIPTION
    1. Downloads the latest release from GitHub.
    2. Installs to $env:LOCALAPPDATA\Programs\llm-mux.
    3. Adds the install directory to the User PATH.
    4. Initializes default configuration.
    5. Creates a Scheduled Task to run llm-mux at logon.

.EXAMPLE
    irm https://raw.githubusercontent.com/nghyane/llm-mux/main/install.ps1 | iex
#>

$ErrorActionPreference = "Stop"
$Repo = "nghyane/llm-mux"
$AppName = "llm-mux"
$InstallDir = "$env:LOCALAPPDATA\Programs\$AppName"
$BinPath = "$InstallDir\$AppName.exe"

# --- 1. Detect Architecture ---
Write-Host "Checking system architecture..." -ForegroundColor Cyan
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $Arch = "arm64"
} elseif ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
    $Arch = "amd64"
} else {
    Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
}

# --- 2. Get Latest Version ---
Write-Host "Fetching latest release info from GitHub..." -ForegroundColor Cyan
try {
    $LatestRelease = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $TagName = $LatestRelease.tag_name
    $Version = $TagName.TrimStart('v')
} catch {
    Write-Error "Failed to fetch latest release. Check your internet connection."
}

Write-Host "Latest version: $TagName" -ForegroundColor Green

# --- 3. Download and Install ---
$ZipName = "llm-mux_${Version}_windows_${Arch}.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$TagName/$ZipName"
$TempZip = "$env:TEMP\$ZipName"

Write-Host "Downloading $DownloadUrl..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempZip

if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

Write-Host "Extracting to $InstallDir..." -ForegroundColor Cyan
Expand-Archive -Path $TempZip -DestinationPath $InstallDir -Force
Remove-Item $TempZip -Force

# Verify binary existence (sometimes it's nested or flattened)
$ExeFound = Get-ChildItem -Path $InstallDir -Filter "$AppName.exe" -Recurse | Select-Object -First 1
if (!$ExeFound) {
    Write-Error "Could not find $AppName.exe in the downloaded archive."
}

# Move to root of install dir if nested
if ($ExeFound.DirectoryName -ne $InstallDir) {
    Move-Item $ExeFound.FullName $InstallDir -Force
}

# --- 4. Update PATH ---
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host "Adding $InstallDir to User PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    $env:Path += ";$InstallDir"
} else {
    Write-Host "PATH already configured." -ForegroundColor Green
}

# --- 5. Init Config ---
Write-Host "Initializing configuration..." -ForegroundColor Cyan
& $BinPath --init | Out-Null

# --- 6. Setup Persistence (Scheduled Task) ---
$TaskName = "Start llm-mux"
Write-Host "Setting up background task ($TaskName)..." -ForegroundColor Cyan

# Remove existing task if present to ensure update
if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

$Action = New-ScheduledTaskAction -Execute $BinPath
$Trigger = New-ScheduledTaskTrigger -AtLogon
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit 0

# Register the task
Register-ScheduledTask -Action $Action -Trigger $Trigger -Settings $Settings -TaskName $TaskName -Description "Runs llm-mux proxy server in the background" | Out-Null

Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host " $AppName $TagName installed successfully!" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Login to a provider:"
Write-Host "     llm-mux --login              # Gemini"
Write-Host "     llm-mux --claude-login       # Claude"
Write-Host ""
Write-Host "  2. The background service has been registered to start on login."
Write-Host "     To start it immediately, run:"
Write-Host "     Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Yellow
Write-Host ""
Write-Host "  3. Restart your terminal to refresh the PATH."
Write-Host ""
