[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version,

    [string]$OutputRoot
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if ([string]::IsNullOrWhiteSpace($OutputRoot)) {
    $OutputRoot = Join-Path $projectRoot "build\release\$Version"
}
$OutputRoot = [System.IO.Path]::GetFullPath($OutputRoot)

if (Test-Path -LiteralPath $OutputRoot) {
    throw "Release output already exists: $OutputRoot"
}

$worktreeStatus = @(& git -C $projectRoot status --porcelain --untracked-files=normal)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect the Git worktree"
}
if ($worktreeStatus.Count -gt 0) {
    throw "Release packaging requires a clean Git worktree. Commit the intended release state before packaging."
}

$binaryRoot = Join-Path $projectRoot "build\bin"
foreach ($binary in @("CodexSkillManager.exe", "csm.exe")) {
    if (-not (Test-Path -LiteralPath (Join-Path $binaryRoot $binary))) {
        throw "Missing release binary: $binary"
    }
}

$cliPath = Join-Path $binaryRoot "csm.exe"
$reportedVersionOutput = @(& $cliPath version)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to read the release CLI version"
}
$reportedVersion = ($reportedVersionOutput -join [Environment]::NewLine).Trim()
if ($reportedVersion -ne $Version) {
    throw "Release CLI version mismatch: expected $Version, got $reportedVersion"
}

$desktopPath = Join-Path $binaryRoot "CodexSkillManager.exe"
$expectedFileVersion = [System.Version]::Parse("$Version.0")
$desktopFileVersion = (Get-Item -LiteralPath $desktopPath).VersionInfo.FileVersionRaw
if ($null -eq $desktopFileVersion -or $desktopFileVersion -ne $expectedFileVersion) {
    throw "Desktop file version mismatch: expected $expectedFileVersion, got $desktopFileVersion"
}

$manifestPath = Join-Path $binaryRoot "build-manifest.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
    throw "Missing build provenance manifest: $manifestPath"
}
$releaseNoticePath = Join-Path $projectRoot "packaging\RELEASE-NOTICE.txt"
if (-not (Test-Path -LiteralPath $releaseNoticePath)) {
    throw "Missing release notice: $releaseNoticePath"
}
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$headCommit = (& git -C $projectRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($headCommit)) {
    throw "Unable to resolve the release commit"
}
if ([int]$manifest.schemaVersion -ne 1 -or [string]$manifest.commit -ne $headCommit) {
    throw "Build provenance does not match the release commit"
}
if ([bool]$manifest.dirty) {
    throw "Release binaries were built from a dirty worktree"
}
if ([string]$manifest.version -ne $Version) {
    throw "Build provenance version mismatch: expected $Version, got $($manifest.version)"
}
foreach ($binary in @("CodexSkillManager.exe", "csm.exe")) {
    $hashProperty = $manifest.files.PSObject.Properties[$binary]
    $expectedHash = if ($null -eq $hashProperty) { "" } else { [string]$hashProperty.Value }
    $actualHash = (Get-FileHash -LiteralPath (Join-Path $binaryRoot $binary) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($expectedHash) -or $expectedHash -ne $actualHash) {
        throw "Build provenance hash mismatch: $binary"
    }
}

$stagingRoot = Join-Path $projectRoot (
    "build\package-staging\{0}-{1}" -f $Version, ([guid]::NewGuid().ToString("N"))
)
$outputParent = [System.IO.Path]::GetDirectoryName($OutputRoot)
$temporaryOutputRoot = Join-Path $outputParent (
    ".csm-release-{0}-{1}" -f $Version, ([guid]::NewGuid().ToString("N"))
)
$standardName = "CodexSkillManager-$Version"
$portableName = "CodexSkillManager-$Version-portable"
$standardRoot = Join-Path $stagingRoot $standardName
$portableRoot = Join-Path $stagingRoot $portableName
New-Item -ItemType Directory -Force -Path $outputParent | Out-Null
New-Item -ItemType Directory -Path $temporaryOutputRoot | Out-Null
New-Item -ItemType Directory -Path $standardRoot, $portableRoot | Out-Null

$rootDocuments = @(
    "AGENTS.md",
    "CODE_OF_CONDUCT.md",
    "CHANGELOG.md",
    "CONTRIBUTING.md",
    "LICENSE",
    "README.md",
    "README_EN.md",
    "SECURITY.md",
    "SECURITY_EN.md",
    "SUPPORT.md"
)

