#Requires -Version 5.1
<#
.SYNOPSIS
    Build Windows release for ClawdSecbot.
.DESCRIPTION
    Builds the Go shared library (DLL), Flutter Windows app, and packages the output
    as a custom Windows installer EXE.
.EXAMPLE
    .\scripts\build_windows_release.ps1 --help
    .\scripts\build_windows_release.ps1 --version 1.3.0 --build 202603230900 --type community
#>

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$GoLibDir = Join-Path $ProjectRoot "go_lib"
$PluginsDir = Join-Path $ProjectRoot "plugins"
$OutputName = "botsec"
$DllName = "${OutputName}.dll"
$Version = "1.0.0"
$BuildNumber = (Get-Date -Format 'yyyyMMddHHmm')
$Arch = "x86_64"
$Type = "community"
$Brand = ""
$ForcePubGet = $false
$PackageFormat = "exe"

function Normalize-PackageType([string]$RawType) {
    $value = if ($null -eq $RawType) { '' } else { $RawType.ToLowerInvariant() }
    switch ($value) {
        'personal' { return 'community' }
        'community' { return 'community' }
        'business' { return 'business' }
        'appstore' { Stop-WithError "Windows release package does not support type=appstore" }
        default { Stop-WithError "Unsupported type: $RawType" }
    }
}

function Normalize-Arch([string]$RawArch) {
    $value = if ($null -eq $RawArch) { '' } else { $RawArch.ToLowerInvariant() }
    switch ($value) {
        'x86_64' { return 'x86_64' }
        'amd64' { return 'x86_64' }
        default { Stop-WithError "Windows release supports only arch=x86_64" }
    }
}

function Normalize-Brand([string]$RawBrand) {
    $value = if ($null -eq $RawBrand) { '' } else { $RawBrand.ToLowerInvariant() }
    $normalized = ($value -replace '[^a-z0-9]+', '-').Trim('-')
    if ([string]::IsNullOrWhiteSpace($normalized)) {
        Stop-WithError "Brand must contain letters or digits"
    }
    return $normalized
}

function Get-ArtifactTypeSegment {
    return $script:Type
}

function Get-ArtifactBrandSegment {
    if ($script:Type -eq 'business' -and -not [string]::IsNullOrWhiteSpace($script:Brand)) {
        return "-$($script:Brand)"
    }
    return ""
}

function Get-ArtifactFileName([string]$Extension) {
    return "ClawdSecbot-$Version-$BuildNumber-$Arch-$(Get-ArtifactTypeSegment)$(Get-ArtifactBrandSegment).$Extension"
}

function Write-Step([string]$msg) {
    Write-Host "[BUILD] $msg" -ForegroundColor Cyan
}

function Write-Ok([string]$msg) {
    Write-Host "[OK]    $msg" -ForegroundColor Green
}

function Write-Warn([string]$msg) {
    Write-Host "[WARN]  $msg" -ForegroundColor Yellow
}

function Stop-WithError([string]$msg) {
    Write-Host "[ERROR] $msg" -ForegroundColor Red
    exit 1
}

function Show-Help {
    @'
Usage:
  .\scripts\build_windows_release.ps1 [options]

Description:
  Build and package the Windows release as either a custom installer EXE or a ZIP archive.

Options:
  -v,  --version <X.Y.Z>     Semantic version (default: 1.0.0)
  -bn, --build <STAMP>       Build timestamp (default: current time, e.g. 202603230900)
       --build-number <STAMP>
  -ar, --arch <ARCH>         Target arch: x86_64
  -t,  --type <TYPE>         Package type: community|business (default: community)
  -br, --brand <NAME>        Brand suffix, only allowed when type=business
       --exe                 Build branded installer EXE (default)
       --zip                 Build ZIP package instead of installer EXE
       --force-pub-get       Force flutter pub get before build
  -h,  --help                Show this help message and exit

Examples:
  .\scripts\build_windows_release.ps1 --help
  .\scripts\build_windows_release.ps1 --zip
  .\scripts\build_windows_release.ps1 --version 1.3.0 --build 202603230900
  .\scripts\build_windows_release.ps1 --version 1.3.0 --type business --brand acme
'@ | Write-Host
}

function Get-RequiredOptionValue {
    param(
        [string[]]$CliArgs,
        [int]$Index,
        [string]$OptionName
    )
    if ($Index + 1 -ge $CliArgs.Length) {
        Stop-WithError "Missing value for option: $OptionName"
    }
    return $CliArgs[$Index + 1]
}

