#Requires -Version 5.1
param(
    [int]$PprofPort = 0,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

# Windows PowerShell 5.1 对无 BOM 的 UTF-8 脚本会按系统 ANSI 解析, 导致中文乱码; 本文件应保存为 UTF-8 BOM.
if ($PSVersionTable.PSEdition -ne 'Core') {
    try {
        chcp 65001 | Out-Null
        [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
        $OutputEncoding = [Console]::OutputEncoding
    } catch {}
}

<#
.SYNOPSIS
    将本仓库在 Windows 上常用的工具链目录置于 PATH 前端, 避免系统级旧版 Go 抢先匹配、CGO 找不到 gcc、Flutter 未注入 PATH 等问题。
#>
function Initialize-BotSecWindowsToolchainPath {
    # Flutter 子进程会查找 PowerShell; 仅含 Go bin 的 PATH 会导致 pub get 失败, 故前置系统目录.
    $system32 = Join-Path $env:SystemRoot "System32"
    $ps10 = Join-Path $system32 "WindowsPowerShell\v1.0"
    foreach ($dir in @($system32, $ps10)) {
        if (Test-Path $dir) {
            $env:Path = "$dir;$env:Path"
        }
    }

    # 优先使用用户 sdk 下的 Go 1.26.x(与 go_lib/go.mod 的 go 版本一致), 避免 Program Files 中 go1.20 抢先被选中.
    $goSdkRoot = Join-Path $env:USERPROFILE "sdk"
    if (Test-Path $goSdkRoot) {
        $goExe = Get-ChildItem -Path $goSdkRoot -Directory -Filter "go1.26*" -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName "bin\go.exe" } |
            Where-Object { Test-Path $_ } |
            Select-Object -First 1
        if ($goExe) {
            $env:Path = "$(Split-Path $goExe -Parent);$env:Path"
        }
    }

    # CGO/c-shared 需要 gcc; 纯 VS 的 cl 与 Go 传入的参数不兼容, 故探测 WinGet 安装的 WinLibs MinGW 并前置其 bin.
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        $wingetPkgs = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
        $winLibsDir = Get-ChildItem -Path $wingetPkgs -Directory -Filter "BrechtSanders.WinLibs.*" -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($winLibsDir) {
            $mingwBin = Join-Path $winLibsDir.FullName "mingw64\bin"
            if (Test-Path (Join-Path $mingwBin "gcc.exe")) {
                $env:Path = "$mingwBin;$env:Path"
            }
        }
    }

    # Flutter 常安装在用户 sdk 目录; 非登录 shell 可能未带上用户 PATH.
    $flutterBin = Join-Path $env:USERPROFILE "sdk\flutter\bin"
    if (Test-Path (Join-Path $flutterBin "flutter.bat")) {
        $env:Path = "$flutterBin;$env:Path"
    }

    # flutter pub get 依赖 git; 部分 IDE 子进程 PATH 过短时会缺失.
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        $gitDirs = @(
            (Join-Path $env:ProgramFiles "Git\cmd"),
            (Join-Path ${env:ProgramFiles(x86)} "Git\cmd"),
            "D:\Software\Git\cmd",
            "D:\Software\Git\bin"
        )
        foreach ($d in $gitDirs) {
            if ($d -and (Test-Path (Join-Path $d "git.exe"))) {
                $env:Path = "$d;$env:Path"
                break
            }
        }
    }
}

Initialize-BotSecWindowsToolchainPath

$ProjectRoot = Split-Path -Parent $PSScriptRoot
$GoLibDir = Join-Path $ProjectRoot "go_lib"
$PluginsDir = Join-Path $ProjectRoot "plugins"
$OutputName = "botsec"
$DllName = "$OutputName.dll"

if ($PprofPort -le 0) {
    if ($env:BOTSEC_PPROF_PORT) {
        $parsed = 0
        if ([int]::TryParse($env:BOTSEC_PPROF_PORT, [ref]$parsed) -and $parsed -gt 0) {
            $PprofPort = $parsed
        } else {
            $PprofPort = 6060
        }
    } else {
        $PprofPort = 6060
    }
}

