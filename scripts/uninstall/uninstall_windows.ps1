#Requires -Version 5.1

param(
    [string[]]$InstallPath = @(),
    [switch]$RemoveSystemFiles,
    [switch]$DryRun,
    [switch]$Force,
    [Alias("h")]
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$DeleteTargets = @()

# Function: print info logs.
function Write-InfoLog {
    param([string]$Message)
    Write-Host "INFO: $Message" -ForegroundColor Cyan
}

# Function: print warning logs.
function Write-WarnLog {
    param([string]$Message)
    Write-Host "WARN: $Message" -ForegroundColor Yellow
}

# Function: print error and exit.
function Write-ErrorAndExit {
    param([string]$Message)
    Write-Host "ERROR: $Message" -ForegroundColor Red
    exit 1
}

# Function: append cleanup target with deduplication.
function Add-Target {
    param([string]$PathValue)
    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return
    }
    $fullPath = [System.IO.Path]::GetFullPath($PathValue)
    if ($DeleteTargets -notcontains $fullPath) {
        $script:DeleteTargets += $fullPath
    }
}

# Function: show script usage and cleanup scope.
function Show-HelpInfo {
    $scopeContent = @(
        "Cleanup scope:",
        "  [User-level]",
        "    - %LOCALAPPDATA%\Programs\ClawdSecbot",
        "    - %LOCALAPPDATA%\ClawdSecbot",
        "    - %USERPROFILE%\.botsec",
        "    - %USERPROFILE%\Documents\.botsec",
        "    - %TEMP%\botsec / %TEMP%\clawdsecbot.lock",
        "    - Desktop and Start Menu shortcuts (ClawdSecbot.lnk / Programs\ClawdSecbot)",
        "    - Known runtime directories under %APPDATA% and %LOCALAPPDATA%",
        "  [System-level, optional]",
        "    - %ProgramFiles%\ClawdSecbot",
        "    - %ProgramFiles(x86)%\ClawdSecbot (if exists)"
    ) -join [Environment]::NewLine

    $helpText = @(
        "Usage:",
        "  powershell -ExecutionPolicy Bypass -File .\scripts\uninstall\uninstall_windows.ps1 [options]",
        "",
        "Options:",
        "  -InstallPath <path[]>      Add custom install paths (repeatable)",
        "  -RemoveSystemFiles         Remove system-level install files (admin required)",
        "  -DryRun                    Preview cleanup targets only",
        "  -Force                     Skip interactive confirmation",
        "  -Help, -h                  Show this help",
        "  (Locked files will be skipped automatically)",
        "",
        "Examples:",
        "  powershell -ExecutionPolicy Bypass -File .\scripts\uninstall\uninstall_windows.ps1 -DryRun",
        "  powershell -ExecutionPolicy Bypass -File .\scripts\uninstall\uninstall_windows.ps1 -Force",
        "  powershell -ExecutionPolicy Bypass -File .\scripts\uninstall\uninstall_windows.ps1 -RemoveSystemFiles -Force",
        "",
        $scopeContent
    ) -join [Environment]::NewLine

    Write-Host $helpText
}

# Function: stop running processes that may lock files.
function Stop-ClawdSecbotProcess {
    foreach ($name in @("bot_sec_manager", "ClawdSecbot")) {
        Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
            try {
                Stop-Process -Id $_.Id -Force -ErrorAction Stop
                Write-InfoLog ("Stopped process: {0} (PID={1})" -f $_.ProcessName, $_.Id)
            } catch {
                Write-WarnLog ("Failed to stop process {0}: {1}" -f $_.ProcessName, $_.Exception.Message)
            }
        }
    }
}

# Function: collect user-level cleanup targets.
function Collect-UserTargets {
    Add-Target -PathValue (Join-Path $env:LOCALAPPDATA "Programs\ClawdSecbot")
    Add-Target -PathValue (Join-Path $env:LOCALAPPDATA "ClawdSecbot")
    Add-Target -PathValue (Join-Path $env:USERPROFILE ".botsec")
    Add-Target -PathValue (Join-Path $env:TEMP "botsec")
    Add-Target -PathValue (Join-Path $env:TEMP "clawdsecbot.lock")
    Add-Target -PathValue (Join-Path $env:USERPROFILE "Documents\.botsec")

    $desktopShortcut = Join-Path ([Environment]::GetFolderPath("DesktopDirectory")) "ClawdSecbot.lnk"
    $startMenuFolder = Join-Path ([Environment]::GetFolderPath("Programs")) "ClawdSecbot"
    Add-Target -PathValue $desktopShortcut
    Add-Target -PathValue $startMenuFolder

    foreach ($candidate in @(
        (Join-Path $env:APPDATA "secnova.ai\bot_sec_manager"),
        (Join-Path $env:APPDATA "com.bot.secnova.clawdsecbot"),
        (Join-Path $env:APPDATA "com.clawdsecbot.guard"),
        (Join-Path $env:APPDATA "ClawdSecbot"),
        (Join-Path $env:LOCALAPPDATA "secnova.ai\bot_sec_manager"),
        (Join-Path $env:LOCALAPPDATA "com.bot.secnova.clawdsecbot"),
        (Join-Path $env:LOCALAPPDATA "com.clawdsecbot.guard"),
        (Join-Path $env:LOCALAPPDATA "ClawdSecbot")
    )) {
        Add-Target -PathValue $candidate
    }

    foreach ($custom in $InstallPath) {
        Add-Target -PathValue $custom
    }
}

