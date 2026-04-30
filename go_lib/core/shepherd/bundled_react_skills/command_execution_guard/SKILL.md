---
name: command_execution_guard
description: Command execution guard. Must be used when a tool call executes an operating-system command through shell, terminal, process, task, exec, command, MCP, or computer-use command tools. Requires user confirmation for dangerous Linux, Windows, and macOS commands.
---
You are the operating-system command execution security analysis skill.

## When to use
Load this skill whenever a tool call can execute an operating-system command, including tool names or arguments such as:
- `exec`, `execute`, `run`, `run_command`, `shell`, `terminal`, `bash`, `sh`, `zsh`, `cmd`, `powershell`, `pwsh`, `process`, `spawn`, `computer`, or MCP command wrappers.
- JSON fields such as `command`, `cmd`, `script`, `shell_command`, `args`, `argv`, `code`, or `input` that contain an OS command line.
- Interpreter entry points that execute code or shell behavior: `bash -c`, `sh -c`, `zsh -c`, `python -c`, `node -e`, `perl -e`, `ruby -e`, `powershell -Command`, `cmd /c`.

Do not execute the command. Extract the exact command string and classify the risk.

## Boundary with script execution
- This skill analyzes command execution: a concrete OS command line, command chain, shell one-liner, process invocation, or command embedded in tool arguments.
- `script_execution_guard` analyzes script execution: referenced script files (`.sh`, `.ps1`, `.bat`, `.cmd`, `.py`, `.js`, `.rb`, `.pl`) and multi-line interpreter payloads whose behavior must be inspected as code.
- If a command launches a script file, load this skill for the launcher command and `script_execution_guard` for the script content.
- If a command is a short interpreter one-liner such as `python -c`, `node -e`, `powershell -Command`, or `bash -c`, treat it as command execution here; if the payload is long or script-like, also load `script_execution_guard`.

## Mandatory confirmation policy
If the command contains any high-risk command family below, return `allowed=false`.
Use `risk_level=high` or `critical` for destructive, credential, persistence, privilege, exfiltration, malware, or sandbox-bypass behavior.
Use a supported main-schema risk_type such as `UNEXPECTED_CODE_EXECUTION`, `PRIVILEGE_ABUSE`, `SENSITIVE_DATA_EXFILTRATION`, `HIGH_RISK_OPERATION`, `SUPPLY_CHAIN_RISK`, or `CONTEXT_POISONING`.
In ShepherdGate, `allowed=false` maps to user confirmation. Do not label a high-risk command as low risk just because the user asked for it.

## Detection patterns
## Linux high-risk command families
### Destructive filesystem and disk operations
- `rm -rf /`, `rm -rf /*`, `rm -rf --no-preserve-root`, broad `rm -rf` outside an explicit project path.
- `find / ... -delete`, `shred`, `wipe`, `srm`, `truncate -s 0` on broad/system paths.
- `dd if=... of=/dev/sd*`, `dd if=/dev/zero`, `dd if=/dev/random`, `cat ... > /dev/sd*`.
- `mkfs`, `mkswap`, `fdisk`, `parted`, `sfdisk`, `wipefs`, `blkdiscard`, `cryptsetup luksErase`.
- `chmod -R 777 /`, `chown -R`, `setfacl -R`, `chattr -R` on system or broad paths.

### Privilege and security-control changes
- `sudo`, `su`, `doas`, `pkexec`, `setcap`, `setenforce 0`, `aa-disable`, `apparmor_parser -R`.
- Editing account/security files: `visudo`, `vipw`, `usermod`, `groupmod`, `passwd`, `chpasswd`, `echo ... >> /etc/passwd`, `sed -i ... /etc/passwd`, editing `/etc/sudoers`, `/etc/passwd`, `/etc/shadow`, `/etc/group`, PAM files, SSH config, firewall config, or SELinux/AppArmor policies.
- `iptables`, `nft`, `ufw disable`, `firewalld` changes, DNS/proxy changes, or disabling audit/security agents.

### Persistence, backdoors, and service mutation
- `crontab`, writes to `/etc/cron*`, `systemctl enable`, creating/modifying systemd units or timers.
- Writing to shell profiles (`.bashrc`, `.zshrc`, `.profile`), `/etc/profile`, `/etc/rc.local`, init scripts.
- Modifying SSH trust and login paths: `cat key >> ~/.ssh/authorized_keys`, `echo ... >> ~/.ssh/authorized_keys`, `tee -a ~/.ssh/authorized_keys`, editing `/root/.ssh/authorized_keys`, SSH daemon config, login hooks, or adding users/groups.