Set-Location $ProjectRoot

function Stop-RepoRuntimeProcesses {
    $targets = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.Name -match 'bot_sec_manager|flutter|dart|dartaotruntime' -and
        $_.CommandLine -like "*$ProjectRoot*"
    }
    foreach ($p in $targets) {
        try {
            Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
        } catch {}
    }
    if ($targets) {
        Start-Sleep -Milliseconds 600
    }
}

function Resolve-BuiltDll {
    $candidates = @(
        (Join-Path $GoLibDir "lib$DllName"),
        (Join-Path $GoLibDir $DllName)
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return $c }
    }
    return $null
}

function Invoke-FlutterPubGetWithFallback {
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
            Write-Host "flutter pub get succeeded with official sources." -ForegroundColor Green
            return
        }

        Write-Host "flutter pub get failed with official sources, retrying China mirrors..." -ForegroundColor Yellow

        # Fallback: China mirrors
        $env:FLUTTER_STORAGE_BASE_URL = "https://storage.flutter-io.cn"
        $env:PUB_HOSTED_URL = "https://pub.flutter-io.cn"
        $env:FLUTTER_GIT_URL = "https://gitee.com/mirrors/flutter.git"
        & flutter pub get
        if ($LASTEXITCODE -ne 0) { throw "flutter pub get failed with both official and China mirrors" }
        Write-Host "flutter pub get succeeded with China mirrors." -ForegroundColor Green
    } finally {
        if ($oldStorage) { $env:FLUTTER_STORAGE_BASE_URL = $oldStorage } else { Remove-Item Env:\FLUTTER_STORAGE_BASE_URL -ErrorAction SilentlyContinue }
        if ($oldPub) { $env:PUB_HOSTED_URL = $oldPub } else { Remove-Item Env:\PUB_HOSTED_URL -ErrorAction SilentlyContinue }
        if ($oldGit) { $env:FLUTTER_GIT_URL = $oldGit } else { Remove-Item Env:\FLUTTER_GIT_URL -ErrorAction SilentlyContinue }
    }
}

