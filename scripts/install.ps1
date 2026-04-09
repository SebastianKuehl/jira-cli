param(
  [string]$InstallDir = "$env:USERPROFILE\bin"
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$BinName = "jira.exe"
$TargetPath = Join-Path $InstallDir $BinName
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("jira-install-" + [System.Guid]::NewGuid().ToString("N"))

Write-Host "[1/6] Starting jira installation for Windows"
Write-Host "[2/6] Repository root: $RepoRoot"
Write-Host "[3/6] Install directory: $InstallDir"
New-Item -ItemType Directory -Path $TempDir | Out-Null
try {
  Write-Host "[4/6] Building binary..."
  Push-Location $RepoRoot
  go build -o (Join-Path $TempDir $BinName) .\cmd\jira
  Pop-Location

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Write-Host "[5/6] Installing binary to $TargetPath"
  Copy-Item -Path (Join-Path $TempDir $BinName) -Destination $TargetPath -Force

  Write-Host "[6/6] Installation complete: $TargetPath"
  Write-Host "Run 'jira --help' to verify."
  if (-not ($env:PATH -split ';' | Where-Object { $_ -eq $InstallDir })) {
    Write-Host "Note: $InstallDir is not on PATH."
    Write-Host "Add it in Windows Environment Variables to run 'jira' globally."
  }
}
finally {
  if (Test-Path $TempDir) {
    Remove-Item -Path $TempDir -Recurse -Force
  }
}