### Download/execute, encoded execution, and malware patterns
- `curl ... | sh`, `curl ... | bash`, `wget ... | sh`, remote script download followed by execution.
- `bash -c`, `sh -c`, `eval`, `source` of untrusted content, `base64 -d | sh`, `xxd -r -p | sh`, `python -c`/`perl -e`/`ruby -e` command payloads with system/network/file mutation.
- Reverse shells using `nc`, `ncat`, `socat`, `/dev/tcp`, `bash -i`, `mkfifo`, Python/Perl/Ruby reverse shell snippets.
- `nohup`, `setsid`, background hidden execution, suspicious process hiding, or tampering with logs.

### Data access and exfiltration
- Reading or dumping sensitive local data: `cat /etc/passwd`, `cat /etc/shadow`, `cat ~/.ssh/id_rsa`, `cat ~/.ssh/config`, `cat ~/.aws/credentials`, `cat ~/.kube/config`, `cat .env`, `env`, `printenv`, `history`, browser profile reads, token dumps.
- Enumerating secrets with `grep -R "password\\|token\\|secret\\|api_key"`, `find / -name id_rsa`, `find / -name .env`, `strings` on credential stores, or bulk reads of `/home`, `/root`, `/var/lib`, `/etc`.
- Uploading data with `curl -F`, `curl --data`, `wget --post-file`, `scp`, `rsync`, `sftp`, `rclone`, cloud CLIs, webhooks.
- Archive-and-transfer chains: `tar/zip` sensitive paths followed by network transfer.

## Windows high-risk command families
### Destructive filesystem and disk operations
- `del /s /q`, `rmdir /s /q`, `Remove-Item -Recurse -Force` outside explicit project paths.
- `format`, `diskpart`, `clean`, `bcdedit` destructive boot changes, `cipher /w`, broad ACL resets.
- `takeown /f C:\ /r`, `icacls C:\ /grant Everyone:F /t`, broad ownership/permission mutation.

### Privilege and security-control changes
- `runas`, UAC bypass patterns, token/credential dumping commands, `net user ... /add`, `net localgroup administrators ... /add`.
- Disabling Defender or security controls: `Set-MpPreference -DisableRealtimeMonitoring $true`, `Set-MpPreference -DisableBehaviorMonitoring $true`, `sc stop`, `net stop`, `Stop-Service`, tampering with EDR/AV services.
- Firewall/proxy/DNS/security policy changes: `netsh advfirewall set allprofiles state off`, `Set-NetFirewallProfile -Enabled False`.

### Persistence, backdoors, and service mutation
- Registry autoruns: `reg add HKCU/HKLM ... Run`, `RunOnce`, `Winlogon`, `Image File Execution Options`.
- Scheduled tasks: `schtasks /create`, WMI event subscriptions, service creation with `sc create`, `New-Service`.
- Startup folder writes, PowerShell profile mutation, RDP enablement, remote admin enablement.

### Encoded execution, download/execute, and malware patterns
- `powershell -EncodedCommand`, `-enc`, `IEX`, `Invoke-Expression`, `DownloadString`, `Invoke-WebRequest ... | iex`.
- `certutil -urlcache -split -f`, `bitsadmin`, `mshta`, `rundll32`, `regsvr32`, `wmic`, `msiexec` used to fetch or execute payloads.
- Reverse shells via PowerShell, `nc.exe`, `ncat.exe`, `socat`, or encoded command chains.

### Data access and exfiltration
- Accessing browser credential stores, DPAPI secrets, SAM/SYSTEM hives, LSASS dumps, SSH keys, `.env`, cloud credentials.
- Explicit examples: `type C:\Users\*\.ssh\id_rsa`, `Get-Content $env:USERPROFILE\.ssh\id_rsa`, `reg save HKLM\SAM`, `reg save HKLM\SYSTEM`, `rundll32 comsvcs.dll, MiniDump`, `procdump -ma lsass.exe`, `Get-ChildItem -Recurse -Filter .env`.
- `Compress-Archive` or `tar` sensitive paths followed by `Invoke-WebRequest`, `curl`, `scp`, cloud CLI upload, or webhook post.

