[CmdletBinding()]
param(
    [switch]$InstallDependencies
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$frontendRoot = Join-Path $projectRoot "frontend"
$localGo = Join-Path $projectRoot ".toolchains\go\bin\go.exe"
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { "go" }
$env:GOCACHE = Join-Path $projectRoot ".gocache"
$env:GOMODCACHE = Join-Path $projectRoot ".gomodcache"

function Require-Command {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [string]$InstallHint
    )

    if ($null -eq (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is required. $InstallHint"
    }
}

if (-not (Test-Path -LiteralPath $localGo)) {
    Require-Command -Name "go" -InstallHint "Install Go 1.26 or place it under .toolchains\go."
}
Require-Command -Name "node" -InstallHint "Install Node.js 22 or newer."
Require-Command -Name "pnpm" -InstallHint "Install pnpm 11.9.0."

$frontendPackage = Get-Content -LiteralPath (Join-Path $frontendRoot "package.json") -Raw | ConvertFrom-Json
$pnpmVersion = ((& pnpm --version) -join "").Trim()
if ($LASTEXITCODE -ne 0 -or $pnpmVersion -ne "11.9.0") {
    throw "pnpm 11.9.0 is required; found '$pnpmVersion'."
}

$viteCommand = Join-Path $frontendRoot "node_modules\.bin\vite.cmd"
if (-not (Test-Path -LiteralPath $viteCommand)) {
    if (-not $InstallDependencies) {
        throw "Frontend dependencies are missing. Re-run with -InstallDependencies or run 'pnpm --dir frontend install --frozen-lockfile'."
    }
    & pnpm --dir $frontendRoot install --frozen-lockfile
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend dependency installation failed with exit code $LASTEXITCODE."
    }
}

Push-Location $projectRoot
try {
    & $goCommand run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 dev
    if ($LASTEXITCODE -ne 0) {
        throw "Wails development session exited with code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
