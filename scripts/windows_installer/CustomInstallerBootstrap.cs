using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows.Forms;

namespace ClawdSecbotInstaller
{
    internal static class InstallerConstants
    {
        public const string AppName = "ClawdSecbot";
        public const string MainExecutableName = "bot_sec_manager.exe";
        public const string PayloadMarker = "BOTSEC_PAYLOAD_V1";
        public const string InstallManifestName = ".clawdsecbot.install-manifest";
        public const string InstallMarkerName = ".clawdsecbot.installed";
    }

    internal static class InstallerText
    {
        public const string WindowTitle = "ClawdSecbot 安装程序 / ClawdSecbot Setup";
        public const string BannerTitle = "ClawdSecbot";
        public const string BannerSubtitle = "AI Bot 安全防护桌面工具 / Desktop security for AI bots";
        public const string InstallLocation = "安装位置 / Install location";
        public const string Browse = "浏览... / Browse...";
        public const string Options = "安装选项 / Installation options";
        public const string LaunchAfterInstall = "安装完成后启动 ClawdSecbot / Launch ClawdSecbot after installation";
        public const string CreateDesktopShortcut = "创建桌面快捷方式 / Create Desktop shortcut";
        public const string CreateStartMenuShortcut = "创建开始菜单快捷方式 / Create Start Menu shortcut";
        public const string Install = "安装 / Install";
        public const string Upgrade = "升级 / Upgrade";
        public const string Cancel = "取消 / Cancel";
        public const string ReadyToInstall = "准备就绪，等待安装。 / Ready to install.";
        public const string PreparingPayload = "正在准备安装内容... / Preparing installer payload...";
        public const string ReadingPayload = "正在读取安装包内容... / Reading installer payload...";
        public const string PreparingUpgrade = "检测到现有安装，正在准备升级... / Existing installation detected. Preparing upgrade...";
        public const string RemovingOldFiles = "正在清理旧版本程序文件... / Removing previous application files...";
        public const string ExtractingFiles = "正在解压程序文件... / Extracting application files...";
        public const string CreatingShortcuts = "正在创建快捷方式... / Creating shortcuts...";
        public const string InstallCompleted = "安装完成。 / Installation completed.";
        public const string InstallFailed = "安装失败。 / Installation failed.";
        public const string InstallSucceededMessage = "ClawdSecbot 已成功安装。 / ClawdSecbot was installed successfully.";
        public const string ChooseInstallLocation = "请选择安装目录。 / Please choose an install location.";
        public const string ChooseInstallFolder = "请选择 ClawdSecbot 的安装位置。 / Choose where to install ClawdSecbot.";
        public const string InvalidInstallLocation = "安装目录无效。 / The install location is invalid.";
        public const string RunningAppPrompt = "检测到目标目录中的 ClawdSecbot 正在运行。\r\n请先关闭程序后再继续升级安装。\r\n\r\nClawdSecbot is currently running from the selected folder.\r\nPlease close it before continuing with the upgrade.";
        public const string UpgradePrompt = "检测到该目录中已安装 ClawdSecbot。\r\n继续将覆盖程序文件，但会保留用户数据与现有配置。\r\n\r\nAn existing ClawdSecbot installation was found in this folder.\r\nContinuing will replace application files while keeping user data and existing configuration.\r\n\r\n是否继续升级覆盖？ / Continue with upgrade?";
        public const string MarkerWriteFailed = "安装已完成，但写入安装标记失败。 / Installation completed, but writing the install marker failed.";
    }

    internal static class Program
    {
        [STAThread]
        private static void Main()
        {
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            Application.Run(new InstallerForm());
        }
    }

    internal sealed class InstallerForm : Form
    {
        private readonly TextBox installPathTextBox;
        private readonly ProgressBar progressBar;
        private readonly Label statusLabel;
        private readonly Button installButton;
        private readonly Button cancelButton;
        private readonly CheckBox launchCheckBox;
        private readonly CheckBox desktopShortcutCheckBox;
        private readonly CheckBox startMenuShortcutCheckBox;
        private readonly BackgroundWorker worker;

