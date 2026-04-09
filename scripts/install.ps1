param(
  [string]$InstallDir = "$env:USERPROFILE\bin"
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$BinName = "jira.exe"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("jira-install-" + [System.Guid]::NewGuid().ToString("N"))

Write-Host "Building jira from $RepoRoot..."
New-Item -ItemType Directory -Path $TempDir | Out-Null
try {
  Push-Location $RepoRoot
  go build -o (Join-Path $TempDir $BinName) .\cmd\jira
  Pop-Location

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Copy-Item -Path (Join-Path $TempDir $BinName) -Destination (Join-Path $InstallDir $BinName) -Force

  Write-Host "Installed to: $(Join-Path $InstallDir $BinName)"
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
