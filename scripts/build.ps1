[CmdletBinding()]
param(
    [string]$OutputDirectory
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$buildRoot = Join-Path $projectRoot "build"
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $buildRoot "bin"
}
$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
$wailsOutputDirectory = Join-Path $buildRoot "bin"
$wailsDesktopPath = Join-Path $wailsOutputDirectory "CodexSkillManager.exe"
$cliOutputPath = Join-Path $OutputDirectory "csm.exe"
$desktopOutputPath = Join-Path $OutputDirectory "CodexSkillManager.exe"
$manifestOutputPath = Join-Path $OutputDirectory "build-manifest.json"
$localGo = Join-Path $projectRoot ".toolchains\go\bin\go.exe"
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { "go" }
$env:GOCACHE = Join-Path $projectRoot ".gocache"
$env:GOMODCACHE = Join-Path $projectRoot ".gomodcache"
$projectInfo = Get-Content -LiteralPath (Join-Path $projectRoot "wails.json") -Raw | ConvertFrom-Json
$frontendPackage = Get-Content -LiteralPath (Join-Path $projectRoot "frontend\package.json") -Raw | ConvertFrom-Json
$expectedVersion = [string]$projectInfo.info.productVersion
if ([string]$frontendPackage.version -ne $expectedVersion) {
    throw "Frontend version mismatch: expected $expectedVersion, got $($frontendPackage.version)"
}
$changelog = Get-Content -LiteralPath (Join-Path $projectRoot "CHANGELOG.md") -Raw
$changelogVersions = [regex]::Matches($changelog, '(?m)^##\s+(\d+\.\d+\.\d+)\s*$')
if ($changelogVersions.Count -eq 0 -or $changelogVersions[0].Groups[1].Value -ne $expectedVersion) {
    throw "The first CHANGELOG version must be $expectedVersion"
}
$sourceCommit = (& git -C $projectRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($sourceCommit)) {
    throw "Unable to resolve the source commit"
}
$sourceStatus = @(& git -C $projectRoot status --porcelain --untracked-files=normal)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect the source worktree"
}
$sourceDirty = $sourceStatus.Count -gt 0

New-Item -ItemType Directory -Force -Path $buildRoot | Out-Null
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
New-Item -ItemType Directory -Force -Path $wailsOutputDirectory | Out-Null
Copy-Item -LiteralPath (Join-Path $projectRoot "frontend\public\logo.png") -Destination (Join-Path $buildRoot "appicon.png")
$windowsIcon = Join-Path $buildRoot "windows\icon.ico"
if (Test-Path -LiteralPath $windowsIcon) {
    Remove-Item -LiteralPath $windowsIcon
}
if (Test-Path -LiteralPath $wailsDesktopPath) {
    # Wails always writes beneath build/bin. Removing this one explicit file
    # prevents a failed build from being mistaken for fresh output.
    Remove-Item -LiteralPath $wailsDesktopPath
}
if (Test-Path -LiteralPath $cliOutputPath) {
    Remove-Item -LiteralPath $cliOutputPath
}
if (Test-Path -LiteralPath $manifestOutputPath) {
    Remove-Item -LiteralPath $manifestOutputPath
}
if (
    -not [System.StringComparer]::OrdinalIgnoreCase.Equals($wailsDesktopPath, $desktopOutputPath) -and
    (Test-Path -LiteralPath $desktopOutputPath)
) {
    Remove-Item -LiteralPath $desktopOutputPath
}

Push-Location (Join-Path $projectRoot "frontend")
try {
    $frontendBin = Join-Path (Get-Location) "node_modules\.bin"
    $vitestCommand = Join-Path $frontendBin "vitest.cmd"
    $tscCommand = Join-Path $frontendBin "tsc.cmd"
    $viteCommand = Join-Path $frontendBin "vite.cmd"
    if (-not (Test-Path -LiteralPath $vitestCommand) -or
        -not (Test-Path -LiteralPath $tscCommand) -or
        -not (Test-Path -LiteralPath $viteCommand)) {
        throw "Frontend dependencies are missing. Run 'pnpm --dir frontend install --frozen-lockfile' first."
    }

    & $vitestCommand run
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend tests failed with exit code $LASTEXITCODE"
    }
    & $tscCommand
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend type-check failed with exit code $LASTEXITCODE"
    }
    & $viteCommand build
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Push-Location $projectRoot
try {
    & $goCommand mod verify
    if ($LASTEXITCODE -ne 0) {
        throw "Go module verification failed with exit code $LASTEXITCODE"
    }
    & $goCommand test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Go tests failed with exit code $LASTEXITCODE"
    }
    & $goCommand vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Go vet failed with exit code $LASTEXITCODE"
    }
    & $goCommand build -trimpath -ldflags "-s -w" -o $cliOutputPath ./cmd/csm
    if ($LASTEXITCODE -ne 0) {
        throw "CLI build failed with exit code $LASTEXITCODE"
    }
    & $goCommand run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 build -s -m -trimpath -platform windows/amd64 -o CodexSkillManager.exe
    if ($LASTEXITCODE -ne 0) {
        throw "Wails build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $wailsDesktopPath)) {
    throw "Wails reported success but did not create $wailsDesktopPath"
}
if (-not [System.StringComparer]::OrdinalIgnoreCase.Equals($wailsDesktopPath, $desktopOutputPath)) {
    Copy-Item -LiteralPath $wailsDesktopPath -Destination $desktopOutputPath -Force
}

$cliPath = $cliOutputPath
$desktopPath = $desktopOutputPath
$manifestPath = $manifestOutputPath
$reportedVersion = ((& $cliPath version) -join [Environment]::NewLine).Trim()
if ($reportedVersion -ne $expectedVersion) {
    throw "CLI version mismatch: expected $expectedVersion, got $reportedVersion"
}
$expectedFileVersion = [System.Version]::Parse("$expectedVersion.0")
$desktopFileVersion = (Get-Item -LiteralPath $desktopPath).VersionInfo.FileVersionRaw
if ($null -eq $desktopFileVersion -or $desktopFileVersion -ne $expectedFileVersion) {
    throw "Desktop file version mismatch: expected $expectedFileVersion, got $desktopFileVersion"
}
$manifest = [ordered]@{
    schemaVersion = 1
    commit = $sourceCommit
    dirty = $sourceDirty
    version = $reportedVersion
    builtAt = [DateTime]::UtcNow.ToString("o")
    toolchain = [ordered]@{
        go = ((& $goCommand version) -join " ").Trim()
        node = ((& node --version) -join " ").Trim()
        packageManager = [string]$frontendPackage.packageManager
        wails = "v2.13.0"
        powershell = $PSVersionTable.PSVersion.ToString()
        windows = [Environment]::OSVersion.VersionString
    }
    files = [ordered]@{
        "csm.exe" = (Get-FileHash -LiteralPath $cliPath -Algorithm SHA256).Hash.ToLowerInvariant()
        "CodexSkillManager.exe" = (Get-FileHash -LiteralPath $desktopPath -Algorithm SHA256).Hash.ToLowerInvariant()
    }
}
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding utf8

Write-Host "Built binaries in $OutputDirectory"