        public InstallerForm()
        {
            Text = InstallerText.WindowTitle;
            FormBorderStyle = FormBorderStyle.FixedDialog;
            MaximizeBox = false;
            MinimizeBox = false;
            StartPosition = FormStartPosition.CenterScreen;
            ClientSize = new Size(680, 448);
            Font = new Font("Segoe UI", 9F, FontStyle.Regular, GraphicsUnit.Point);
            BackColor = Color.White;

            var bannerPanel = new Panel
            {
                Dock = DockStyle.Top,
                Height = 108,
                BackColor = Color.FromArgb(18, 61, 113)
            };
            Controls.Add(bannerPanel);

            var titleLabel = new Label
            {
                AutoSize = false,
                Bounds = new Rectangle(24, 18, 360, 32),
                Text = InstallerText.BannerTitle,
                ForeColor = Color.White,
                Font = new Font("Segoe UI Semibold", 20F, FontStyle.Bold, GraphicsUnit.Point)
            };
            bannerPanel.Controls.Add(titleLabel);

            var subtitleLabel = new Label
            {
                AutoSize = false,
                Bounds = new Rectangle(24, 56, 560, 24),
                Text = InstallerText.BannerSubtitle,
                ForeColor = Color.FromArgb(222, 232, 245),
                Font = new Font("Segoe UI", 10F, FontStyle.Regular, GraphicsUnit.Point)
            };
            bannerPanel.Controls.Add(subtitleLabel);

            var installToLabel = new Label
            {
                AutoSize = true,
                Location = new Point(24, 132),
                Text = InstallerText.InstallLocation
            };
            Controls.Add(installToLabel);

            installPathTextBox = new TextBox
            {
                Location = new Point(24, 156),
                Width = 528,
                Text = GetDefaultInstallPath()
            };
            Controls.Add(installPathTextBox);

            var browseButton = new Button
            {
                Text = InstallerText.Browse,
                Location = new Point(562, 153),
                Size = new Size(94, 30)
            };
            browseButton.Click += BrowseButton_Click;
            Controls.Add(browseButton);

            var optionsLabel = new Label
            {
                AutoSize = true,
                Location = new Point(24, 208),
                Text = InstallerText.Options
            };
            Controls.Add(optionsLabel);

            launchCheckBox = new CheckBox
            {
                AutoSize = true,
                Location = new Point(28, 236),
                Text = InstallerText.LaunchAfterInstall,
                Checked = true
            };
            Controls.Add(launchCheckBox);

            desktopShortcutCheckBox = new CheckBox
            {
                AutoSize = true,
                Location = new Point(28, 264),
                Text = InstallerText.CreateDesktopShortcut,
                Checked = true
            };
            Controls.Add(desktopShortcutCheckBox);

            startMenuShortcutCheckBox = new CheckBox
            {
                AutoSize = true,
                Location = new Point(28, 292),
                Text = InstallerText.CreateStartMenuShortcut,
                Checked = true
            };
            Controls.Add(startMenuShortcutCheckBox);

            progressBar = new ProgressBar
            {
                Location = new Point(24, 344),
                Size = new Size(632, 18),
                Minimum = 0,
                Maximum = 100
            };
            Controls.Add(progressBar);

            statusLabel = new Label
            {
                AutoSize = false,
                Location = new Point(24, 371),
                Size = new Size(632, 42),
                Text = InstallerText.ReadyToInstall,
                ForeColor = Color.FromArgb(70, 70, 70)
            };
            Controls.Add(statusLabel);

            installButton = new Button
            {
                Text = InstallerText.Install,
                Location = new Point(478, 414),
                Size = new Size(84, 28)
            };
            installButton.Click += InstallButton_Click;
            Controls.Add(installButton);

            cancelButton = new Button
            {
                Text = InstallerText.Cancel,
                Location = new Point(572, 414),
                Size = new Size(84, 28)
            };
            cancelButton.Click += CancelButton_Click;
            Controls.Add(cancelButton);

            worker = new BackgroundWorker();
            worker.WorkerReportsProgress = true;
            worker.DoWork += Worker_DoWork;
            worker.ProgressChanged += Worker_ProgressChanged;
            worker.RunWorkerCompleted += Worker_RunWorkerCompleted;
        }

        private static string GetDefaultInstallPath()
        {
            return Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "Programs",
                InstallerConstants.AppName);
        }