function Parse-Args {
    param([string[]]$CliArgs)

    $i = 0
    while ($i -lt $CliArgs.Length) {
        $arg = $CliArgs[$i]
        switch ($arg) {
            '--help' { Show-Help; exit 0 }
            '-h' { Show-Help; exit 0 }
            '--version' { $script:Version = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-v' { $script:Version = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-Version' { $script:Version = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--build' { $script:BuildNumber = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--build-number' { $script:BuildNumber = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-bn' { $script:BuildNumber = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-Build' { $script:BuildNumber = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-BuildNumber' { $script:BuildNumber = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--arch' { $script:Arch = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-ar' { $script:Arch = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-Arch' { $script:Arch = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--type' { $script:Type = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-t' { $script:Type = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-Type' { $script:Type = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--brand' { $script:Brand = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-br' { $script:Brand = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '-Brand' { $script:Brand = Get-RequiredOptionValue -CliArgs $CliArgs -Index $i -OptionName $arg; $i += 2; continue }
            '--exe' { $script:PackageFormat = "exe"; $i += 1; continue }
            '--zip' { $script:PackageFormat = "zip"; $i += 1; continue }
            '--force-pub-get' { $script:ForcePubGet = $true; $i += 1; continue }
            '-ForcePubGet' { $script:ForcePubGet = $true; $i += 1; continue }
            default { Stop-WithError "Unknown option: $arg" }
        }
    }
}

function Invoke-FlutterPubGetWithFallback {
    Write-Step "Resolving flutter dependencies"
    $oldStorage = $env:FLUTTER_STORAGE_BASE_URL
    $oldPub = $env:PUB_HOSTED_URL
    $oldGit = $env:FLUTTER_GIT_URL

    try {
        # Primary: official overseas sources
        $env:FLUTTER_STORAGE_BASE_URL = "https://storage.googleapis.com"
        $env:PUB_HOSTED_URL = "https://pub.dev"
        if (Test-Path Env:\FLUTTER_GIT_URL) {
            Remove-Item Env:\FLUTTER_GIT_URL -ErrorAction SilentlyContinue
        }
        & flutter pub get
        if ($LASTEXITCODE -eq 0) {
            Write-Ok "flutter pub get succeeded with official sources"
            return
        }

        Write-Warn "flutter pub get failed with official sources. Retrying with China mirrors..."

        # Fallback: China mirrors
        $env:FLUTTER_STORAGE_BASE_URL = "https://storage.flutter-io.cn"
        $env:PUB_HOSTED_URL = "https://pub.flutter-io.cn"
        $env:FLUTTER_GIT_URL = "https://gitee.com/mirrors/flutter.git"
        & flutter pub get
        if ($LASTEXITCODE -ne 0) {
            Stop-WithError "flutter pub get failed (official and China mirrors)"
        }
        Write-Ok "flutter pub get succeeded with China mirrors"
    } finally {
        if ($oldStorage) { $env:FLUTTER_STORAGE_BASE_URL = $oldStorage } else { Remove-Item Env:\FLUTTER_STORAGE_BASE_URL -ErrorAction SilentlyContinue }
        if ($oldPub) { $env:PUB_HOSTED_URL = $oldPub } else { Remove-Item Env:\PUB_HOSTED_URL -ErrorAction SilentlyContinue }
        if ($oldGit) { $env:FLUTTER_GIT_URL = $oldGit } else { Remove-Item Env:\FLUTTER_GIT_URL -ErrorAction SilentlyContinue }
    }
}

function Test-NeedFlutterPubGet {
    param(
        [string]$ProjectRootPath,
        [bool]$Force
    )
    if ($Force) { return $true }

    $packageConfig = Join-Path $ProjectRootPath ".dart_tool\package_config.json"
    if (-not (Test-Path $packageConfig)) { return $true }

    $lockFile = Join-Path $ProjectRootPath "pubspec.lock"
    if (-not (Test-Path $lockFile)) { return $true }

    $packageConfigTime = (Get-Item $packageConfig).LastWriteTimeUtc
    $lockTime = (Get-Item $lockFile).LastWriteTimeUtc
    if ($packageConfigTime -lt $lockTime) { return $true }

    $overrides = Join-Path $ProjectRootPath "pubspec_overrides.yaml"
    if (Test-Path $overrides) {
        $overrideTime = (Get-Item $overrides).LastWriteTimeUtc
        if ($packageConfigTime -lt $overrideTime) { return $true }
    }

    return $false
}

function Stop-BotsecRuntimeProcesses {
    param(
        [string]$ProjectRootPath
    )
    try {
        $targets = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
            $_.Name -match 'bot_sec_manager|flutter|dart|dartaotruntime' -and
            $_.CommandLine -like "*$ProjectRootPath*"
        }
        foreach ($p in $targets) {
            try {
                Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
            } catch {}
        }
        if ($targets) {
            Write-Warn "Stopped runtime processes that may lock plugin DLLs"
            Start-Sleep -Milliseconds 800
        }
    } catch {}
}

function Copy-ItemWithRetry {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Destination,
        [int]$Retries = 4,
        [int]$DelayMs = 700
    )
    $lastError = $null
    for ($i = 1; $i -le $Retries; $i++) {
        try {
            Copy-Item -Force $Source $Destination -ErrorAction Stop
            return $true
        } catch {
            $lastError = $_
            Start-Sleep -Milliseconds $DelayMs
        }
    }
    if ($lastError) {
        Write-Warn ("Copy failed after {0} retries: {1}" -f $Retries, $lastError.Exception.Message)
    }
    return $false
}

function Test-Command([string]$cmd) {
    $null = Get-Command $cmd -ErrorAction SilentlyContinue
    return $?
}

function Get-CMakeCommand() {
    # 1) Try PATH first
    $cmakeCmd = Get-Command "cmake" -ErrorAction SilentlyContinue
    if ($cmakeCmd) {
        return $cmakeCmd.Source
    }

    # 2) Try Visual Studio bundled CMake via vswhere
    $vswhereCandidates = @(
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe",
        "${env:ProgramFiles}\Microsoft Visual Studio\Installer\vswhere.exe"
    )

    foreach ($vswhere in $vswhereCandidates) {
        if (-not (Test-Path $vswhere)) { continue }

        $installPaths = @(
            (& $vswhere -latest -products * -property installationPath 2>$null),
            (& $vswhere -products * -property installationPath 2>$null)
        ) | Where-Object { $_ -and $_.Trim().Length -gt 0 } | Select-Object -Unique

        foreach ($installPath in $installPaths) {
            $candidate = Join-Path $installPath "Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe"
            if (Test-Path $candidate) {
                return $candidate
            }
        }
    }

    # 3) Fallback common VS layouts
    $fallbackCandidates = @(
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\Professional\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\Enterprise\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\BuildTools\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2019\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2019\Professional\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2019\Enterprise\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe",
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2019\BuildTools\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe"
    )

    foreach ($candidate in $fallbackCandidates) {
        if (Test-Path $candidate) {
            return $candidate
        }
    }

    return $null
}

function Get-CSharpCompiler() {
    $cscCmd = Get-Command "csc" -ErrorAction SilentlyContinue
    if ($cscCmd) {
        return $cscCmd.Source
    }

    $candidates = @(
        "C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe",
        "C:\Windows\Microsoft.NET\Framework\v4.0.30319\csc.exe"
    )

    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    return $null
}

function New-ZipPayloadArchive {
    param(
        [Parameter(Mandatory = $true)][string]$SourceDirectory,
        [Parameter(Mandatory = $true)][string]$OutputZipPath
    )

    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem

    if (Test-Path -LiteralPath $OutputZipPath) {
        Remove-Item -Force -LiteralPath $OutputZipPath
    }
    # 使用 ZipArchive 手工逐文件写入，避免 Compress-Archive 产物在部分 7-Zip 环境下的兼容性问题。
    $sourceRoot = (Resolve-Path -LiteralPath $SourceDirectory).Path
    $files = Get-ChildItem -LiteralPath $sourceRoot -Recurse -File

    $zip = [System.IO.Compression.ZipFile]::Open($OutputZipPath, [System.IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in $files) {
            $relativePath = $file.FullName.Substring($sourceRoot.Length).TrimStart('\', '/')
            $entryPath = $relativePath -replace '\\', '/'
            [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
                $zip,
                $file.FullName,
                $entryPath,
                [System.IO.Compression.CompressionLevel]::Optimal
            ) | Out-Null
        }
    } finally {
        $zip.Dispose()
    }
}

function New-ZipPayloadArchiveSafe {
    param(
        [Parameter(Mandatory = $true)][string]$SourceDirectory,
        [Parameter(Mandatory = $true)][string]$OutputZipPath
    )

    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem

    if (Test-Path -LiteralPath $OutputZipPath) {
        Remove-Item -Force -LiteralPath $OutputZipPath
    }

    # Use ZipArchive directly to avoid archive compatibility issues in some environments.
    $sourceRoot = (Resolve-Path -LiteralPath $SourceDirectory).Path
    $files = Get-ChildItem -LiteralPath $sourceRoot -Recurse -File

    $outputZipDir = Split-Path -Parent $OutputZipPath
    if (-not [string]::IsNullOrWhiteSpace($outputZipDir) -and -not (Test-Path -LiteralPath $outputZipDir)) {
        New-Item -ItemType Directory -Path $outputZipDir -Force | Out-Null
    }

    $zip = [System.IO.Compression.ZipFile]::Open($OutputZipPath, [System.IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in $files) {
            $relativePath = $file.FullName.Substring($sourceRoot.Length).TrimStart('\', '/')
            $entryPath = $relativePath -replace '\\', '/'
            [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
                $zip,
                $file.FullName,
                $entryPath,
                [System.IO.Compression.CompressionLevel]::Optimal
            ) | Out-Null
        }
    } finally {
        $zip.Dispose()
    }
}

function New-CustomInstallerExe {
    param(
        [Parameter(Mandatory = $true)][string]$CSharpCompiler,
        [Parameter(Mandatory = $true)][string]$BootstrapSourcePath,
        [Parameter(Mandatory = $true)][string]$PayloadZipPath,
        [Parameter(Mandatory = $true)][string]$OutputExePath,
        [Parameter(Mandatory = $true)][string]$IconPath,
        [Parameter(Mandatory = $true)][string]$IntermediateExePath
    )

    $referenceArgs = @(
        "/r:System.dll",
        "/r:System.Drawing.dll",
        "/r:System.Windows.Forms.dll",
        "/r:System.IO.Compression.dll",
        "/r:System.IO.Compression.FileSystem.dll"
    )

    $compileArgs = @(
        "/nologo",
        "/target:winexe",
        "/optimize+",
        "/codepage:65001",
        "/utf8output",
        "/out:$IntermediateExePath",
        "/win32icon:$IconPath"
    ) + $referenceArgs + @($BootstrapSourcePath)

    & $CSharpCompiler @compileArgs
    if ($LASTEXITCODE -ne 0) {
        Stop-WithError "Custom installer bootstrap compilation failed"
    }

    $stubBytes = [System.IO.File]::ReadAllBytes($IntermediateExePath)
    $payloadBytes = [System.IO.File]::ReadAllBytes($PayloadZipPath)
    $markerBytes = [System.Text.Encoding]::ASCII.GetBytes("BOTSEC_PAYLOAD_V1")
    $payloadLengthBytes = [System.BitConverter]::GetBytes([Int64]$payloadBytes.Length)
    $totalLength = $stubBytes.Length + $payloadBytes.Length + $payloadLengthBytes.Length + $markerBytes.Length
    $combined = New-Object byte[] $totalLength

    [System.Buffer]::BlockCopy($stubBytes, 0, $combined, 0, $stubBytes.Length)
    [System.Buffer]::BlockCopy($payloadBytes, 0, $combined, $stubBytes.Length, $payloadBytes.Length)
    [System.Buffer]::BlockCopy($payloadLengthBytes, 0, $combined, $stubBytes.Length + $payloadBytes.Length, $payloadLengthBytes.Length)
    [System.Buffer]::BlockCopy($markerBytes, 0, $combined, $stubBytes.Length + $payloadBytes.Length + $payloadLengthBytes.Length, $markerBytes.Length)

    $outDir = Split-Path -Parent $OutputExePath
    if (-not (Test-Path -LiteralPath $outDir)) {
        New-Item -ItemType Directory -Path $outDir -Force | Out-Null
    }
    if (Test-Path -LiteralPath $OutputExePath) {
        Remove-Item -Force -LiteralPath $OutputExePath
    }
    [System.IO.File]::WriteAllBytes($OutputExePath, $combined)
}

# 解析本机 7-Zip 安装路径，返回 7z.exe 与 GUI 自解压模块 7z.sfx
function Resolve-SevenZipPaths {
    if ($env:SEVENZIP_EXE -and $env:SEVENZIP_SFX) {
        if ((Test-Path -LiteralPath $env:SEVENZIP_EXE) -and (Test-Path -LiteralPath $env:SEVENZIP_SFX)) {
            return [PSCustomObject]@{ ExePath = $env:SEVENZIP_EXE; SfxPath = $env:SEVENZIP_SFX }
        }
    }

    $exeCandidates = @(
        (Join-Path $env:ProgramFiles "7-Zip\7z.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "7-Zip\7z.exe"),
        (Join-Path $env:USERPROFILE "scoop\apps\7zip\current\7z.exe")
    )
    foreach ($exe in $exeCandidates) {
        if (-not (Test-Path -LiteralPath $exe)) { continue }
        $dir = Split-Path -Parent $exe
        $sfx = Join-Path $dir "7z.sfx"
        if (Test-Path -LiteralPath $sfx) {
            return [PSCustomObject]@{ ExePath = $exe; SfxPath = $sfx }
        }
    }
    $cmd7z = Get-Command "7z" -ErrorAction SilentlyContinue
    if ($cmd7z -and $cmd7z.Source) {
        $candidateDirs = @(
            (Split-Path -Parent $cmd7z.Source)
        )

        if ($cmd7z.Source -like "*\scoop\shims\*") {
            $scoopRoot = if ($env:SCOOP) { $env:SCOOP } else { Join-Path $env:USERPROFILE "scoop" }
            $candidateDirs += @(
                (Join-Path $scoopRoot "apps\7zip\current"),
                (Split-Path -Parent (Resolve-Path (Join-Path $scoopRoot "apps\7zip\current\7z.exe") -ErrorAction SilentlyContinue))
            ) | Where-Object { $_ }
        }

        foreach ($dir in ($candidateDirs | Select-Object -Unique)) {
            $exe = Join-Path $dir "7z.exe"
            $sfx = Join-Path $dir "7z.sfx"
            if ((Test-Path -LiteralPath $exe) -and (Test-Path -LiteralPath $sfx)) {
                return [PSCustomObject]@{ ExePath = $exe; SfxPath = $sfx }
            }
        }
    }

    $scoopRoot = if ($env:SCOOP) { $env:SCOOP } else { Join-Path $env:USERPROFILE "scoop" }
    $scoopSfx = Get-ChildItem (Join-Path $scoopRoot "apps") -Recurse -Filter "7z.sfx" -ErrorAction SilentlyContinue |
        Sort-Object FullName -Descending |
        Select-Object -First 1
    if ($scoopSfx) {
        $dir = Split-Path -Parent $scoopSfx.FullName
        $exe = Join-Path $dir "7z.exe"
        if (Test-Path -LiteralPath $exe) {
            return [PSCustomObject]@{ ExePath = $exe; SfxPath = $scoopSfx.FullName }
        }
    }

    return $null
}

function Get-LatestDirectoryByName {
    param(
        [Parameter(Mandatory = $true)][string[]]$CandidatePaths,
        [string]$NamePattern = '*'
    )

    $dirs = @()
    foreach ($path in $CandidatePaths) {
        if (Test-Path -LiteralPath $path) {
            $dirs += Get-ChildItem -LiteralPath $path -Directory -ErrorAction SilentlyContinue | Where-Object {
                $_.Name -like $NamePattern
            }
        }
    }

    return $dirs | Sort-Object Name -Descending | Select-Object -First 1
}

function Resolve-AppLocalRuntimeFiles {
    $runtimeFiles = New-Object System.Collections.Generic.List[string]

    $vsInstallPath = $null
    $vswhereCandidates = @(
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe",
        "${env:ProgramFiles}\Microsoft Visual Studio\Installer\vswhere.exe"
    )
    foreach ($vswhere in $vswhereCandidates) {
        if (-not (Test-Path -LiteralPath $vswhere)) { continue }
        $detected = & $vswhere -latest -products * -property installationPath 2>$null
        if ($detected) {
            $vsInstallPath = $detected.Trim()
            break
        }
    }

    $vcRedistRoots = @()
    if ($vsInstallPath) {
        $vcRedistRoots += (Join-Path $vsInstallPath "VC\Redist\MSVC")
    }
    $vcRedistRoots += @(
        "C:\BuildTools\VC\Redist\MSVC",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\BuildTools\VC\Redist\MSVC",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022\Community\VC\Redist\MSVC"
    ) | Select-Object -Unique

    $vcRedistVersionDir = Get-LatestDirectoryByName -CandidatePaths $vcRedistRoots -NamePattern '*.*'
    if (-not $vcRedistVersionDir) {
        Stop-WithError "VC runtime redistributable directory not found. Install Visual C++ x64 build tools/redist."
    }

    $vcRuntimeDir = $null
    foreach ($candidate in @(
        (Join-Path $vcRedistVersionDir.FullName "x64\Microsoft.VC143.CRT"),
        (Join-Path $vcRedistVersionDir.FullName "x64\Microsoft.VC142.CRT")
    )) {
        if (Test-Path -LiteralPath $candidate) {
            $vcRuntimeDir = $candidate
            break
        }
    }
    if (-not $vcRuntimeDir) {
        Stop-WithError "VC runtime app-local directory not found under $($vcRedistVersionDir.FullName)"
    }

    foreach ($dll in @("msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll")) {
        $path = Join-Path $vcRuntimeDir $dll
        if (-not (Test-Path -LiteralPath $path)) {
            Stop-WithError "Required VC runtime DLL not found: $path"
        }
        $runtimeFiles.Add($path)
    }

    return $runtimeFiles | Select-Object -Unique
}

function Copy-AppLocalRuntimeFiles {
    param(
        [Parameter(Mandatory = $true)][string[]]$RuntimeFiles,
        [Parameter(Mandatory = $true)][string[]]$TargetDirectories
    )

    foreach ($targetDir in $TargetDirectories) {
        if (-not (Test-Path -LiteralPath $targetDir)) {
            New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
        }

        foreach ($file in $RuntimeFiles) {
            $dest = Join-Path $targetDir ([System.IO.Path]::GetFileName($file))
            Copy-Item -LiteralPath $file -Destination $dest -Force
        }
        Write-Ok "Copied app-local runtimes to $targetDir"
    }
}

# Write UTF-8 with BOM text file for 7-Zip SFX config.
function Write-Utf8BomTextFile {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string]$Text
    )
    $utf8Bom = New-Object System.Text.UTF8Encoding $true
    [System.IO.File]::WriteAllText($FilePath, $Text, $utf8Bom)
}

# 将 Release 目录打成 7z 并与 SFX 模块拼接为单一自解压 exe
function New-SevenZipSfxInstaller {
    param(
        [Parameter(Mandatory = $true)][string]$SevenZipExe,
        [Parameter(Mandatory = $true)][string]$SfxModulePath,
        [Parameter(Mandatory = $true)][string]$BundleDirectory,
        [Parameter(Mandatory = $true)][string]$TempArchivePath,
        [Parameter(Mandatory = $true)][string]$SfxConfigPath,
        [Parameter(Mandatory = $true)][string]$OutputExePath,
        [Parameter(Mandatory = $true)][string]$MainExecutableName
    )
    if (Test-Path -LiteralPath $TempArchivePath) {
        Remove-Item -Force -LiteralPath $TempArchivePath
    }
    Write-Step "Creating 7z payload for SFX"
    Push-Location $BundleDirectory
    try {
        $sevenZipArgs = @("a", "-t7z", "-mx=7", "-y", $TempArchivePath, ".\*")
        & $SevenZipExe @sevenZipArgs
        if ($LASTEXITCODE -ne 0) {
            Stop-WithError "7z archive creation failed (exit code $LASTEXITCODE)"
        }
    } finally {
        Pop-Location
    }

    $sfxConfig = @"
;!@Install@!UTF-8!
Title="ClawdSecbot"
BeginPrompt="将解压 ClawdSecbot 到所选目录并启动 ${MainExecutableName}。Extract to the selected folder and start ${MainExecutableName}. Continue?"
ExtractPathTitle="ClawdSecbot"
ExtractPathText="解压到 / Extract to:"
InstallPath="%LOCALAPPDATA%\\ClawdSecbot"
RunProgram="%T\${MainExecutableName}"
GUIMode="1"
;!@InstallEnd@!
"@
    Write-Utf8BomTextFile -FilePath $SfxConfigPath -Text $sfxConfig

    Write-Step "Merging 7z.sfx, SFX config, and archive into single EXE"
    $sfxBytes = [System.IO.File]::ReadAllBytes($SfxModulePath)
    $cfgBytes = [System.IO.File]::ReadAllBytes($SfxConfigPath)
    $arcBytes = [System.IO.File]::ReadAllBytes($TempArchivePath)
    $totalLen = $sfxBytes.Length + $cfgBytes.Length + $arcBytes.Length
    $combined = New-Object byte[] $totalLen
    [System.Buffer]::BlockCopy($sfxBytes, 0, $combined, 0, $sfxBytes.Length)
    [System.Buffer]::BlockCopy($cfgBytes, 0, $combined, $sfxBytes.Length, $cfgBytes.Length)
    [System.Buffer]::BlockCopy($arcBytes, 0, $combined, $sfxBytes.Length + $cfgBytes.Length, $arcBytes.Length)

    $outDir = Split-Path -Parent $OutputExePath
    if (-not (Test-Path -LiteralPath $outDir)) {
        New-Item -ItemType Directory -Path $outDir -Force | Out-Null
    }
    if (Test-Path -LiteralPath $OutputExePath) {
        Remove-Item -Force -LiteralPath $OutputExePath
    }
    [System.IO.File]::WriteAllBytes($OutputExePath, $combined)
}

Parse-Args -CliArgs $args

# Validate version format
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    Stop-WithError "Invalid version format: $Version (expected X.Y.Z)"
}
if ($BuildNumber -notmatch '^\d+$') {
    Stop-WithError "Invalid build number: $BuildNumber (expected digits only)"
}
$Type = Normalize-PackageType $Type
$Arch = Normalize-Arch $Arch
if (-not [string]::IsNullOrWhiteSpace($Brand)) {
    $Brand = Normalize-Brand $Brand
}
if ($PackageFormat -notin @("exe", "zip")) {
    Stop-WithError "Unsupported package format: $PackageFormat"
}
if ($Type -ne 'business' -and -not [string]::IsNullOrWhiteSpace($Brand)) {
    Stop-WithError "Brand is only allowed when Type=business"
}

# Pre-build icon checks
$requiredIcons = @(
    @{ Path = (Join-Path $ProjectRoot "images\tray_icon.ico"); Desc = "Tray icon (ICO)" },
    @{ Path = (Join-Path $ProjectRoot "windows\runner\resources\app_icon.ico"); Desc = "App icon (ICO)" }
)
foreach ($iconEntry in $requiredIcons) {
    if (-not (Test-Path $iconEntry.Path)) {
        Stop-WithError "Missing required icon: $($iconEntry.Desc) at $($iconEntry.Path). Run scripts/generate_icons.sh first."
    }
    Write-Ok "Found $($iconEntry.Desc): $($iconEntry.Path)"
}

# Ensure exe file icon in Explorer uses project icon (Runner.rc embeds app_icon.ico at link time)
$trayIco = Join-Path $ProjectRoot "images\tray_icon.ico"
$appIco = Join-Path $ProjectRoot "windows\runner\resources\app_icon.ico"
if (Test-Path $trayIco) {
    Copy-Item -Force $trayIco $appIco
    Write-Ok "Synced app_icon.ico from tray_icon.ico (for Explorer/executable icon)"
}

Write-Host "============================================" -ForegroundColor White
Write-Host " ClawdSecbot Windows Release Build"
Write-Host "============================================" -ForegroundColor White
Write-Host "Version:      ${Version}+${BuildNumber}"
Write-Host "Type:         $Type"
if (-not [string]::IsNullOrWhiteSpace($Brand)) {
    Write-Host "Brand:        $Brand"
}
Write-Host "Package:      $PackageFormat"
Write-Host "Arch:         $Arch"
Write-Host "Project Root: $ProjectRoot"
Write-Host ""

# Check prerequisites
if (-not (Test-Command "go")) { Stop-WithError "Go is not installed or not in PATH" }
if (-not (Test-Command "flutter")) { Stop-WithError "Flutter is not installed or not in PATH" }
if (-not (Test-Command "gcc")) {
    Write-Warn "GCC not found. CGO requires a C compiler (e.g. mingw-w64)."
    Write-Warn "Install via: choco install mingw  or  scoop install mingw"
    Stop-WithError "C compiler required for CGO build"
}
$CSharpCompilerPath = $null
if ($PackageFormat -eq "exe") {
    $CSharpCompilerPath = Get-CSharpCompiler
    if (-not $CSharpCompilerPath) {
        Stop-WithError "C# compiler not found. Install .NET Framework build tools or Visual Studio Build Tools."
    }
    Write-Ok "Using C# compiler: $CSharpCompilerPath"
}

# Step 1: Update pubspec version
Write-Step "Updating pubspec.yaml version to ${Version}+${BuildNumber}"
$pubspecPath = Join-Path $ProjectRoot "pubspec.yaml"
$pubspec = Get-Content $pubspecPath -Raw
$updatedPubspec = $pubspec -replace 'version: .+', "version: ${Version}+${BuildNumber}"
if ($updatedPubspec -ne $pubspec) {
    Set-Content -Path $pubspecPath -Value $updatedPubspec -NoNewline
    Write-Ok "pubspec.yaml updated"
} else {
    Write-Ok "pubspec.yaml version already up-to-date"
}

# Step 2: Build Go shared library (DLL)
Write-Step "Building Go shared library ($DllName)"
Push-Location $GoLibDir
try {
    # Clean previous build artifacts
    Remove-Item -Force -ErrorAction SilentlyContinue "${OutputName}.dll", "${OutputName}.h", "lib${OutputName}.dll", "lib${OutputName}.h"

    $env:CGO_ENABLED = "1"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    & go build -buildvcs=false -o "${OutputName}.dll" -buildmode=c-shared .
    if ($LASTEXITCODE -ne 0) { Stop-WithError "Go build failed" }

    # Check output (Go may add 'lib' prefix)
    $builtFile = $null
    if (Test-Path "lib${OutputName}.dll") { $builtFile = "lib${OutputName}.dll" }
    elseif (Test-Path "${OutputName}.dll") { $builtFile = "${OutputName}.dll" }
    else { Stop-WithError "DLL not found after build" }

    # Copy to plugins directory
    if (-not (Test-Path $PluginsDir)) { New-Item -ItemType Directory -Path $PluginsDir | Out-Null }
    Stop-BotsecRuntimeProcesses -ProjectRootPath $ProjectRoot
    Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $PluginsDir "${OutputName}.dll"), (Join-Path $PluginsDir "lib${OutputName}.dll")
    $pluginDllDest = Join-Path $PluginsDir "${OutputName}.dll"
    if (Copy-ItemWithRetry -Source $builtFile -Destination $pluginDllDest) {
        Write-Ok "DLL built and copied to $pluginDllDest"
    } else {
        Write-Warn "Using existing plugin DLL because destination is locked; package step will verify freshness."
    }
} finally {
    Pop-Location
}

# Step 3: Build sandbox hook DLL (MinHook)
$HookDir = Join-Path $GoLibDir "core\sandbox\windows_hook"
$HookBuildDir = Join-Path $HookDir "build"
$existingSandboxDll = Join-Path $PluginsDir "sandbox_hook.dll"
$sandboxHookReady = $false

if (Test-Path $HookDir) {
    Write-Step "Building sandbox hook DLL (MinHook)"
    $cmakeExe = Get-CMakeCommand
    if ($cmakeExe) {
        Write-Ok "Using CMake: $cmakeExe"
        if (Test-Path $HookBuildDir) { Remove-Item -Recurse -Force $HookBuildDir }
        New-Item -ItemType Directory -Path $HookBuildDir | Out-Null
        Push-Location $HookBuildDir
        try {
            # Force x64 generator platform to avoid ARM64 default on some VS BuildTools setups.
            & $cmakeExe .. -A x64 -DCMAKE_BUILD_TYPE=Release -DENABLE_CUSTOM_COMPILER_FLAGS=Off
            if ($LASTEXITCODE -ne 0) {
                Write-Warn "CMake configure failed for sandbox_hook; falling back to existing plugins/sandbox_hook.dll if available."
            } else {
                & $cmakeExe --build . --config Release
                if ($LASTEXITCODE -ne 0) {
                    Write-Warn "CMake build failed for sandbox_hook; falling back to existing plugins/sandbox_hook.dll if available."
                } else {
                    $hookDll = Get-ChildItem -Recurse -Filter "sandbox_hook.dll" | Select-Object -First 1
                    if ($hookDll) {
                        if (-not (Test-Path $PluginsDir)) { New-Item -ItemType Directory -Path $PluginsDir | Out-Null }
                        Copy-Item $hookDll.FullName $existingSandboxDll -Force
                        Write-Ok "sandbox_hook.dll built and copied to $PluginsDir"
                        $sandboxHookReady = $true
                    } else {
                        Write-Warn "sandbox_hook.dll not found after build; will try existing plugins/sandbox_hook.dll"
                    }
                }
            }
        } finally {
            Pop-Location
        }
    } else {
        Write-Warn "CMake not found, skipping sandbox hook DLL build"
        Write-Warn "Install Visual Studio C++ workload or run: choco install cmake / scoop install cmake"
    }
} else {
    Write-Warn "Hook source directory not found, skipping sandbox_hook.dll"
}

if (-not $sandboxHookReady) {
    if (Test-Path $existingSandboxDll) {
        Write-Warn "Using existing sandbox_hook.dll: $existingSandboxDll"
        $sandboxHookReady = $true
    } else {
        Stop-WithError "sandbox_hook.dll is unavailable (build failed and no existing plugin found)"
    }
}

# Step 4: Flutter build
Push-Location $ProjectRoot
try {
    Write-Step "Running flutter clean"
    & flutter clean
    if ($LASTEXITCODE -ne 0) { Write-Warn "flutter clean returned non-zero (continuing)" }

    if (Test-NeedFlutterPubGet -ProjectRootPath $ProjectRoot -Force $ForcePubGet) {
        Invoke-FlutterPubGetWithFallback
    } else {
        Write-Step "Skipping explicit flutter pub get (package config is up-to-date)"
    }

    Write-Step "Building Flutter Windows release"
    $buildArgs = @(
        "build",
        "windows",
        "--release",
        "--no-tree-shake-icons",
        "--dart-define=BUILD_VARIANT=personal",
        "--dart-define=BUILD_TYPE=$Type"
    )

    $oldStorage = $env:FLUTTER_STORAGE_BASE_URL
    $oldPub = $env:PUB_HOSTED_URL
    $oldGit = $env:FLUTTER_GIT_URL
    try {
        # Primary: official overseas sources
        $env:FLUTTER_STORAGE_BASE_URL = "https://storage.googleapis.com"
        $env:PUB_HOSTED_URL = "https://pub.dev"
        if (Test-Path Env:\FLUTTER_GIT_URL) {
            Remove-Item Env:\FLUTTER_GIT_URL -ErrorAction SilentlyContinue
        }
        & flutter @buildArgs
        if ($LASTEXITCODE -ne 0) {
            Write-Warn "flutter build failed with official sources. Retrying with China mirrors..."

            # Fallback: China mirrors
            $env:FLUTTER_STORAGE_BASE_URL = "https://storage.flutter-io.cn"
            $env:PUB_HOSTED_URL = "https://pub.flutter-io.cn"
            $env:FLUTTER_GIT_URL = "https://gitee.com/mirrors/flutter.git"
            & flutter @buildArgs
            if ($LASTEXITCODE -ne 0) { Stop-WithError "flutter build windows failed (official and China mirrors)" }
        }
    } finally {
        if ($oldStorage) { $env:FLUTTER_STORAGE_BASE_URL = $oldStorage } else { Remove-Item Env:\FLUTTER_STORAGE_BASE_URL -ErrorAction SilentlyContinue }
        if ($oldPub) { $env:PUB_HOSTED_URL = $oldPub } else { Remove-Item Env:\PUB_HOSTED_URL -ErrorAction SilentlyContinue }
        if ($oldGit) { $env:FLUTTER_GIT_URL = $oldGit } else { Remove-Item Env:\FLUTTER_GIT_URL -ErrorAction SilentlyContinue }
    }
    Write-Ok "Flutter Windows build completed"
} finally {
    Pop-Location
}

# Step 5: Package output
$bundleDir = Join-Path $ProjectRoot "build\windows\x64\runner\Release"
$outputDir = Join-Path $ProjectRoot "build\windows_release"
$buildStagingDir = Join-Path $ProjectRoot "build"
$installerExeFile = Join-Path $ProjectRoot ("build\" + (Get-ArtifactFileName "exe"))
$releaseZipFile = Join-Path $ProjectRoot ("build\" + (Get-ArtifactFileName "zip"))
$installerPayloadZipFile = Join-Path $buildStagingDir "clawdsecbot_windows_release_payload.zip"
$bootstrapExeFile = Join-Path $buildStagingDir "clawdsecbot_installer_bootstrap.exe"
$bootstrapSourceFile = Join-Path $ProjectRoot "scripts\windows_installer\CustomInstallerBootstrap.cs"
$bundleExeName = "bot_sec_manager.exe"
$uninstallScriptSource = Join-Path $ProjectRoot "scripts\uninstall\uninstall_windows.ps1"

if (-not (Test-Path $bundleDir)) {
    # Try alternative path for older Flutter versions
    $bundleDir = Join-Path $ProjectRoot "build\windows\runner\Release"
}
if (-not (Test-Path $bundleDir)) {
    Stop-WithError "Flutter build output not found at expected paths"
}

Write-Step "Packaging release output"
# Clear previous bundle; if dir/files are locked (e.g. botsec.dll in use), remove what we can and continue
if (Test-Path $outputDir) {
    try {
        Remove-Item -Recurse -Force $outputDir -ErrorAction Stop
    } catch {
        Write-Warn "Could not remove $outputDir entirely (e.g. app or DLL in use). Clearing contents where possible."
        Get-ChildItem -Path $outputDir -Recurse -File | ForEach-Object {
            Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
        }
        Get-ChildItem -Path $outputDir -Recurse -Directory | Sort-Object { $_.FullName.Length } -Descending | ForEach-Object {
            Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
        }
    }
}
New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

# Copy Flutter bundle (overwrite any remaining locked files if possible)
Copy-Item -Recurse -Force "$bundleDir\*" $outputDir

# Copy plugins (overwrite; if botsec.dll is locked by a running process, copy may skip it)
$pluginsDest = Join-Path $outputDir "plugins"
if (Test-Path $PluginsDir) {
    if (-not (Test-Path $pluginsDest)) { New-Item -ItemType Directory -Path $pluginsDest | Out-Null }
    $srcDll = Join-Path $PluginsDir $DllName
    Copy-Item -Recurse -Force "$PluginsDir\*" $pluginsDest -ErrorAction SilentlyContinue
    $destDll = Join-Path $pluginsDest $DllName
    if ((Test-Path $srcDll) -and (Test-Path $destDll) -and ((Get-Item $destDll).LastWriteTime -lt (Get-Item $srcDll).LastWriteTime)) {
        Write-Warn "Plugins copied but $DllName was locked and may be outdated. Close any running instance and re-run to refresh."
    } else {
        Write-Ok "Plugins copied"
    }
}

# Copy tray icon to output (tray_manager resolves paths relative to the exe)
$imagesSrc = Join-Path $ProjectRoot "images"
$imagesDest = Join-Path $outputDir "images"
if (Test-Path $imagesSrc) {
    if (-not (Test-Path $imagesDest)) { New-Item -ItemType Directory -Path $imagesDest | Out-Null }
    Copy-Item "$imagesSrc\tray_icon.ico" $imagesDest -ErrorAction SilentlyContinue
    Copy-Item "$imagesSrc\tray_icon.png" $imagesDest -ErrorAction SilentlyContinue
    Write-Ok "Tray icons copied to $imagesDest"
}

$mainExePath = Join-Path $outputDir $bundleExeName
if (-not (Test-Path -LiteralPath $mainExePath)) {
    Stop-WithError "Main executable not found after packaging: $mainExePath"
}

if (Test-Path -LiteralPath $uninstallScriptSource) {
    $uninstallScriptDest = Join-Path $outputDir "uninstall_windows.ps1"
    Copy-Item -LiteralPath $uninstallScriptSource -Destination $uninstallScriptDest -Force
    Write-Ok "Uninstall script copied beside executable: $uninstallScriptDest"
}
else {
    Write-Warn "Uninstall script not found: $uninstallScriptSource"
}

$runtimeFiles = Resolve-AppLocalRuntimeFiles
Write-Step "Bundling VC/UCRT app-local runtimes"
Copy-AppLocalRuntimeFiles -RuntimeFiles $runtimeFiles -TargetDirectories @(
    $outputDir
)

if ($PackageFormat -eq "zip") {
    if (Test-Path -LiteralPath $releaseZipFile) {
        Remove-Item -Force -LiteralPath $releaseZipFile
    }
    Write-Step "Creating ZIP release package with .NET ZipArchive"
    New-ZipPayloadArchiveSafe -SourceDirectory $outputDir -OutputZipPath $releaseZipFile
    Write-Ok "Release packaged: $releaseZipFile"
} else {
    if (-not (Test-Path -LiteralPath $bootstrapSourceFile)) {
        Stop-WithError "Installer bootstrap source not found: $bootstrapSourceFile"
    }
    if ([string]::IsNullOrWhiteSpace($installerPayloadZipFile)) {
        Stop-WithError "Installer payload ZIP path is empty"
    }
    Write-Step "Creating installer payload ZIP"
    New-ZipPayloadArchiveSafe -SourceDirectory $outputDir -OutputZipPath $installerPayloadZipFile

    Write-Step "Compiling custom Windows installer"
    New-CustomInstallerExe `
        -CSharpCompiler $CSharpCompilerPath `
        -BootstrapSourcePath $bootstrapSourceFile `
        -PayloadZipPath $installerPayloadZipFile `
        -OutputExePath $installerExeFile `
        -IconPath $appIco `
        -IntermediateExePath $bootstrapExeFile
    Remove-Item -Force -ErrorAction SilentlyContinue $installerPayloadZipFile, $bootstrapExeFile
    Write-Ok "Release packaged: $installerExeFile"
}

# Summary
Write-Host ""
Write-Host "============================================" -ForegroundColor Green
Write-Host " Windows Release Build Complete"
Write-Host "============================================" -ForegroundColor Green
Write-Host "Bundle:              $outputDir"
if ($PackageFormat -eq "zip") {
    Write-Host "ZIP Package:         $releaseZipFile"
} else {
    Write-Host "Installer EXE:       $installerExeFile"
}
Write-Host ""
