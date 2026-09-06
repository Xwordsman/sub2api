$ErrorActionPreference = "Stop"

$PluginDir = $PSScriptRoot
if (-not $PluginDir) {
    $PluginDir = (Get-Location).Path
}

$UIFile = Join-Path $PluginDir "ui\index.html"
if (-not (Test-Path $UIFile)) {
    Write-Error "ui\index.html not found in $PluginDir"
    exit 1
}

$manifestPath = Join-Path $PluginDir "manifest.json"
if (-not (Test-Path $manifestPath)) {
    Write-Error "manifest.json not found in $PluginDir"
    exit 1
}

$hash = (Get-FileHash -Path $UIFile -Algorithm SHA256).Hash.ToLower()

$json = [System.IO.File]::ReadAllText($manifestPath, [System.Text.Encoding]::UTF8)
$pattern = '"ui/index\.html":\s*"[0-9a-fA-F]*"'
$replacement = '"ui/index.html": "' + $hash + '"'
$updatedJson = [System.Text.RegularExpressions.Regex]::Replace($json, $pattern, $replacement)

[System.IO.File]::WriteAllText($manifestPath, $updatedJson, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Updated manifest.json with ui/index.html hash: $hash"

$zipPath = Join-Path $PluginDir "account-platform-manager.s2plugin"
if (Test-Path $zipPath) {
    Remove-Item -Path $zipPath -Force
}

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

$zip = [System.IO.Compression.ZipFile]::Open($zipPath, [System.IO.Compression.ZipArchiveMode]::Create)
try {
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $manifestPath, "manifest.json")
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $UIFile, "ui/index.html")
} finally {
    $zip.Dispose()
}

Write-Host "Successfully packaged plugin to $zipPath"