        private void BrowseButton_Click(object sender, EventArgs e)
        {
            using (var dialog = new FolderBrowserDialog())
            {
                dialog.Description = InstallerText.ChooseInstallFolder;
                dialog.SelectedPath = installPathTextBox.Text;
                dialog.ShowNewFolderButton = true;
                if (dialog.ShowDialog(this) == DialogResult.OK)
                {
                    installPathTextBox.Text = dialog.SelectedPath;
                }
            }
        }

        private void InstallButton_Click(object sender, EventArgs e)
        {
            if (worker.IsBusy)
            {
                return;
            }

            var installPath = installPathTextBox.Text.Trim();
            if (string.IsNullOrWhiteSpace(installPath))
            {
                MessageBox.Show(this, InstallerText.ChooseInstallLocation, Text, MessageBoxButtons.OK, MessageBoxIcon.Warning);
                return;
            }

            try
            {
                installPath = Path.GetFullPath(installPath);
            }
            catch
            {
                MessageBox.Show(this, InstallerText.InvalidInstallLocation, Text, MessageBoxButtons.OK, MessageBoxIcon.Warning);
                return;
            }

            var installState = InspectInstallState(installPath);
            if (installState.IsInstalled)
            {
                if (installState.IsRunning)
                {
                    MessageBox.Show(this, InstallerText.RunningAppPrompt, Text, MessageBoxButtons.OK, MessageBoxIcon.Warning);
                    return;
                }

                var result = MessageBox.Show(
                    this,
                    InstallerText.UpgradePrompt,
                    InstallerText.WindowTitle,
                    MessageBoxButtons.OKCancel,
                    MessageBoxIcon.Information);

                if (result != DialogResult.OK)
                {
                    return;
                }

                installButton.Text = InstallerText.Upgrade;
                statusLabel.Text = InstallerText.PreparingUpgrade;
            }
            else
            {
                installButton.Text = InstallerText.Install;
                statusLabel.Text = InstallerText.PreparingPayload;
            }

            installButton.Enabled = false;
            cancelButton.Enabled = false;
            progressBar.Value = 0;

            worker.RunWorkerAsync(new InstallOptions
            {
                InstallPath = installPath,
                LaunchAfterInstall = launchCheckBox.Checked,
                CreateDesktopShortcut = desktopShortcutCheckBox.Checked,
                CreateStartMenuShortcut = startMenuShortcutCheckBox.Checked,
                IsUpgrade = installState.IsInstalled
            });
        }

        private void CancelButton_Click(object sender, EventArgs e)
        {
            if (!worker.IsBusy)
            {
                Close();
            }
        }

        private void Worker_DoWork(object sender, DoWorkEventArgs e)
        {
            var options = (InstallOptions)e.Argument;
            var backgroundWorker = (BackgroundWorker)sender;
            InstallPayload(options, backgroundWorker);
            e.Result = options;
        }

        private void Worker_ProgressChanged(object sender, ProgressChangedEventArgs e)
        {
            progressBar.Value = Math.Max(progressBar.Minimum, Math.Min(progressBar.Maximum, e.ProgressPercentage));
            if (e.UserState is string)
            {
                statusLabel.Text = (string)e.UserState;
            }
        }

        private void Worker_RunWorkerCompleted(object sender, RunWorkerCompletedEventArgs e)
        {
            installButton.Enabled = true;
            cancelButton.Enabled = true;
            installButton.Text = InstallerText.Install;

            if (e.Error != null)
            {
                statusLabel.Text = InstallerText.InstallFailed;
                MessageBox.Show(this, e.Error.Message, Text, MessageBoxButtons.OK, MessageBoxIcon.Error);
                return;
            }

            progressBar.Value = 100;
            statusLabel.Text = InstallerText.InstallCompleted;
            MessageBox.Show(this, InstallerText.InstallSucceededMessage, Text, MessageBoxButtons.OK, MessageBoxIcon.Information);

            var options = (InstallOptions)e.Result;
            if (options.LaunchAfterInstall)
            {
                var exePath = Path.Combine(options.InstallPath, InstallerConstants.MainExecutableName);
                if (File.Exists(exePath))
                {
                    Process.Start(new ProcessStartInfo
                    {
                        FileName = exePath,
                        WorkingDirectory = options.InstallPath
                    });
                }
            }

            Close();
        }

