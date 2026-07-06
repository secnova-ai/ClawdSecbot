param(
    [Parameter(Mandatory = $true)]
    [string]$InstallerPath,

    [string]$OutputDirectory = (Join-Path -Path $env:TEMP -ChildPath 'clawdsecbot-web-package-verify')
)

$ErrorActionPreference = 'Stop'

if ($PSVersionTable.PSVersion.Major -lt 5) {
    throw "This script requires PowerShell 5.0 or higher. Current version: $($PSVersionTable.PSVersion)"
}

if (-not (Test-Path -LiteralPath $InstallerPath)) {
    throw "Installer not found: $InstallerPath"
}

$resolvedInstaller = (Resolve-Path -LiteralPath $InstallerPath).Path
$resolvedTemp = [System.IO.Path]::GetFullPath($env:TEMP)
$resolvedOutput = [System.IO.Path]::GetFullPath($OutputDirectory)

if (-not $resolvedOutput.StartsWith($resolvedTemp, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputDirectory must stay under the temp directory. OutputDirectory=$resolvedOutput Temp=$resolvedTemp"
}

if (Test-Path -LiteralPath $resolvedOutput) {
    Remove-Item -LiteralPath $resolvedOutput -Recurse -Force
}

New-Item -ItemType Directory -Path $resolvedOutput -Force | Out-Null

$extractDir = Join-Path -Path $resolvedOutput -ChildPath 'installed'
$payloadZip = Join-Path -Path $resolvedOutput -ChildPath 'payload-from-installer.zip'
New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

$installerBytes = [System.IO.File]::ReadAllBytes($resolvedInstaller)
$markerBytes = [System.Text.Encoding]::ASCII.GetBytes('BOTSEC_PAYLOAD_V1')
$metadataLength = $markerBytes.Length + 8

if ($installerBytes.Length -lt $metadataLength) {
    throw 'Installer is too small to contain payload metadata.'
}

$markerOffset = $installerBytes.Length - $metadataLength
for ($i = 0; $i -lt $markerBytes.Length; $i++) {
    if ($installerBytes[$markerOffset + $i] -ne $markerBytes[$i]) {
        throw 'Installer payload marker was not found at the expected tail position.'
    }
}

$payloadLength = [System.BitConverter]::ToInt64($installerBytes, $markerOffset + $markerBytes.Length)
$payloadStart = $installerBytes.Length - $metadataLength - $payloadLength
if ($payloadStart -lt 0) {
    throw 'Installer payload length is invalid.'
}

$payloadBytes = New-Object byte[] ([int]$payloadLength)
[System.Array]::Copy($installerBytes, [int]$payloadStart, $payloadBytes, 0, [int]$payloadLength)
[System.IO.File]::WriteAllBytes($payloadZip, $payloadBytes)

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory($payloadZip, $extractDir)

$requiredFiles = @(
    (Join-Path -Path $extractDir -ChildPath 'ClawdSecbotWebLauncher.exe'),
    (Join-Path -Path $extractDir -ChildPath 'bin\botsec_webd.exe'),
    (Join-Path -Path $extractDir -ChildPath 'web\main.dart.js'),
    (Join-Path -Path $extractDir -ChildPath 'README-Web.txt')
)

$checks = foreach ($path in $requiredFiles) {
    $exists = Test-Path -LiteralPath $path
    [PSCustomObject]@{
        Path = $path
        Exists = $exists
        Length = if ($exists) { (Get-Item -LiteralPath $path).Length } else { 0 }
    }
}

$missing = $checks | Where-Object { -not $_.Exists }
if ($missing.Count -gt 0) {
    $checks | ConvertTo-Json -Depth 4
    throw 'Extracted package is missing required files.'
}

[PSCustomObject]@{
    Installer = $resolvedInstaller
    ExtractDirectory = $extractDir
    PayloadLength = $payloadLength
    Checks = $checks
} | ConvertTo-Json -Depth 5