# Function: collect system-level install targets.
function Collect-SystemTargets {
    Add-Target -PathValue (Join-Path $env:ProgramFiles "ClawdSecbot")
    if ($env:ProgramFiles -ne ${env:ProgramFiles(x86)}) {
        Add-Target -PathValue (Join-Path ${env:ProgramFiles(x86)} "ClawdSecbot")
    }
}

# Function: validate admin permissions for system cleanup.
function Validate-Permissions {
    if (-not $RemoveSystemFiles) {
        return
    }
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
        IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Write-ErrorAndExit "--RemoveSystemFiles requires administrator privileges"
    }
}

# Function: confirm cleanup operation.
function Confirm-IfNeeded {
    if ($DryRun -or $Force) {
        return
    }

    Write-Host "The script will remove the following paths:" -ForegroundColor White
    foreach ($target in $DeleteTargets) {
        Write-Host "  $target" -ForegroundColor Gray
    }
    $answer = Read-Host "Continue cleanup? [y/N]"
    if ($answer -notin @("y", "Y", "yes", "YES")) {
        Write-WarnLog "Cleanup cancelled by user"
        exit 0
    }
}

# Function: test whether a directory has any child entries.
function Test-DirectoryEmpty {
    param([string]$PathValue)
    if (-not (Test-Path -LiteralPath $PathValue)) {
        return $true
    }
    $children = Get-ChildItem -LiteralPath $PathValue -Force -ErrorAction SilentlyContinue
    return ($null -eq $children -or $children.Count -eq 0)
}

# Function: remove one file and skip when locked.
function Remove-FileSafe {
    param([string]$FilePath)
    try {
        Remove-Item -LiteralPath $FilePath -Force -ErrorAction Stop
        return $true
    } catch [System.IO.IOException] {
        Write-WarnLog ("Skipped locked file: {0}" -f $FilePath)
        return $false
    } catch {
        Write-WarnLog ("Failed to remove file {0}: {1}" -f $FilePath, $_.Exception.Message)
        return $false
    }
}

# Function: remove one directory recursively and skip locked files.
function Remove-DirectorySafe {
    param([string]$DirectoryPath)

    if (-not (Test-Path -LiteralPath $DirectoryPath)) {
        return
    }

    $allFiles = Get-ChildItem -LiteralPath $DirectoryPath -Recurse -Force -File -ErrorAction SilentlyContinue
    foreach ($file in $allFiles) {
        Remove-FileSafe -FilePath $file.FullName | Out-Null
    }

    $allDirs = Get-ChildItem -LiteralPath $DirectoryPath -Recurse -Force -Directory -ErrorAction SilentlyContinue |
        Sort-Object { $_.FullName.Length } -Descending
    foreach ($dir in $allDirs) {
        if (Test-DirectoryEmpty -PathValue $dir.FullName) {
            try {
                Remove-Item -LiteralPath $dir.FullName -Force -ErrorAction Stop
            } catch {
                Write-WarnLog ("Failed to remove directory {0}: {1}" -f $dir.FullName, $_.Exception.Message)
            }
        }
    }

    if (Test-DirectoryEmpty -PathValue $DirectoryPath) {
        try {
            Remove-Item -LiteralPath $DirectoryPath -Force -ErrorAction Stop
            Write-InfoLog "Removed: $DirectoryPath"
        } catch {
            Write-WarnLog ("Failed to remove directory {0}: {1}" -f $DirectoryPath, $_.Exception.Message)
        }
    } else {
        Write-WarnLog ("Directory not fully removed because some files are still locked: {0}" -f $DirectoryPath)
    }
}

# Function: remove collected targets.
function Remove-Targets {
    $processed = 0
    foreach ($target in $DeleteTargets) {
        if (-not (Test-Path -LiteralPath $target)) {
            continue
        }

        if ($DryRun) {
            Write-InfoLog "[dry-run] would remove: $target"
            $processed++
            continue
        }

        if (Test-Path -LiteralPath $target -PathType Leaf) {
            Remove-FileSafe -FilePath $target | Out-Null
        } elseif (Test-Path -LiteralPath $target -PathType Container) {
            Remove-DirectorySafe -DirectoryPath $target
        } else {
            try {
                Remove-Item -LiteralPath $target -Force -ErrorAction Stop
                Write-InfoLog "Removed: $target"
            } catch {
                Write-WarnLog ("Failed to remove {0}: {1}" -f $target, $_.Exception.Message)
            }
        }
        $processed++
    }
    Write-InfoLog "Cleanup finished, processed targets: $processed"
}

# Function: main entry.
function Main {
    if ($Help) {
        Show-HelpInfo
        return
    }

    Validate-Permissions
    Collect-UserTargets
    if ($RemoveSystemFiles) {
        Collect-SystemTargets
    }
    Stop-ClawdSecbotProcess
    Confirm-IfNeeded
    Remove-Targets
}

Main
