param(
    [string]$InputDirectory = (Join-Path $PSScriptRoot "..\docs\images\ui-screens"),
    [string]$OutputPath = "",
    [ValidateSet("en-US", "zh-CN")]
    [string]$Locale = "en-US",
    [string]$ManifestPath = (Join-Path $PSScriptRoot "screenshot-frames.json")
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$manifest = Get-Content -Raw -Encoding UTF8 -LiteralPath $ManifestPath | ConvertFrom-Json
$screens = @($manifest.frames | ForEach-Object {
    @{
        File = $_.file
        Label = $_.label.PSObject.Properties[$Locale].Value
    }
})
$InputDirectory = Join-Path $InputDirectory $Locale
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $PSScriptRoot "..\docs\images\ui-gallery.$Locale.png"
}

$columns = 4
$rows = [Math]::Ceiling($screens.Count / $columns)
$tileWidth = 600
$imageHeight = 375
$labelHeight = 42
$gap = 18
$canvasWidth = ($columns * $tileWidth) + (($columns + 1) * $gap)
$canvasHeight = ($rows * ($imageHeight + $labelHeight)) + (($rows + 1) * $gap)

$canvas = New-Object System.Drawing.Bitmap($canvasWidth, $canvasHeight)
$graphics = [System.Drawing.Graphics]::FromImage($canvas)
$graphics.Clear([System.Drawing.Color]::FromArgb(243, 246, 250))
$graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
$graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
$graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
$graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality

$labelBackground = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(16, 24, 39))
$labelForeground = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::White)
$borderPen = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(208, 216, 228), 1)
$font = New-Object System.Drawing.Font("Segoe UI", 16, [System.Drawing.FontStyle]::Bold)
$format = New-Object System.Drawing.StringFormat
$format.Alignment = [System.Drawing.StringAlignment]::Near
$format.LineAlignment = [System.Drawing.StringAlignment]::Center

try {
    for ($index = 0; $index -lt $screens.Count; $index++) {
        $screen = $screens[$index]
        $sourcePath = Join-Path $InputDirectory $screen.File
        if (-not (Test-Path -LiteralPath $sourcePath)) {
            throw "Screenshot not found: $sourcePath"
        }

        $column = $index % $columns
        $row = [Math]::Floor($index / $columns)
        $x = $gap + ($column * ($tileWidth + $gap))
        $y = $gap + ($row * ($imageHeight + $labelHeight + $gap))

        $source = [System.Drawing.Image]::FromFile($sourcePath)
        try {
            $imageRect = New-Object System.Drawing.Rectangle($x, $y, $tileWidth, $imageHeight)
            $labelRect = New-Object System.Drawing.Rectangle($x, ($y + $imageHeight), $tileWidth, $labelHeight)
            $graphics.DrawImage($source, $imageRect)
            $graphics.FillRectangle($labelBackground, $labelRect)
            $graphics.DrawRectangle($borderPen, $x, $y, ($tileWidth - 1), ($imageHeight + $labelHeight - 1))

            $textRect = New-Object System.Drawing.RectangleF(
                ($x + 16),
                ($y + $imageHeight),
                ($tileWidth - 32),
                $labelHeight
            )
            $graphics.DrawString($screen.Label, $font, $labelForeground, $textRect, $format)
        }
        finally {
            $source.Dispose()
        }
    }

    $outputDirectory = Split-Path -Parent $OutputPath
    if (-not (Test-Path -LiteralPath $outputDirectory)) {
        New-Item -ItemType Directory -Path $outputDirectory | Out-Null
    }
    $canvas.Save($OutputPath, [System.Drawing.Imaging.ImageFormat]::Png)
}
finally {
    $format.Dispose()
    $font.Dispose()
    $borderPen.Dispose()
    $labelForeground.Dispose()
    $labelBackground.Dispose()
    $graphics.Dispose()
    $canvas.Dispose()
}

Write-Output "Screenshot gallery created: $OutputPath"