function Test-NeedFlutterPubGet {
    param(
        [string]$ProjectRootPath
    )
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

# 读取注册表判断 Windows 是否已开启开发者模式(与 Flutter 要求的 symlink 支持一致).
function Test-WindowsDeveloperModeEnabled {
    try {
        $regPath = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock"
        if (-not (Test-Path $regPath)) {
            return $false
        }
        $raw = (Get-ItemProperty -Path $regPath -Name AllowDevelopmentWithoutDevLicense -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense
        if ($null -eq $raw) {
            return $false
        }
        return [int64]$raw -eq 1
    } catch {
        return $false
    }
}

# 在用户临时目录尝试创建符号链接, 与 Flutter 插件要求一致; 比单独读注册表更贴近实际权限.
function Test-BotSecUserCanCreateSymlink {
    $dir = Join-Path $env:TEMP ("botsec_symlink_probe_" + [Guid]::NewGuid().ToString("N"))
    $target = Join-Path $dir "t.txt"
    $link = Join-Path $dir "l.txt"
    try {
        $null = New-Item -ItemType Directory -Path $dir -Force -ErrorAction Stop
        Set-Content -LiteralPath $target -Value "ok" -Encoding utf8 -ErrorAction Stop
        $null = New-Item -ItemType SymbolicLink -Path $link -Target $target -Force -ErrorAction Stop
        return $true
    } catch {
        return $false
    } finally {
        Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 在调用 flutter 前校验 symlink 前提, 避免 pub get 长时间下载后才失败.
function Assert-FlutterWindowsSymlinkSupport {
    if ($env:BOTSEC_SKIP_DEV_MODE_CHECK -eq "1") {
        return
    }
    if (Test-WindowsDeveloperModeEnabled) {
        return
    }
    if (Test-BotSecUserCanCreateSymlink) {
        return
    }
    Write-Host ""
    Write-Host "[错误] Flutter 含插件工程在 Windows 上需要符号链接, 请先开启系统「开发者模式」." -ForegroundColor Red
    Write-Host "      路径: 设置 -> 系统 -> 开发者选项 -> 开发人员模式(或 隐私和安全性 -> 开发者选项, 视 Windows 版本而定)." -ForegroundColor Yellow
    Write-Host "      快捷打开: start ms-settings:developers" -ForegroundColor DarkGray
    Write-Host "      开启后建议重新打开终端再运行本脚本." -ForegroundColor DarkGray
    Write-Host "      若你已通过组策略等方式授予创建 symlink 权限, 可设 BOTSEC_SKIP_DEV_MODE_CHECK=1 跳过此检查." -ForegroundColor DarkGray
    Write-Host ""
    throw "Windows Developer Mode off: Flutter plugin build requires symlink support (enable Developer Mode or set BOTSEC_SKIP_DEV_MODE_CHECK=1)"
}

Write-Host "============================================" -ForegroundColor White
Write-Host "  BotSecManager - pprof mode (Windows)" -ForegroundColor White
Write-Host "============================================" -ForegroundColor White
Write-Host ""

if (-not $SkipBuild) {
    Write-Host "[1/2] Building Go plugin..." -ForegroundColor Cyan
    if (-not (Test-Path $PluginsDir)) {
        New-Item -ItemType Directory -Path $PluginsDir | Out-Null
    }

    Stop-RepoRuntimeProcesses
    Push-Location $GoLibDir
    try {
        $env:CGO_ENABLED = "1"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        & go build -buildvcs=false -buildmode=c-shared -o $DllName .
        if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    $builtDll = Resolve-BuiltDll
    if (-not $builtDll) {
        throw "Build output not found: expected $DllName or lib$DllName in $GoLibDir"
    }

    $destDll = Join-Path $PluginsDir $DllName
    Copy-Item -Force $builtDll $destDll
    $headerPath = [System.IO.Path]::ChangeExtension($builtDll, ".h")
    if (Test-Path $headerPath) {
        Copy-Item -Force $headerPath (Join-Path $PluginsDir "$OutputName.h")
    }
    Write-Host ("Built and copied: {0}" -f $destDll) -ForegroundColor Green
    Write-Host ""
} else {
    Write-Host "[1/2] Skipping Go plugin build (-SkipBuild)." -ForegroundColor Yellow
    Write-Host ""
}

Write-Host ("[2/2] Starting Flutter with pprof port: {0}" -f $PprofPort) -ForegroundColor Cyan
Write-Host ("pprof URL: http://127.0.0.1:{0}/debug/pprof/" -f $PprofPort) -ForegroundColor DarkGray
Write-Host "Common commands:" -ForegroundColor DarkGray
Write-Host ("  go tool pprof http://127.0.0.1:{0}/debug/pprof/heap" -f $PprofPort) -ForegroundColor DarkGray
Write-Host ("  go tool pprof ""http://127.0.0.1:{0}/debug/pprof/profile?seconds=30""" -f $PprofPort) -ForegroundColor DarkGray
Write-Host ("  go tool pprof http://127.0.0.1:{0}/debug/pprof/goroutine" -f $PprofPort) -ForegroundColor DarkGray
Write-Host ""
Write-Host "============================================" -ForegroundColor White
Write-Host ""

Assert-FlutterWindowsSymlinkSupport

if (Test-NeedFlutterPubGet -ProjectRootPath $ProjectRoot) {
    Write-Host "Flutter dependencies are missing or stale. Running flutter pub get..." -ForegroundColor Yellow
    Invoke-FlutterPubGetWithFallback
} else {
    Write-Host "Flutter dependencies are up-to-date. Skipping flutter pub get." -ForegroundColor DarkGray
}

$env:BOTSEC_PPROF_PORT = "$PprofPort"
$env:FLUTTER_STORAGE_BASE_URL = "https://storage.flutter-io.cn"
$env:PUB_HOSTED_URL = "https://pub.flutter-io.cn"
$env:FLUTTER_GIT_URL = "https://gitee.com/mirrors/flutter.git"
$env:FLUTTER_ALREADY_LOCKED = "true"

& flutter run -d windows --no-pub
exit $LASTEXITCODE