        private static InstallState InspectInstallState(string installPath)
        {
            var mainExePath = Path.Combine(installPath, InstallerConstants.MainExecutableName);
            var markerPath = Path.Combine(installPath, InstallerConstants.InstallMarkerName);
            var manifestPath = Path.Combine(installPath, InstallerConstants.InstallManifestName);

            bool installed = File.Exists(mainExePath) || File.Exists(markerPath) || File.Exists(manifestPath);
            bool running = installed && IsFileLocked(mainExePath);

            return new InstallState
            {
                IsInstalled = installed,
                IsRunning = running
            };
        }

        private static bool IsFileLocked(string path)
        {
            if (!File.Exists(path))
            {
                return false;
            }

            try
            {
                using (new FileStream(path, FileMode.Open, FileAccess.ReadWrite, FileShare.None))
                {
                    return false;
                }
            }
            catch (IOException)
            {
                return true;
            }
            catch (UnauthorizedAccessException)
            {
                return true;
            }
        }

        private static void InstallPayload(InstallOptions options, BackgroundWorker worker)
        {
            var exePath = Application.ExecutablePath;
            worker.ReportProgress(8, InstallerText.ReadingPayload);
            var payload = ReadPayload(exePath);

            Directory.CreateDirectory(options.InstallPath);

            var tempZip = Path.Combine(Path.GetTempPath(), "clawdsecbot-installer-" + Guid.NewGuid().ToString("N") + ".zip");
            var manifestEntries = new List<string>();

            try
            {
                File.WriteAllBytes(tempZip, payload);

                if (options.IsUpgrade)
                {
                    worker.ReportProgress(16, InstallerText.RemovingOldFiles);
                    RemoveFilesFromPreviousManifest(options.InstallPath);
                }

                worker.ReportProgress(24, InstallerText.ExtractingFiles);
                manifestEntries = ExtractZipToDirectory(tempZip, options.InstallPath, worker);

                worker.ReportProgress(92, InstallerText.CreatingShortcuts);
                if (options.CreateStartMenuShortcut)
                {
                    CreateStartMenuShortcut(options.InstallPath);
                }
                else
                {
                    DeleteStartMenuShortcut();
                }

                if (options.CreateDesktopShortcut)
                {
                    CreateDesktopShortcut(options.InstallPath);
                }
                else
                {
                    DeleteDesktopShortcut();
                }

                WriteInstallMetadata(options.InstallPath, manifestEntries);
            }
            finally
            {
                try
                {
                    if (File.Exists(tempZip))
                    {
                        File.Delete(tempZip);
                    }
                }
                catch
                {
                }
            }
        }

        private static byte[] ReadPayload(string installerExePath)
        {
            byte[] markerBytes = Encoding.ASCII.GetBytes(InstallerConstants.PayloadMarker);
            using (var stream = new FileStream(installerExePath, FileMode.Open, FileAccess.Read, FileShare.Read))
            {
                if (stream.Length < markerBytes.Length + sizeof(long))
                {
                    throw new InvalidOperationException("Installer payload metadata is missing.");
                }

                stream.Seek(-(markerBytes.Length + sizeof(long)), SeekOrigin.End);

                byte[] lengthBytes = new byte[sizeof(long)];
                ReadExactly(stream, lengthBytes, 0, lengthBytes.Length);

                byte[] actualMarker = new byte[markerBytes.Length];
                ReadExactly(stream, actualMarker, 0, actualMarker.Length);

                if (!ByteArrayEquals(markerBytes, actualMarker))
                {
                    throw new InvalidOperationException("Installer payload marker was not found.");
                }

                long payloadLength = BitConverter.ToInt64(lengthBytes, 0);
                long payloadStart = stream.Length - markerBytes.Length - sizeof(long) - payloadLength;
                if (payloadLength <= 0 || payloadStart < 0)
                {
                    throw new InvalidOperationException("Installer payload length is invalid.");
                }

                stream.Seek(payloadStart, SeekOrigin.Begin);
                byte[] payload = new byte[payloadLength];
                ReadExactly(stream, payload, 0, payload.Length);
                return payload;
            }
        }

        private static void ReadExactly(Stream stream, byte[] buffer, int offset, int count)
        {
            int totalRead = 0;
            while (totalRead < count)
            {
                int read = stream.Read(buffer, offset + totalRead, count - totalRead);
                if (read == 0)
                {
                    throw new EndOfStreamException("Unexpected end of installer payload.");
                }
                totalRead += read;
            }
        }