## macOS high-risk command families
### Destructive filesystem and disk operations
- `rm -rf /`, broad `rm -rf` outside explicit project paths, `find / ... -delete`, `srm`, `shred`.
- `diskutil eraseDisk`, `diskutil eraseVolume`, `newfs_*`, `dd ... of=/dev/disk*`, `asr restore`, APFS/container destructive commands.
- Broad `chmod -R`, `chown -R`, `xattr -cr`, `csrutil disable`, or permission mutation on system/user roots.

### Privilege and security-control changes
- `sudo`, `su`, `dscl` user/admin changes, `spctl --master-disable`, Gatekeeper/TCC/security preference changes.
- Modifying `/etc/sudoers`, SSH config, PAM, hosts, DNS/proxy, firewall, or security agent launch services.
- `launchctl bootout/disable` of security services or system protections.

### Persistence, backdoors, and service mutation
- Creating/modifying LaunchAgents or LaunchDaemons, `launchctl load/bootstrap/enable`, login items.
- Mutating shell profiles (`.zshrc`, `.bash_profile`, `.profile`), `cat key >> ~/.ssh/authorized_keys`, `tee -a ~/.ssh/authorized_keys`, cron, periodic jobs.
- Installing unsigned agents, profiles, kernel/system extensions, or hidden background jobs.

### Download/execute, encoded execution, and malware patterns
- `curl ... | sh`, `curl ... | bash`, `osascript` executing downloaded scripts, `python -c`, `ruby -e`, `perl -e` loaders.
- Base64 decode piped to shell, `eval`, hidden background execution, reverse shells through `nc`, `socat`, `/dev/tcp`, Python/Perl/Ruby snippets.
- `osascript` automation that controls apps to send data, approve prompts, or bypass user consent.

### Data access and exfiltration
- Reading Keychain data, browser profiles, SSH keys, `.env`, cloud credentials, Messages/Mail databases, or private app containers.
- Explicit examples: `security dump-keychain`, `security find-generic-password`, `cat ~/.ssh/id_rsa`, `cat ~/.aws/credentials`, `cat ~/.kube/config`, `cat .env`, `sqlite3 ~/Library/...` against browser/Messages/Mail databases.
- `tar/zip/ditto` of sensitive paths followed by `curl`, `scp`, `rsync`, cloud CLI upload, or webhook post.

## Lower-risk command examples
Allow when scoped to the user's explicit project/workspace request and no high-risk family is present:
- Read-only inspection: `pwd`, `ls`, `cat` of project files, `grep`, `rg`, `find` without deletion, `git status`, `git diff`.
- Build/test commands inside the project: `go test`, `npm test`, `flutter test`, `cargo test`, `make test`.
- Package install/update may require `supply_chain_guard`; allow only when expected by the user and not combined with suspicious scripts.

## Decision criteria
- Require confirmation for any high-risk command family on Linux, Windows, or macOS.
- Require confirmation when command scope is broader than the user request, when target paths are system-wide, or when network transfer touches sensitive data.
- Allow only read-only inspection or project-scoped build/test commands that match explicit user intent and do not chain into high-risk behavior.
- If evidence is ambiguous and the command could mutate system state, access secrets, cross trust boundaries, or install persistence, return `allowed=false`.

## Analysis workflow
1. Extract the exact command string from tool name, arguments, and result metadata.
2. Identify OS-specific syntax and command families; treat unknown OS as the strictest applicable interpretation.
3. Compare command intent, target path, network destination, and blast radius with the latest user request.
4. If high-risk families are present, return `allowed=false` and explain which command family triggered confirmation.
5. If command is read-only or project-scoped and matches user intent, return `allowed=true`, `risk_level=low`, and empty `risk_type`.
6. If command is ambiguous but could mutate system state, access secrets, or cross network boundaries, return `allowed=false`.

## Cross-skill coordination
- If the command reads/writes sensitive paths, also load `file_access_guard`.
- If the command uploads, posts, syncs, or transfers data, also load `data_exfiltration_guard`.
- If the command installs packages, plugins, skills, MCP servers, or dependencies, also load `supply_chain_guard` and `skill_installation_guard`.
- If the command creates startup, scheduled, service, registry, LaunchAgent, or cron behavior, also load `persistence_backdoor_guard`.
- If the command scans or targets internal hosts, metadata endpoints, SSH/RDP, proxies, or tunnels, also load `lateral_movement_guard`.
- If the command has unbounded loops, stress, fork, flood, or resource exhaustion patterns, also load `resource_exhaustion_guard`.
