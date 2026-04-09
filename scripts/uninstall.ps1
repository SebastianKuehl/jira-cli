param(
  [string]$InstallDir = "$env:USERPROFILE\bin"
)

$ErrorActionPreference = "Stop"

$BinName = "jira.exe"
$TargetPath = Join-Path $InstallDir $BinName

Write-Host "[1/3] Starting jira uninstall for Windows"
Write-Host "[2/3] Target binary: $TargetPath"
if (Test-Path $TargetPath) {
  Remove-Item -Path $TargetPath -Force
  Write-Host "[3/3] Removed: $TargetPath"
} else {
  Write-Host "[3/3] Nothing to remove (file not found)."
}