        private static bool ByteArrayEquals(byte[] left, byte[] right)
        {
            if (left.Length != right.Length)
            {
                return false;
            }

            for (int i = 0; i < left.Length; i++)
            {
                if (left[i] != right[i])
                {
                    return false;
                }
            }

            return true;
        }

        private static List<string> ExtractZipToDirectory(string zipPath, string outputDir, BackgroundWorker worker)
        {
            var manifestEntries = new List<string>();

            using (var archive = ZipFile.OpenRead(zipPath))
            {
                int totalEntries = archive.Entries.Count;
                int processedEntries = 0;
                string normalizedOutputDir = EnsureTrailingDirectorySeparator(Path.GetFullPath(outputDir));

                foreach (var entry in archive.Entries)
                {
                    string destinationPath = Path.Combine(outputDir, entry.FullName);
                    string normalizedDestinationPath = Path.GetFullPath(destinationPath);

                    if (!normalizedDestinationPath.StartsWith(normalizedOutputDir, StringComparison.OrdinalIgnoreCase))
                    {
                        throw new InvalidOperationException("Installer payload contains an invalid path: " + entry.FullName);
                    }

                    if (string.IsNullOrEmpty(entry.Name))
                    {
                        Directory.CreateDirectory(normalizedDestinationPath);
                    }
                    else
                    {
                        string directory = Path.GetDirectoryName(normalizedDestinationPath);
                        if (!string.IsNullOrEmpty(directory))
                        {
                            Directory.CreateDirectory(directory);
                        }

                        using (var input = entry.Open())
                        using (var output = new FileStream(normalizedDestinationPath, FileMode.Create, FileAccess.Write, FileShare.None))
                        {
                            input.CopyTo(output);
                        }
                    }

                    manifestEntries.Add(entry.FullName.Replace('/', Path.DirectorySeparatorChar));
                    processedEntries++;
                    int progress = 24 + (int)((processedEntries * 60L) / Math.Max(totalEntries, 1));
                    worker.ReportProgress(progress, InstallerText.ExtractingFiles + " " + entry.FullName);
                }
            }

            return manifestEntries;
        }

        private static void RemoveFilesFromPreviousManifest(string installPath)
        {
            string manifestPath = Path.Combine(installPath, InstallerConstants.InstallManifestName);
            if (!File.Exists(manifestPath))
            {
                return;
            }

            var entries = File.ReadAllLines(manifestPath)
                .Where(line => !string.IsNullOrWhiteSpace(line))
                .OrderByDescending(line => line.Length)
                .ToList();

            foreach (var entry in entries)
            {
                string fullPath = Path.GetFullPath(Path.Combine(installPath, entry));
                string installRoot = EnsureTrailingDirectorySeparator(Path.GetFullPath(installPath));
                if (!fullPath.StartsWith(installRoot, StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }

                if (Directory.Exists(fullPath))
                {
                    TryDeleteDirectoryIfEmpty(fullPath, installRoot);
                }
                else if (File.Exists(fullPath))
                {
                    TryDeleteFile(fullPath);
                }
            }
        }

        private static void WriteInstallMetadata(string installPath, IList<string> manifestEntries)
        {
            string manifestPath = Path.Combine(installPath, InstallerConstants.InstallManifestName);
            string markerPath = Path.Combine(installPath, InstallerConstants.InstallMarkerName);

            var normalizedEntries = manifestEntries
                .Where(entry => !string.IsNullOrWhiteSpace(entry))
                .Select(entry => entry.Replace('/', Path.DirectorySeparatorChar))
                .Distinct(StringComparer.OrdinalIgnoreCase)
                .OrderBy(entry => entry, StringComparer.OrdinalIgnoreCase)
                .ToList();

            try
            {
                File.WriteAllLines(manifestPath, normalizedEntries, Encoding.UTF8);
                File.WriteAllText(markerPath, InstallerConstants.AppName + Environment.NewLine + DateTime.UtcNow.ToString("O"), Encoding.UTF8);
            }
            catch (Exception ex)
            {
                throw new InvalidOperationException(InstallerText.MarkerWriteFailed + Environment.NewLine + ex.Message, ex);
            }
        }

        private static string EnsureTrailingDirectorySeparator(string path)
        {
            if (path.EndsWith(Path.DirectorySeparatorChar.ToString(), StringComparison.Ordinal))
            {
                return path;
            }

            return path + Path.DirectorySeparatorChar;
        }

        private static void TryDeleteFile(string path)
        {
            try
            {
                File.Delete(path);
            }
            catch
            {
            }
        }

        private static void TryDeleteDirectoryIfEmpty(string path, string installRoot)
        {
            string current = path;
            while (!string.IsNullOrEmpty(current) &&
                   current.StartsWith(installRoot, StringComparison.OrdinalIgnoreCase) &&
                   !string.Equals(current.TrimEnd(Path.DirectorySeparatorChar), installRoot.TrimEnd(Path.DirectorySeparatorChar), StringComparison.OrdinalIgnoreCase))
            {
                try
                {
                    if (Directory.Exists(current) && !Directory.EnumerateFileSystemEntries(current).Any())
                    {
                        Directory.Delete(current, false);
                    }
                    else
                    {
                        break;
                    }
                }
                catch
                {
                    break;
                }

                current = Path.GetDirectoryName(current);
            }
        }

        private static void CreateDesktopShortcut(string installPath)
        {
            string desktopDir = Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory);
            string shortcutPath = Path.Combine(desktopDir, InstallerConstants.AppName + ".lnk");
            CreateShortcut(shortcutPath, installPath);
        }

