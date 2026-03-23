#Requires -Version 5.1
<#
.SYNOPSIS
    Build Windows release for ClawdSecbot.
.DESCRIPTION
    Builds the Go shared library (DLL), Flutter Windows app, and packages the output
    with GNU-style CLI options.
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

function Normalize-PackageType([string]$RawType) {
    $value = if ($null -eq $RawType) { '' } else { $RawType.ToLowerInvariant() }
    switch ($value) {
        'personal' { return 'community' }
        'community' { return 'community' }
        'business' { return 'business' }
        'appstore' { Stop-WithError "Windows zip does not support type=appstore" }
        default { Stop-WithError "Unsupported type: $RawType" }
    }
}

function Normalize-Arch([string]$RawArch) {
    $value = if ($null -eq $RawArch) { '' } else { $RawArch.ToLowerInvariant() }
    switch ($value) {
        'x86_64' { return 'x86_64' }
        'amd64' { return 'x86_64' }
        default { Stop-WithError "Windows zip only supports arch=x86_64" }
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
  Build and package the Windows release artifact for ClawdSecbot.

Options:
  -v,  --version <X.Y.Z>     Semantic version (default: 1.0.0)
  -bn, --build <STAMP>       Build timestamp (default: current time, e.g. 202603230900)
       --build-number <STAMP>
  -ar, --arch <ARCH>         Target arch: x86_64
  -t,  --type <TYPE>         Package type: community|business (default: community)
  -br, --brand <NAME>        Brand suffix, only allowed when type=business
       --force-pub-get       Force flutter pub get before build
  -h,  --help                Show this help message and exit

Examples:
  .\scripts\build_windows_release.ps1 --help
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
    $buildArgs = @("build", "windows", "--release", "--no-tree-shake-icons")

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
$zipFile = Join-Path $ProjectRoot ("build\" + (Get-ArtifactFileName "zip"))

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

# Create zip
if (Test-Path $zipFile) { Remove-Item -Force $zipFile }
Compress-Archive -Path "$outputDir\*" -DestinationPath $zipFile
Write-Ok "Release packaged: $zipFile"

# Summary
Write-Host ""
Write-Host "============================================" -ForegroundColor Green
Write-Host " Windows Release Build Complete"
Write-Host "============================================" -ForegroundColor Green
Write-Host "Bundle:  $outputDir"
Write-Host "Archive: $zipFile"
Write-Host ""
