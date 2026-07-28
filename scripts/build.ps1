[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot "..\build\bin")
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$localGo = Join-Path $projectRoot ".toolchains\go\bin\go.exe"
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { "go" }
Copy-Item -LiteralPath (Join-Path $projectRoot "frontend\public\logo.png") -Destination (Join-Path $projectRoot "build\appicon.png")
$windowsIcon = Join-Path $projectRoot "build\windows\icon.ico"
if (Test-Path -LiteralPath $windowsIcon) {
    Remove-Item -LiteralPath $windowsIcon
}

Push-Location (Join-Path $projectRoot "frontend")
try {
    pnpm run build
}
finally {
    Pop-Location
}

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
& $goCommand test ./...
& $goCommand build -trimpath -ldflags "-s -w" -o (Join-Path $OutputDirectory "csm.exe") ./cmd/csm
& $goCommand run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -s -m -trimpath -platform windows/amd64 -o CodexSkillManager.exe

Write-Host "Built binaries in $OutputDirectory"