function Copy-ReleaseContents {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Destination
    )

    Copy-Item -LiteralPath (Join-Path $binaryRoot "CodexSkillManager.exe") -Destination $Destination
    Copy-Item -LiteralPath (Join-Path $binaryRoot "csm.exe") -Destination $Destination
    Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $Destination "BUILD-MANIFEST.json")
    Copy-Item -LiteralPath $releaseNoticePath -Destination (Join-Path $Destination "RELEASE-NOTICE.txt")

    foreach ($document in $rootDocuments) {
        Copy-Item -LiteralPath (Join-Path $projectRoot $document) -Destination $Destination
    }

    $agentSkillRoot = Join-Path $Destination "agent-skill"
    New-Item -ItemType Directory -Path $agentSkillRoot | Out-Null
    $skillSource = Join-Path $projectRoot "skills\codex-skill-manager"
    Copy-Item -LiteralPath $skillSource -Destination (Join-Path $agentSkillRoot "codex-skill-manager") -Recurse

    $docsRoot = Join-Path $Destination "docs"
    New-Item -ItemType Directory -Path (Join-Path $docsRoot "images") -Force | Out-Null
    $trackedDocs = & git -C $projectRoot ls-files "docs/*.md" "docs/**/*.md"
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to enumerate tracked documentation"
    }
    foreach ($relativePath in $trackedDocs) {
        $sourcePath = Join-Path $projectRoot $relativePath
        $destinationPath = Join-Path $Destination $relativePath
        New-Item -ItemType Directory -Path ([System.IO.Path]::GetDirectoryName($destinationPath)) -Force | Out-Null
        Copy-Item -LiteralPath $sourcePath -Destination $destinationPath
    }

    $trackedImages = & git -C $projectRoot ls-files "docs/images/*"
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to enumerate tracked documentation images"
    }
    foreach ($relativePath in $trackedImages) {
        $sourcePath = Join-Path $projectRoot $relativePath
        $destinationPath = Join-Path $Destination $relativePath
        New-Item -ItemType Directory -Path ([System.IO.Path]::GetDirectoryName($destinationPath)) -Force | Out-Null
        Copy-Item -LiteralPath $sourcePath -Destination $destinationPath
    }
}

Copy-ReleaseContents -Destination $standardRoot
Copy-ReleaseContents -Destination $portableRoot
Copy-Item -LiteralPath (Join-Path $projectRoot "packaging\portable.marker") -Destination $portableRoot
New-Item -ItemType Directory -Path (Join-Path $portableRoot "data") | Out-Null

$standardArchive = Join-Path $temporaryOutputRoot "$standardName-windows-amd64.zip"
$portableArchive = Join-Path $temporaryOutputRoot "$portableName-windows-amd64.zip"
Compress-Archive -LiteralPath $standardRoot -DestinationPath $standardArchive -CompressionLevel Optimal
Compress-Archive -LiteralPath $portableRoot -DestinationPath $portableArchive -CompressionLevel Optimal

$releaseManifest = Join-Path $temporaryOutputRoot "CodexSkillManager-$Version-BUILD-MANIFEST.json"
Copy-Item -LiteralPath $manifestPath -Destination $releaseManifest
$releaseNotice = Join-Path $temporaryOutputRoot "CodexSkillManager-$Version-RELEASE-NOTICE.txt"
Copy-Item -LiteralPath $releaseNoticePath -Destination $releaseNotice
$checksumPath = Join-Path $temporaryOutputRoot "CodexSkillManager-$Version-SHA256SUMS.txt"
$checksumLines = foreach ($releaseFile in @(
    $standardArchive,
    $portableArchive,
    $releaseManifest,
    $releaseNotice
)) {
    $hash = (Get-FileHash -LiteralPath $releaseFile -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($releaseFile))"
}
Set-Content -LiteralPath $checksumPath -Value $checksumLines -Encoding ascii

# Publish only after every asset has been generated and hashed. If packaging
# fails, the final version path remains absent and a later retry is not blocked.
Move-Item -LiteralPath $temporaryOutputRoot -Destination $OutputRoot
$standardArchive = Join-Path $OutputRoot ([System.IO.Path]::GetFileName($standardArchive))
$portableArchive = Join-Path $OutputRoot ([System.IO.Path]::GetFileName($portableArchive))
$releaseManifest = Join-Path $OutputRoot ([System.IO.Path]::GetFileName($releaseManifest))
$releaseNotice = Join-Path $OutputRoot ([System.IO.Path]::GetFileName($releaseNotice))
$checksumPath = Join-Path $OutputRoot ([System.IO.Path]::GetFileName($checksumPath))

[pscustomobject]@{
    Version = $Version
    OutputRoot = $OutputRoot
    Assets = @(
        [System.IO.Path]::GetFileName($standardArchive),
        [System.IO.Path]::GetFileName($portableArchive),
        [System.IO.Path]::GetFileName($releaseManifest),
        [System.IO.Path]::GetFileName($releaseNotice),
        [System.IO.Path]::GetFileName($checksumPath)
    )
} | ConvertTo-Json -Depth 3
