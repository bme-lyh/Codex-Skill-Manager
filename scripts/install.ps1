[CmdletBinding()]
param(
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA "CodexSkillManager"),
    [string]$DataDirectory = (Join-Path $env:USERPROFILE ".codex\skill-manager"),
    [string]$SkillsDirectory = (Join-Path $env:USERPROFILE ".codex\skills")
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$sourceBuildRoot = Join-Path $projectRoot "build\bin"
$binaryRoot = if (Test-Path -LiteralPath (Join-Path $sourceBuildRoot "csm.exe")) {
    $sourceBuildRoot
}
elseif (Test-Path -LiteralPath (Join-Path $PSScriptRoot "csm.exe")) {
    $PSScriptRoot
}
else {
    throw "Cannot find csm.exe and CodexSkillManager.exe."
}

New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
New-Item -ItemType Directory -Force -Path $DataDirectory | Out-Null
Copy-Item -LiteralPath (Join-Path $binaryRoot "csm.exe") -Destination (Join-Path $InstallDirectory "csm.exe")
Copy-Item -LiteralPath (Join-Path $binaryRoot "CodexSkillManager.exe") -Destination (Join-Path $InstallDirectory "CodexSkillManager.exe")

$configPath = Join-Path $DataDirectory "config.yaml"
if (-not (Test-Path -LiteralPath $configPath)) {
    $config = @"
schemaVersion: 1
paths:
  skillsRoot: '$SkillsDirectory'
  dataRoot: '$DataDirectory'
  logsRoot: '$(Join-Path $DataDirectory "logs")'
  reportsRoot: '$(Join-Path $DataDirectory "reports")'
  backupsRoot: '$(Join-Path $DataDirectory "backups")'
  quarantineRoot: '$(Join-Path $DataDirectory "quarantine")'
  cacheRoot: '$(Join-Path $DataDirectory "cache")'
  stagingRoot: '$(Join-Path $DataDirectory "staging")'
schedule:
  enabled: false
  frequency: weekly
  time: "09:00"
locale: zh-CN
githubHost: github.com
maxFileBytes: 20971520
maxFiles: 2000
"@
    Set-Content -LiteralPath $configPath -Value $config -Encoding UTF8
}

Write-Host "Installed to $InstallDirectory"
Write-Host "Configuration: $configPath"
