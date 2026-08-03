[CmdletBinding()]
param(
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA "CodexSkillManager"),
    [switch]$Apply,
    [switch]$Launch
)

$ErrorActionPreference = "Stop"
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$binaryRoot = Join-Path $projectRoot "build\bin"
$installPath = [System.IO.Path]::GetFullPath($InstallDirectory)
$installParent = [System.IO.Path]::GetDirectoryName($installPath)
$transactionID = "deploy-" + [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssfffffffZ")
$stagePath = Join-Path $installParent (".CodexSkillManager-stage-" + $transactionID)
$backupRoot = Join-Path $installParent "CodexSkillManager-backups"
$backupPath = Join-Path $backupRoot $transactionID
$journalRoot = Join-Path $projectRoot "build\deployments"
$journalPath = Join-Path $journalRoot ($transactionID + ".json")
$manifestPath = Join-Path $binaryRoot "build-manifest.json"
$desktopSource = Join-Path $binaryRoot "CodexSkillManager.exe"
$cliSource = Join-Path $binaryRoot "csm.exe"

foreach ($requiredPath in @($manifestPath, $desktopSource, $cliSource)) {
    if (-not (Test-Path -LiteralPath $requiredPath -PathType Leaf)) {
        throw "Missing build output: $requiredPath. Run scripts\build.ps1 first."
    }
}

$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
foreach ($fileName in @("CodexSkillManager.exe", "csm.exe")) {
    $sourcePath = Join-Path $binaryRoot $fileName
    $hashProperty = $manifest.files.PSObject.Properties[$fileName]
    $expectedHash = if ($null -eq $hashProperty) { "" } else { [string]$hashProperty.Value }
    $actualHash = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($expectedHash) -or $expectedHash -ne $actualHash) {
        throw "Build manifest hash mismatch: $fileName"
    }
}

$running = Get-Process -Name "CodexSkillManager" -ErrorAction SilentlyContinue | Where-Object {
    try {
        [System.StringComparer]::OrdinalIgnoreCase.Equals($_.Path, (Join-Path $installPath "CodexSkillManager.exe"))
    }
    catch {
        $false
    }
}
if ($null -ne $running) {
    throw "CodexSkillManager is running from the target directory. Close it before deploying."
}

$plan = [ordered]@{
    schemaVersion = 1
    transactionId = $transactionID
    operation = "deploy-local"
    status = if ($Apply) { "planned" } else { "dry-run" }
    version = [string]$manifest.version
    source = $binaryRoot
    target = $installPath
    stage = $stagePath
    backup = if (Test-Path -LiteralPath $installPath) { $backupPath } else { $null }
    files = @(
        [ordered]@{ name = "CodexSkillManager.exe"; sha256 = [string]$manifest.files."CodexSkillManager.exe" },
        [ordered]@{ name = "csm.exe"; sha256 = [string]$manifest.files."csm.exe" },
        [ordered]@{ name = "BUILD-MANIFEST.json"; sha256 = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant() }
    )
    recovery = "If activation fails, move the backup directory back to the target path. Failed staged content is retained for inspection."
}

if (-not $Apply) {
    $plan | ConvertTo-Json -Depth 5
    return
}

New-Item -ItemType Directory -Force -Path $installParent | Out-Null
New-Item -ItemType Directory -Force -Path $backupRoot | Out-Null
New-Item -ItemType Directory -Force -Path $journalRoot | Out-Null
New-Item -ItemType Directory -Path $stagePath | Out-Null
Copy-Item -LiteralPath $desktopSource -Destination (Join-Path $stagePath "CodexSkillManager.exe")
Copy-Item -LiteralPath $cliSource -Destination (Join-Path $stagePath "csm.exe")
Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $stagePath "BUILD-MANIFEST.json")

$plan.status = "staged"
$plan | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $journalPath -Encoding utf8
$hadPreviousInstall = Test-Path -LiteralPath $installPath

try {
    if ($hadPreviousInstall) {
        Move-Item -LiteralPath $installPath -Destination $backupPath
    }
    Move-Item -LiteralPath $stagePath -Destination $installPath
    $plan.status = "completed"
    $plan.completedAt = [DateTime]::UtcNow.ToString("o")
    $plan | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $journalPath -Encoding utf8
}
catch {
    $activationError = $_
    $failedPath = Join-Path $installParent ("CodexSkillManager-failed-" + $transactionID)
    if (Test-Path -LiteralPath $stagePath) {
        Move-Item -LiteralPath $stagePath -Destination $failedPath
    }
    if ($hadPreviousInstall -and -not (Test-Path -LiteralPath $installPath) -and (Test-Path -LiteralPath $backupPath)) {
        Move-Item -LiteralPath $backupPath -Destination $installPath
    }
    $plan.status = "failed"
    $plan.error = $activationError.Exception.Message
    $plan.failedContent = if (Test-Path -LiteralPath $failedPath) { $failedPath } else { $null }
    $plan | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $journalPath -Encoding utf8
    throw
}

if ($Launch) {
    Start-Process -FilePath (Join-Path $installPath "CodexSkillManager.exe") -WindowStyle Hidden
}

$plan | ConvertTo-Json -Depth 5