        private static void DeleteDesktopShortcut()
        {
            string desktopDir = Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory);
            string shortcutPath = Path.Combine(desktopDir, InstallerConstants.AppName + ".lnk");
            TryDeleteFile(shortcutPath);
        }

        private static void CreateStartMenuShortcut(string installPath)
        {
            string startMenuDir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.Programs),
                InstallerConstants.AppName);
            Directory.CreateDirectory(startMenuDir);
            string shortcutPath = Path.Combine(startMenuDir, InstallerConstants.AppName + ".lnk");
            CreateShortcut(shortcutPath, installPath);
        }

        private static void DeleteStartMenuShortcut()
        {
            string startMenuDir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.Programs),
                InstallerConstants.AppName);
            string shortcutPath = Path.Combine(startMenuDir, InstallerConstants.AppName + ".lnk");
            TryDeleteFile(shortcutPath);
            TryDeleteDirectoryIfEmpty(startMenuDir, EnsureTrailingDirectorySeparator(Path.GetDirectoryName(startMenuDir)));
        }

        private static void CreateShortcut(string shortcutPath, string installPath)
        {
            string exePath = Path.Combine(installPath, InstallerConstants.MainExecutableName);
            if (!File.Exists(exePath))
            {
                return;
            }

            Type shellType = Type.GetTypeFromProgID("WScript.Shell");
            if (shellType == null)
            {
                return;
            }

            object shell = Activator.CreateInstance(shellType);
            try
            {
                object shortcut = shellType.InvokeMember(
                    "CreateShortcut",
                    System.Reflection.BindingFlags.InvokeMethod,
                    null,
                    shell,
                    new object[] { shortcutPath });

                try
                {
                    Type shortcutType = shortcut.GetType();
                    shortcutType.InvokeMember("TargetPath", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { exePath });
                    shortcutType.InvokeMember("WorkingDirectory", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { installPath });
                    shortcutType.InvokeMember("IconLocation", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { exePath + ",0" });
                    shortcutType.InvokeMember("Description", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { InstallerConstants.AppName });
                    shortcutType.InvokeMember("Save", System.Reflection.BindingFlags.InvokeMethod, null, shortcut, null);
                }
                finally
                {
                    if (shortcut != null && Marshal.IsComObject(shortcut))
                    {
                        Marshal.FinalReleaseComObject(shortcut);
                    }
                }
            }
            finally
            {
                if (shell != null && Marshal.IsComObject(shell))
                {
                    Marshal.FinalReleaseComObject(shell);
                }
            }
        }
    }

    internal sealed class InstallOptions
    {
        public string InstallPath;
        public bool LaunchAfterInstall;
        public bool CreateDesktopShortcut;
        public bool CreateStartMenuShortcut;
        public bool IsUpgrade;
    }

    internal sealed class InstallState
    {
        public bool IsInstalled;
        public bool IsRunning;
    }
}
