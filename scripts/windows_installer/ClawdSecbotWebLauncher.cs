using System;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Threading;
using System.Windows.Forms;

namespace ClawdSecbotWebLauncher
{
    internal static class Program
    {
        private const int PreferredPort = 18080;
        private const string BackendExecutableName = "botsec_webd.exe";

        [STAThread]
        private static int Main(string[] args)
        {
            string baseDir = AppDomain.CurrentDomain.BaseDirectory;
            string serverPath = Path.Combine(baseDir, "bin", BackendExecutableName);
            string webRoot = Path.Combine(baseDir, "web");

            try
            {
                Log("Launcher started. BaseDir=" + baseDir);

                if (!File.Exists(serverPath))
                {
                    return Fail("botsec_webd.exe not found: " + serverPath);
                }
                if (!Directory.Exists(webRoot))
                {
                    return Fail("web directory not found: " + webRoot);
                }

                int port = ResolvePort();
                string addr = "127.0.0.1:" + port;
                string url = "http://" + addr + "/";
                string healthUrl = url + "health";

                if (WaitForHealthyEndpoint(healthUrl, TimeSpan.FromSeconds(1), null))
                {
                    Log("Existing healthy backend found: " + healthUrl);
                    OpenBrowser(url);
                    return 0;
                }

                Process serverProcess = StartBackend(serverPath, webRoot, addr, baseDir);
                Log("Started backend pid=" + serverProcess.Id + " addr=" + addr);
                WriteRuntimeState(baseDir, serverProcess.Id, url);

                if (!WaitForHealthyEndpoint(healthUrl, TimeSpan.FromSeconds(20), serverProcess))
                {
                    string exitInfo = serverProcess.HasExited
                        ? " Backend exited with code " + serverProcess.ExitCode + "."
                        : "";
                    StopServer(serverProcess);
                    return Fail("Web backend did not become healthy: " + healthUrl + "." + exitInfo);
                }

                Log("Backend healthy: " + healthUrl);
                OpenBrowser(url);
                return 0;
            }
            catch (Exception ex)
            {
                return Fail("Failed to start ClawdSecbot Web: " + ex.Message);
            }
        }

        private static int ResolvePort()
        {
            string preferredHealthUrl = "http://127.0.0.1:" + PreferredPort + "/health";
            if (WaitForHealthyEndpoint(preferredHealthUrl, TimeSpan.FromSeconds(1), null))
            {
                Log("Preferred port already has healthy backend: " + PreferredPort);
                return PreferredPort;
            }

            if (CanListen(PreferredPort))
            {
                return PreferredPort;
            }

            int fallbackPort = FindFreePort();
            Log("Preferred port " + PreferredPort + " is occupied by another service; using " + fallbackPort);
            return fallbackPort;
        }

        private static Process StartBackend(string serverPath, string webRoot, string addr, string baseDir)
        {
            Process process = new Process
            {
                StartInfo = new ProcessStartInfo
                {
                    FileName = serverPath,
                    Arguments = "--addr " + addr + " --web-root \"" + webRoot + "\"",
                    WorkingDirectory = baseDir,
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    WindowStyle = ProcessWindowStyle.Hidden
                }
            };
            process.Start();
            return process;
        }

        private static int FindFreePort()
        {
            TcpListener listener = new TcpListener(IPAddress.Loopback, 0);
            listener.Start();
            int port = ((IPEndPoint)listener.LocalEndpoint).Port;
            listener.Stop();
            return port;
        }

        private static bool CanListen(int port)
        {
            try
            {
                TcpListener listener = new TcpListener(IPAddress.Loopback, port);
                listener.Start();
                listener.Stop();
                return true;
            }
            catch
            {
                return false;
            }
        }

        private static bool WaitForHealthyEndpoint(string url, TimeSpan timeout, Process process)
        {
            DateTime deadline = DateTime.UtcNow.Add(timeout);
            while (DateTime.UtcNow < deadline)
            {
                if (process != null && process.HasExited)
                {
                    Log("Backend process exited before health check succeeded. ExitCode=" + process.ExitCode);
                    return false;
                }

                try
                {
                    HttpWebRequest request = (HttpWebRequest)WebRequest.Create(url);
                    request.Timeout = 1000;
                    using (HttpWebResponse response = (HttpWebResponse)request.GetResponse())
                    using (Stream stream = response.GetResponseStream())
                    using (StreamReader reader = new StreamReader(stream, Encoding.UTF8))
                    {
                        string body = reader.ReadToEnd();
                        if (response.StatusCode == HttpStatusCode.OK &&
                            body.IndexOf("\"success\":true", StringComparison.OrdinalIgnoreCase) >= 0)
                        {
                            return true;
                        }
                    }
                }
                catch (Exception ex)
                {
                    Log("Health check pending for " + url + ": " + ex.Message);
                    Thread.Sleep(300);
                }
            }
            return false;
        }

        private static void OpenBrowser(string url)
        {
            Log("Opening browser: " + url);
            Process.Start(new ProcessStartInfo
            {
                FileName = url,
                UseShellExecute = true
            });
        }

        private static void WriteRuntimeState(string baseDir, int pid, string url)
        {
            try
            {
                string statePath = Path.Combine(baseDir, "clawdsecbot-web.runtime");
                File.WriteAllText(statePath, "pid=" + pid + Environment.NewLine + "url=" + url + Environment.NewLine, Encoding.UTF8);
            }
            catch (Exception ex)
            {
                Log("Failed to write runtime state: " + ex.Message);
            }
        }

        private static void StopServer(Process serverProcess)
        {
            try
            {
                if (serverProcess != null && !serverProcess.HasExited)
                {
                    Log("Stopping backend pid=" + serverProcess.Id);
                    serverProcess.Kill();
                    serverProcess.WaitForExit(3000);
                }
            }
            catch (Exception ex)
            {
                Log("Failed to stop backend: " + ex.Message);
            }
        }

        private static int Fail(string message)
        {
            Log("ERROR: " + message);
            MessageBox.Show(
                message + Environment.NewLine + Environment.NewLine + "Log: " + LogPath(),
                "ClawdSecbot Web",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
            return 1;
        }

        private static void Log(string message)
        {
            try
            {
                string path = LogPath();
                Directory.CreateDirectory(Path.GetDirectoryName(path));
                File.AppendAllText(
                    path,
                    DateTime.Now.ToString("yyyy-MM-dd HH:mm:ss.fff") + " " + message + Environment.NewLine,
                    Encoding.UTF8);
            }
            catch
            {
            }
        }

        private static string LogPath()
        {
            string dir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "ClawdSecbotWeb");
            return Path.Combine(dir, "launcher.log");
        }
    }
}
