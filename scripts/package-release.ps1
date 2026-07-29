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

$binaryRoot = Join-Path $projectRoot "build\bin"
foreach ($binary in @("CodexSkillManager.exe", "csm.exe")) {
    if (-not (Test-Path -LiteralPath (Join-Path $binaryRoot $binary))) {
        throw "Missing release binary: $binary"
    }
}

$stagingRoot = Join-Path $OutputRoot "staging"
$standardName = "CodexSkillManager-$Version"
$portableName = "CodexSkillManager-$Version-portable"
$standardRoot = Join-Path $stagingRoot $standardName
$portableRoot = Join-Path $stagingRoot $portableName
New-Item -ItemType Directory -Path $standardRoot, $portableRoot | Out-Null

$rootDocuments = @(
    "CHANGELOG.md",
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
}

Copy-ReleaseContents -Destination $standardRoot
Copy-ReleaseContents -Destination $portableRoot
Copy-Item -LiteralPath (Join-Path $projectRoot "packaging\portable.marker") -Destination $portableRoot
New-Item -ItemType Directory -Path (Join-Path $portableRoot "data") | Out-Null

$standardArchive = Join-Path $OutputRoot "$standardName-windows-amd64.zip"
$portableArchive = Join-Path $OutputRoot "$portableName-windows-amd64.zip"
Compress-Archive -LiteralPath $standardRoot -DestinationPath $standardArchive -CompressionLevel Optimal
Compress-Archive -LiteralPath $portableRoot -DestinationPath $portableArchive -CompressionLevel Optimal

$checksumPath = Join-Path $OutputRoot "CodexSkillManager-$Version-SHA256SUMS.txt"
$checksumLines = foreach ($archive in @($standardArchive, $portableArchive)) {
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($archive))"
}
Set-Content -LiteralPath $checksumPath -Value $checksumLines -Encoding utf8

[pscustomobject]@{
    Version = $Version
    OutputRoot = $OutputRoot
    Assets = @(
        [System.IO.Path]::GetFileName($standardArchive),
        [System.IO.Path]::GetFileName($portableArchive),
        [System.IO.Path]::GetFileName($checksumPath)
    )
} | ConvertTo-Json -Depth 3
