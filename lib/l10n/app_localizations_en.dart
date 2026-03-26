// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'ClawdSecbot';

  @override
  String get showWindow => 'Show Window';

  @override
  String get exit => 'Exit';

  @override
  String get idleTitle => 'ClawdSecbot Security';

  @override
  String get idleSubtitle =>
      'Scan your Clawdbot configuration for security risks';

  @override
  String get startScan => 'Start Security Scan';

  @override
  String get scanning => 'Scanning...';

  @override
  String get scanComplete => 'Scan Complete';

  @override
  String get rescan => 'Rescan';

  @override
  String get rescanConfirmTitle => 'Confirm Rescan';

  @override
  String rescanConfirmMessage(int count) {
    return 'There are currently $count Bot asset(s) under protection. Rescanning will stop all active protections. Continue?';
  }

  @override
  String get continueButton => 'Continue';

  @override
  String get checkingProtectionStatus => 'Checking protection status...';

  @override
  String get configuration => 'Configuration';

  @override
  String get status => 'Status';

  @override
  String get found => 'Found';

  @override
  String get notFound => 'Not Found';

  @override
  String get path => 'Path';

  @override
  String get gatewayConfiguration => 'Gateway Configuration';

  @override
  String get noGatewayConfig => 'No gateway configuration found';

  @override
  String get port => 'Port';

  @override
  String get bind => 'Bind';

  @override
  String get auth => 'Auth';

  @override
  String get controlUi => 'Control UI';

  @override
  String get enabled => 'Enabled';

  @override
  String get disabled => 'Disabled';

  @override
  String get securityFindings => 'Security Findings';

  @override
  String get noSecurityIssues => 'No security issues found';

  @override
  String get secureConfigMessage => 'Your Clawdbot configuration looks secure!';

  @override
  String get testGoIntegration => 'Test Go Integration';

  @override
  String get goIntegrationTest => 'Go Integration Test';

  @override
  String get close => 'Close';

  @override
  String errorCallingGo(String error) {
    return 'Error calling Go: $error';
  }

  @override
  String get settings => 'Global Settings';

  @override
  String get language => 'Language';

  @override
  String get switchLanguage => 'Switch Language';

  @override
  String get menuHelp => 'Help';

  @override
  String aboutApp(String appName) {
    return 'About $appName';
  }

  @override
  String get buildNumber => 'Build';

  @override
  String get currentPlatform => 'Platform';

  @override
  String aboutVersionWithBuild(String version, String build) {
    return 'Version $version ($build)';
  }

  @override
  String get aboutCopyright =>
      'Copyright © 2026 secnova.ai. All rights reserved.';

  @override
  String get riskNonLoopbackBinding => 'Non-loopback address binding';

  @override
  String riskNonLoopbackBindingDesc(String bind) {
    return 'Gateway is bound to \"$bind\", which allows external access. Consider binding to 127.0.0.1 only.';
  }

  @override
  String get riskNoAuth => 'No authentication configured';

  @override
  String get riskNoAuthDesc =>
      'Gateway has no authentication enabled. Anyone with network access can connect.';

  @override
  String get riskWeakPassword => 'Weak authentication password';

  @override
  String get riskWeakPasswordDesc =>
      'Password length is less than 12 characters. Use a stronger password.';

  @override
  String get riskAllPluginsAllowed => 'All plugins allowed';

  @override
  String get riskAllPluginsAllowedDesc =>
      'Wildcard plugin permissions enabled. This may allow untrusted code execution.';

  @override
  String get riskControlUiEnabled => 'Control UI enabled';

  @override
  String get riskControlUiEnabledDesc =>
      'Web control interface is enabled. Ensure it is properly secured.';

  @override
  String get riskRunningAsRoot => 'Running as root';

  @override
  String get riskRunningAsRootDesc =>
      'Application is running with root privileges. This increases the attack surface.';

  @override
  String get riskConfigPermUnsafe => 'Config File Permission Unsafe';

  @override
  String riskConfigPermUnsafeDesc(String path, String current) {
    return 'Config file permissions are $current, expected 600. Run chmod 600 $path to fix.';
  }

  @override
  String get riskConfigDirPermUnsafe => 'Config Directory Permission Unsafe';

  @override
  String riskConfigDirPermUnsafeDesc(String path, String current) {
    return 'Config directory permissions are $current, expected 700. Run chmod 700 $path to fix.';
  }

  @override
  String get riskSandboxDisabledDefault => 'Default Sandbox Disabled';

  @override
  String get riskSandboxDisabledDefaultDesc =>
      'Default sandbox mode is set to \'none\'. Consider enabling sandbox isolation.';

  @override
  String get riskSandboxDisabledAgent => 'Agent Sandbox Disabled';

  @override
  String riskSandboxDisabledAgentDesc(String agent) {
    return 'Sandbox mode for agent \'$agent\' is set to \'none\'.';
  }

  @override
  String get riskLoggingRedactOff => 'Sensitive Data Redaction Disabled';

  @override
  String get riskLoggingRedactOffDesc =>
      'Logging redaction is set to \'off\'. This may expose sensitive data in logs.';

  @override
  String get riskLogDirPermUnsafe => 'Log Directory Permission Unsafe';

  @override
  String get riskLogDirPermUnsafeDesc =>
      'Log directory permissions are unsafe, expected 700.';

  @override
  String get riskPlaintextSecrets => 'Plaintext Secrets Detected';

  @override
  String riskPlaintextSecretsDesc(String pattern) {
    return 'Potential plaintext secret found in config file (pattern: $pattern). Use environment variables or a secrets manager.';
  }

  @override
  String get riskOneClickRce =>
      '1-click RCE Remote Code Execution Vulnerability';

  @override
  String riskOneClickRceDesc(String version) {
    return 'OpenClaw has a critical 1-click RCE vulnerability (CVSS 10.0). Attackers can execute arbitrary code by tricking users into visiting malicious websites. Affected versions: < 2026.1.24-1, current version: $version. Upgrade to the latest version immediately.';
  }

  @override
  String get riskSkillsNotScanned => 'Skills Not Scanned for Prompt Injection';

  @override
  String riskSkillsNotScannedDesc(int count, String skills) {
    return '$count skill(s) have not been scanned for prompt injection risks: $skills. Click to scan.';
  }

  @override
  String riskSkillSecurityIssue(String skillName) {
    return 'Risky Skill: $skillName';
  }

  @override
  String riskSkillSecurityIssueDesc(String skillName, int issueCount) {
    return 'Skill \"$skillName\" has $issueCount security issue(s). Consider deleting this skill.';
  }

  @override
  String get riskLevelLow => 'Low';

  @override
  String get riskLevelMedium => 'Medium';

  @override
  String get riskLevelHigh => 'High';

  @override
  String get riskLevelCritical => 'Critical';

  @override
  String get detectedAssets => 'Detected Bot';

  @override
  String get assetName => 'Bot Name';

  @override
  String get assetType => 'Bot Type';

  @override
  String get version => 'Version';

  @override
  String get serviceName => 'Service Name';

  @override
  String get processPaths => 'Process Paths';

  @override
  String get metadata => 'Metadata';

  @override
  String get mitigate => 'Mitigate';

  @override
  String get fixApplied => 'Fix applied successfully';

  @override
  String get cancel => 'Cancel';

  @override
  String get aiModelConfig => 'AI Model Config';

  @override
  String get skillScanTitle => 'AI Skill Security Analysis';

  @override
  String get skillScanScanning => 'Scanning';

  @override
  String get skillScanCompleted => 'Scan completed';

  @override
  String get skillScanPreparing => 'Preparing...';

  @override
  String get skillScanConfigError => 'Please configure AI model first';

  @override
  String get skillScanAllSafe => 'All skills passed security check';

  @override
  String get skillScanRiskDetected => 'Risk detected';

  @override
  String get skillScanIssues => 'issue(s)';

  @override
  String get skillScanDelete => 'Delete';

  @override
  String get skillScanDeleted => 'Deleted';

  @override
  String get skillScanTrust => 'Trust';

  @override
  String get skillScanTrusted => 'Trusted';

  @override
  String get skillScanTrustTitle => 'Trust Skill';

  @override
  String skillScanTrustConfirm(String skillName) {
    return 'Trust \"$skillName\"? Risks from this skill will no longer appear on the main page.';
  }

  @override
  String get skillScanFailed => 'Scan failed - will retry on next scan';

  @override
  String get skillScanDeleteTitle => 'Delete Skill';

  @override
  String skillScanDeleteConfirm(String skillName) {
    return 'Are you sure you want to delete \"$skillName\"? This action cannot be undone.';
  }

  @override
  String get skillScanDone => 'Done';

  @override
  String skillScanFailedLoadConfig(String error) {
    return 'Failed to load config: $error';
  }

  @override
  String skillScanScanningSkill(String skillName) {
    return 'Scanning skill: $skillName';
  }

  @override
  String skillScanRiskDetectedLog(String summary) {
    return 'RISK DETECTED: $summary';
  }

  @override
  String get skillScanSkillSafe => 'Skill appears safe';

  @override
  String skillScanErrorScanning(String error) {
    return 'Error scanning skill: $error';
  }

  @override
  String get skillScanAnalysisComplete => '--- Analysis Complete ---';

  @override
  String get skillScanSafe => 'Safe';

  @override
  String get skillScanRiskLevel => 'Risk Level';

  @override
  String get skillScanSummary => 'Summary';

  @override
  String get skillScanIssueType => 'Type';

  @override
  String get skillScanIssueSeverity => 'Severity';

  @override
  String get skillScanIssueFile => 'File';

  @override
  String get skillScanIssueDesc => 'Description';

  @override
  String get skillScanIssueEvidence => 'Evidence';

  @override
  String get skillScanTypePromptInjection => 'Prompt Injection';

  @override
  String get skillScanTypeDataTheft => 'Data Theft';

  @override
  String get skillScanTypeCodeExecution => 'Code Execution';

  @override
  String get skillScanTypeSocialEngineering => 'Social Engineering';

  @override
  String get skillScanTypeSupplyChain => 'Supply Chain Attack';

  @override
  String get skillScanTypeOther => 'Other Risk';

  @override
  String get skillScanNoSkills => 'No skills found to scan';

  @override
  String get modelConfigTitle => 'Security Model';

  @override
  String get modelConfigProvider => 'Model Provider';

  @override
  String get modelConfigEndpoint => 'Endpoint';

  @override
  String get modelConfigEndpointId => 'Endpoint ID';

  @override
  String get modelConfigBaseUrl => 'Base URL';

  @override
  String get modelConfigBaseUrlOptional => 'Base URL (optional)';

  @override
  String get modelConfigApiKey => 'API Key';

  @override
  String get modelConfigAccessKey => 'Access Key';

  @override
  String get modelConfigSecretKey => 'Secret Key';

  @override
  String get modelConfigModelName => 'Model Name';

  @override
  String get modelConfigSave => 'Save';

  @override
  String get modelConfigFillRequired => 'Please fill in all required fields';

  @override
  String get modelConfigSaveFailed => 'Failed to save configuration';

  @override
  String get modelConfigRequired => 'Please configure AI model to continue';

  @override
  String get modelConfigTesting => 'Testing...';

  @override
  String get modelConfigSaving => 'Saving...';

  @override
  String modelConfigTestFailed(String error) {
    return 'Connection test failed: $error';
  }

  @override
  String get oneClickProtection => 'Enable Protection';

  @override
  String get protectionMonitor => 'Protection Monitor';

  @override
  String get protectionStarting => 'Starting...';

  @override
  String get launchAtStartup => 'Launch at Startup';

  @override
  String get auditLog => 'Audit Log';

  @override
  String get protectionConfirmTitle => 'Enable Protection';

  @override
  String get protectionConfirmMessage =>
      'Enabling protection will analyze agent behavior in real-time to ensure your device security';

  @override
  String get protectionConfirmButton => 'Confirm Enable';

  @override
  String get protectionMonitorTitle => 'Protection Monitor Center';

  @override
  String get protectionStatus => 'Protection Status';

  @override
  String get protectionActive => 'Active';

  @override
  String get protectionInactive => 'Inactive';

  @override
  String get behaviorAnalysis => 'Behavior Analysis';

  @override
  String get threatDetection => 'Threat Detection';

  @override
  String get realTimeMonitor => 'Real-time Monitor';

  @override
  String get noThreatsDetected => 'No threats detected';

  @override
  String get allSystemsNormal => 'All systems operating normally';

  @override
  String get proxyStarting => 'Starting protection proxy...';

  @override
  String get proxyStartingDesc => 'Reading config and starting proxy server';

  @override
  String get proxyStartFailed => 'Start failed';

  @override
  String get retry => 'Retry';

  @override
  String get analyzing => 'Analyzing...';

  @override
  String get analysisCount => 'Analysis Count';

  @override
  String get messageCountLabel => 'Message Count';

  @override
  String get warningCountLabel => 'Warning Count';

  @override
  String get blockedCount => 'Blocked Count';

  @override
  String get analysisLogs => 'Analysis Logs';

  @override
  String get clear => 'Clear';

  @override
  String get waitingLogs => 'Waiting for logs...';

  @override
  String get securityEvents => 'Security Events';

  @override
  String get noSecurityEvents => 'No security events';

  @override
  String get latestResult => 'Latest Result';

  @override
  String get maliciousDetected => 'Malicious instruction detected:';

  @override
  String get dartProxyStarting => '[Protection Proxy] Starting proxy...';

  @override
  String dartProxyStarted(int port, String provider) {
    return '[Protection Proxy] Started on port $port, provider: $provider';
  }

  @override
  String dartProxyFailed(String error) {
    return '[Protection Proxy] Failed: $error';
  }

  @override
  String dartProxyError(String error) {
    return '[Protection Proxy] Error: $error';
  }

  @override
  String get dartProxyStopping => '[Protection Proxy] Stopping proxy...';

  @override
  String get dartProxyStopped => '[Protection Proxy] Stopped';

  @override
  String get eventProxyStarting => 'Starting protection proxy';

  @override
  String eventProxyStarted(int port, String provider) {
    return 'Proxy started on port $port for provider $provider';
  }

  @override
  String eventProxyError(String error) {
    return 'Failed to start proxy: $error';
  }

  @override
  String eventProxyException(String error) {
    return 'Exception starting proxy: $error';
  }

  @override
  String get proxyNewRequest => '[Proxy] ========== NEW REQUEST ==========';

  @override
  String proxyRequestInfo(String model, int messageCount, String stream) {
    return '[Proxy] Request: model=$model, messages=$messageCount, stream=$stream';
  }

  @override
  String proxyMessageInfo(int index, String role, String content) {
    return '[Proxy] Message[$index] role=$role: $content';
  }

  @override
  String get proxyToolActivityDetected =>
      '[Proxy] ========== TOOL ACTIVITY DETECTED IN REQUEST ==========';

  @override
  String proxyToolCallsFound(int toolCount, int resultCount) {
    return '[Proxy] Found $toolCount tool calls, $resultCount tool results';
  }

  @override
  String get proxyResponseNonStream =>
      '[Proxy] ========== RESPONSE (non-stream) ==========';

  @override
  String proxyResponseInfo(String model, int choiceCount) {
    return '[Proxy] Response: model=$model, choices=$choiceCount';
  }

  @override
  String proxyResponseContent(String content) {
    return '[Proxy] Response content: $content';
  }

  @override
  String get proxyToolCallsDetected =>
      '[Proxy] ========== TOOL CALLS DETECTED ==========';

  @override
  String proxyToolCallCount(int count) {
    return '[Proxy] Tool calls count: $count';
  }

  @override
  String proxyToolCallName(int index, String name) {
    return '[Proxy] ToolCall[$index]: $name';
  }

  @override
  String proxyToolCallArgs(int index, String args) {
    return '[Proxy] ToolCall[$index] args: $args';
  }

  @override
  String get proxyStartingAnalysis =>
      '[Proxy] ========== STARTING ANALYSIS ==========';

  @override
  String proxyStreamFinished(String reason) {
    return '[Proxy] ========== STREAM FINISHED (reason=$reason) ==========';
  }

  @override
  String get proxyToolCallsInStream =>
      '[Proxy] ========== TOOL CALLS DETECTED IN STREAM ==========';

  @override
  String proxyStreamContentNoTools(String content) {
    return '[Proxy] Stream content (no tool calls): $content';
  }

  @override
  String get proxyAgentNotAvailable =>
      '[Proxy] Protection agent not available, allowing request';

  @override
  String get proxySendingAnalysis =>
      '[Proxy] Sending to protection agent for analysis...';

  @override
  String proxyOriginalTask(String task) {
    return '[Proxy] Original user task: $task';
  }

  @override
  String proxyMessageCountLog(int count) {
    return '[Proxy] Messages count: $count';
  }

  @override
  String proxyAnalyzeMessage(int index, String role, String content) {
    return '[Proxy] Analyze msg[$index] role=$role: $content';
  }

  @override
  String proxyAnalysisError(String error) {
    return '[Proxy] Analysis error: $error, allowing request';
  }

  @override
  String get proxyAnalysisResult =>
      '[Proxy] ========== ANALYSIS RESULT ==========';

  @override
  String proxyRiskLevel(String level) {
    return '[Proxy] Risk Level: $level';
  }

  @override
  String proxyConfidence(int confidence) {
    return '[Proxy] Confidence: $confidence%';
  }

  @override
  String proxySuggestedAction(String action) {
    return '[Proxy] Suggested Action: $action';
  }

  @override
  String proxyReason(String reason) {
    return '[Proxy] Reason: $reason';
  }

  @override
  String proxyMaliciousInstruction(String instruction) {
    return '[Proxy] Malicious Instruction: $instruction';
  }

  @override
  String proxyTraceableQuote(String quote) {
    return '[Proxy] Traceable Quote: $quote';
  }

  @override
  String get proxyBlocking => '[Proxy] *** BLOCKING REQUEST *** Risk detected!';

  @override
  String get proxyWarning =>
      '[Proxy] *** WARNING *** Potential risk, but allowing request';

  @override
  String get proxyAllowed => '[Proxy] *** ALLOWED *** Request is safe';

  @override
  String get proxyRestartingGateway => '[Proxy] Restarting openclaw gateway...';

  @override
  String proxyGatewayRestartError(String error) {
    return '[Proxy] Gateway restart error: $error';
  }

  @override
  String get proxyGatewayRestartSuccess =>
      '[Proxy] Gateway restarted successfully';

  @override
  String get proxyGatewayRestartSkippedAppstore =>
      '[Proxy] Gateway restart skipped (App Store build)';

  @override
  String proxyServerError(String error) {
    return '[Proxy] Server error: $error';
  }

  @override
  String proxyStarted(int port, String target, String provider) {
    return '[Proxy] Started on port $port, forwarding to $target (provider: $provider)';
  }

  @override
  String proxyConfigUpdateFailed(String error) {
    return '[Proxy] Warning: failed to update config: $error';
  }

  @override
  String proxyConfigUpdated(String provider, String url) {
    return '[Proxy] Updated $provider provider baseUrl to $url';
  }

  @override
  String get configUpdated => 'Configuration updated successfully';

  @override
  String proxyGatewayRestartFailed(String error) {
    return '[Proxy] Warning: failed to restart gateway: $error';
  }

  @override
  String get proxyStopping => '[Proxy] Stopping...';

  @override
  String proxyConfigRestoreFailed(String error) {
    return '[Proxy] Warning: failed to restore config: $error';
  }

  @override
  String proxyConfigRestored(String provider, String url) {
    return '[Proxy] Restored $provider provider baseUrl to $url';
  }

  @override
  String get proxyStopped => '[Proxy] Stopped';

  @override
  String protectionAgentAnalyzing(int count) {
    return '[Protection Agent] Analyzing $count messages...';
  }

  @override
  String get protectionAgentSendingLLM =>
      '[Protection Agent] Sending to LLM for analysis...';

  @override
  String protectionAgentError(String error) {
    return '[Protection Agent] Error: $error';
  }

  @override
  String protectionAgentRawResponse(String response) {
    return '[Protection Agent] Raw response: $response';
  }

  @override
  String protectionAgentWarning(String warning) {
    return '[Protection Agent] Warning: $warning';
  }

  @override
  String protectionAgentResult(String level, int confidence) {
    return '[Protection Agent] Risk Level: $level, Confidence: $confidence%';
  }

  @override
  String protectionAgentReason(String reason) {
    return '[Protection Agent] Reason: $reason';
  }

  @override
  String protectionAgentSuggestedAction(String action) {
    return '[Protection Agent] Suggested Action: $action';
  }

  @override
  String toolValidatorBlocked(String reason) {
    return '[Tool Validator] *** BLOCKED *** $reason';
  }

  @override
  String toolValidatorPassed(String toolName) {
    return '[Tool Validator] ✓ Passed: $toolName';
  }

  @override
  String dartAnalysisError(String error) {
    return '[Analysis] Error: $error';
  }

  @override
  String eventAnalysisError(String error) {
    return 'Analysis error: $error';
  }

  @override
  String get eventAnalysisCancelled => 'Analysis cancelled';

  @override
  String get eventProxyStopped => 'Protection proxy stopped';

  @override
  String get eventRequestBlocked => 'Request blocked';

  @override
  String get eventSecurityWarning => 'Security warning';

  @override
  String get eventRequestAllowed => 'Request allowed';

  @override
  String get eventAnalysisStarted => 'Analysis started';

  @override
  String get eventToolCallsDetected => 'Tool calls detected';

  @override
  String eventToolBlocked(String status, String reason) {
    return 'Tool call blocked [$status]: $reason';
  }

  @override
  String eventToolWarning(String status, String reason) {
    return 'Tool call warning [$status]: $reason';
  }

  @override
  String eventToolWarningAudit(String status, String reason) {
    return 'Tool call risk (Audit mode) [$status]: $reason';
  }

  @override
  String eventToolAllowed(String reason) {
    return 'Tool call allowed: $reason';
  }

  @override
  String eventToolBlockedWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return 'Tool call blocked <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolWarningWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return 'Tool call warning <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolWarningAuditWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return 'Tool call risk (Audit) <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolAllowedWithRisk(String riskTag, String reason) {
    return 'Tool call allowed <$riskTag>: $reason';
  }

  @override
  String eventQuotaExceeded(String limitType, int current, int limit) {
    return 'Quota exceeded [$limitType]: $current/$limit';
  }

  @override
  String eventServerError(String error) {
    return 'Server error: $error';
  }

  @override
  String get totalTokens => 'Total Tokens';

  @override
  String get promptTokens => 'Prompt Tokens';

  @override
  String get completionTokens => 'Completion Tokens';

  @override
  String get toolCallCount => 'Tool Calls';

  @override
  String get tokenTrend => 'Token Trend';

  @override
  String get toolCallTrend => 'Tool Call Trend';

  @override
  String get noDataYet => 'No data yet';

  @override
  String get analysisTokens => 'Analysis Total Tokens';

  @override
  String get analysisPromptTokens => 'Analysis Prompt Tokens';

  @override
  String get analysisCompletionTokens => 'Analysis Completion Tokens';

  @override
  String get analysisTokenTooltip =>
      'Token consumption from security analysis (not included in main business flow)';

  @override
  String get protectionConfigTitle => 'Protection Config';

  @override
  String get securityPromptTab => 'Smart Rules';

  @override
  String get tokenLimitTab => 'Token Limit';

  @override
  String get permissionTab => 'Perms';

  @override
  String get botModelTab => 'Bot Model';

  @override
  String get customSecurityPromptTitle => 'Custom Security Prompt';

  @override
  String get customSecurityPromptDesc =>
      'Configure security rules you care about, which will be prioritized during protection analysis';

  @override
  String get customSecurityPromptPlaceholder =>
      'Examples:\n- Deny access to /etc/passwd file\n- Block rm -rf commands\n- Block access to sensitive directory /home/user/.ssh/\n- Focus on database connection operations';

  @override
  String get customSecurityPromptTip =>
      'Your input will be wrapped in <USER_DEFINED></USER_DEFINED> tags and appended to the analysis system prompt. The model will prioritize your security rules during analysis.';

  @override
  String get tokenLimitTitle => 'Token Usage Limit';

  @override
  String get tokenLimitDesc =>
      'Configure token usage limits. The proxy will terminate the session when limits are exceeded';

  @override
  String get singleSessionTokenLimit => 'Single Session Token Limit';

  @override
  String get singleSessionTokenLimitPlaceholder =>
      'Leave empty or 0 for no limit';

  @override
  String get dailyTokenLimit => 'Daily Token Limit';

  @override
  String get dailyTokenLimitPlaceholder => 'Leave empty or 0 for no limit';

  @override
  String get tokenLimitTip =>
      'When token usage exceeds the configured limit, the proxy will return an over-limit error and terminate the current session to prevent excessive resource consumption.';

  @override
  String get tokenUnitK => 'K';

  @override
  String get tokenUnitM => 'M';

  @override
  String get tokenPresetLabel => 'Quick select';

  @override
  String get tokenNoLimit => 'No limit';

  @override
  String get tokenPreset50K => '50K';

  @override
  String get tokenPreset100K => '100K';

  @override
  String get tokenPreset300K => '300K';

  @override
  String get tokenPreset500K => '500K';

  @override
  String get tokenPreset1M => '1M';

  @override
  String get tokenPreset10M => '10M';

  @override
  String get tokenPreset50M => '50M';

  @override
  String get tokenPreset100M => '100M';

  @override
  String get pathPermissionTitle => 'Path Access Permission';

  @override
  String get pathPermissionDesc =>
      'Configure allowed or denied file paths for the proxied agent';

  @override
  String get pathPermissionPlaceholder => 'e.g.: /etc/passwd, /home/user/.ssh/';

  @override
  String get networkPermissionTitle => 'Network Access Permission';

  @override
  String get networkPermissionDesc =>
      'Configure allowed or denied network segments and domains';

  @override
  String get networkPermissionDescSandbox =>
      'Sandbox mode only supports * (all addresses) or localhost as host';

  @override
  String get networkPermissionPlaceholder =>
      'e.g.: 192.168.1.0/24, *.internal.com';

  @override
  String get networkPermissionPlaceholderSandbox =>
      'e.g.: *:*, localhost:8080, localhost:*';

  @override
  String get networkAddressInvalidForSandbox =>
      'Sandbox limitation: host must be * or localhost, specific IPs, CIDR and domains are not supported';

  @override
  String get networkOutboundTitle => 'Outbound';

  @override
  String get networkOutboundDesc =>
      'Control outgoing connections initiated by the process';

  @override
  String get networkInboundTitle => 'Inbound';

  @override
  String get networkInboundDesc =>
      'Control incoming connections to the process';

  @override
  String get shellPermissionTitle => 'Shell Command Permission';

  @override
  String get shellPermissionDesc =>
      'Configure allowed or denied shell commands';

  @override
  String get shellPermissionPlaceholder => 'e.g.: rm, chmod, sudo';

  @override
  String get blacklistMode => 'Blacklist';

  @override
  String get whitelistMode => 'Whitelist';

  @override
  String get permissionNote =>
      'Note: Permission configuration requires sandbox protection to be enabled. When enabled, the gateway process runs in a restricted environment.';

  @override
  String get shepherdRulesTab => 'User Rules';

  @override
  String get shepherdRulesTitle => 'User Defined Rules';

  @override
  String get shepherdRulesDesc =>
      'Configure tool call blacklist and sensitive actions for enhanced protection';

  @override
  String get shepherdBlacklistTitle => 'Tool Call Blacklist';

  @override
  String get shepherdBlacklistDesc =>
      'Forbidden tool names, e.g., delete_user, drop_table';

  @override
  String get shepherdBlacklistPlaceholder =>
      'Enter tool name, press Enter to add';

  @override
  String get shepherdSensitiveTitle => 'Need User Confirmation';

  @override
  String get shepherdSensitiveDesc =>
      'Define sensitive actions, e.g., delete, remove';

  @override
  String get shepherdSensitivePlaceholder =>
      'Enter keyword, press Enter to add';

  @override
  String get shepherdRulesTip =>
      'These rules directly apply to ShepherdGate protection logic and affect all requests.';

  @override
  String get securitySkillsTitle => 'Security Skills';

  @override
  String get securitySkillsDesc =>
      'Built-in security guard skills, automatically applied to tool call risk analysis';

  @override
  String get sandboxProtection => 'Sandbox Protection';

  @override
  String get sandboxProtectionDesc =>
      'restrict gateway process system resource access, enforcing permission configuration rules';

  @override
  String get saveConfig => 'Save Config';

  @override
  String get configSavedRestartRequired =>
      'Config saved, restart protection to apply new settings';

  @override
  String get restartNow => 'Restart Now';

  @override
  String get restarting => 'Restarting...';

  @override
  String get protectionConfigBtn => 'Config';

  @override
  String get auditLogTitle => 'Audit Log';

  @override
  String get auditLogTotal => 'Analyzed';

  @override
  String get auditLogRisk => 'Risk Count';

  @override
  String get auditLogBlocked => 'Blocked Count';

  @override
  String get auditLogWarned => 'Warned';

  @override
  String get auditLogAllowed => 'Allowed Count';

  @override
  String get auditLogSearchHint => 'Search logs...';

  @override
  String get auditLogRiskOnly => 'Risk Only';

  @override
  String get auditLogNoLogs => 'No audit logs';

  @override
  String get auditLogRefresh => 'Refresh';

  @override
  String get auditLogClearAll => 'Clear All';

  @override
  String get auditLogClearConfirmTitle => 'Clear All Logs';

  @override
  String get auditLogClearConfirmMessage =>
      'Are you sure you want to clear all audit logs? This action cannot be undone.';

  @override
  String get auditLogCancel => 'Cancel';

  @override
  String get auditLogClear => 'Clear';

  @override
  String get auditLogDetail => 'Log Detail';

  @override
  String get auditLogId => 'Log ID';

  @override
  String get auditLogTimestamp => 'Timestamp';

  @override
  String get auditLogRequestId => 'Request ID';

  @override
  String get auditLogModel => 'Model';

  @override
  String get auditLogAction => 'Action';

  @override
  String get auditLogRiskLevel => 'Risk Level';

  @override
  String get auditLogConfidence => 'Confidence';

  @override
  String get auditLogRiskReason => 'Risk Reason';

  @override
  String get auditLogDuration => 'Duration';

  @override
  String get auditLogTokens => 'Tokens';

  @override
  String get auditLogRequestContent => 'User Request';

  @override
  String get auditLogToolCalls => 'Tool Calls';

  @override
  String get auditLogOutputContent => 'Final Response';

  @override
  String get auditLogToolArguments => 'Arguments';

  @override
  String get auditLogToolResult => 'Result';

  @override
  String get auditLogSensitive => 'SENSITIVE';

  @override
  String auditLogPageInfo(int current, int total) {
    return 'Page $current of $total';
  }

  @override
  String get auditLogActionAllow => 'Allowed';

  @override
  String get auditLogActionWarn => 'Risk';

  @override
  String get auditLogActionBlock => 'Blocked';

  @override
  String get auditLogActionHardBlock => 'Hard Block';

  @override
  String get initializingProtectionMonitor => 'Initializing Protection Monitor';

  @override
  String get initDatabase => 'Initializing database';

  @override
  String get startCallbackBridge => 'Starting callback bridge';

  @override
  String get loadStatistics => 'Loading statistics';

  @override
  String get startListener => 'Starting listener service';

  @override
  String initFailed(String error) {
    return 'Initialization failed: $error';
  }

  @override
  String get configureAiModelFirst =>
      'Please configure AI model first (click the settings icon)';

  @override
  String get welcomeSlogan =>
      'Welcome to ClawdSecbot, guardian of your AI bots';

  @override
  String get authDeniedExit => 'Directory access denied. The app will exit.';

  @override
  String get onboardingScanReady =>
      'Onboarding complete. You can start scanning.';

  @override
  String get onboardingProtectionEnabled =>
      'Protection is enabled. Onboarding completed.';

  @override
  String get onboardingCongratsTitle => 'All set!';

  @override
  String get onboardingPersonalDone =>
      'Onboarding complete. Returning to main page...';

  @override
  String get onboardingTitle => 'Quick Start';

  @override
  String get onboardingStepModelTitle => 'Configure Proxy Model';

  @override
  String get onboardingStepModelDesc =>
      'Set up the model used by the proxy service';

  @override
  String get onboardingStepProxyTitle => 'Start Proxy Service';

  @override
  String get onboardingStepProxyDesc =>
      'Start the local proxy and create a listener';

  @override
  String get onboardingStepConnectivityTitle => 'Update Bot Config';

  @override
  String get onboardingStepConnectivityDesc =>
      'Update bot config and verify proxy connectivity';

  @override
  String get onboardingActionConfigureModel => 'Configure Proxy Model';

  @override
  String get onboardingActionStartProxy => 'Start Proxy';

  @override
  String get onboardingActionExecuteCommand => 'Run Command';

  @override
  String get onboardingActionCopyCommand => 'Copy Command';

  @override
  String get onboardingActionCopyPrompt => 'Copy Prompt';

  @override
  String get onboardingCommandResultTitle => 'Command output';

  @override
  String get onboardingCommandSuccess => 'Command completed successfully';

  @override
  String get onboardingActionBack => 'Back';

  @override
  String get onboardingActionNext => 'Next';

  @override
  String get onboardingActionSaveNext => 'Save and Next';

  @override
  String get onboardingActionFinish => 'Finish';

  @override
  String get onboardingActionSaveFinish => 'Save and Finish';

  @override
  String get onboardingActionEnterApp => 'Enter Main Page';

  @override
  String get onboardingWelcomeTitle => 'Welcome';

  @override
  String get onboardingWelcomeDesc =>
      'Complete the steps below to start using the security features.';

  @override
  String get onboardingQuickStartTitle => 'Core Features';

  @override
  String get onboardingQuickStartDesc =>
      'Welcome to Aglaugus, the guardian of the AI Bot era.';

  @override
  String get onboardingFeatureInjectTitle =>
      'Real-time Protection & Intent Detection';

  @override
  String get onboardingFeatureInjectDesc =>
      'Continuously monitors bot conversations and commands, identifies risks like injection, bypass, and malicious guidance; detects business intent deviation; automatically blocks/challenges/restricts high-risk requests to ensure operation within security boundaries.';

  @override
  String get onboardingFeaturePermissionTitle =>
      'Permission Configuration & Tool/Skill Management';

  @override
  String get onboardingFeaturePermissionDesc =>
      'Provides fine-grained permissions for bot tools/plugins/skills: available capabilities, scenarios, parameter limits; high-risk operations (such as sensitive data access, batch changes) require dedicated risk control and secondary confirmation to reduce misuse and abuse risks.';

  @override
  String get onboardingFeatureBaselineTitle =>
      'Protection Monitoring & Audit Traceability';

  @override
  String get onboardingFeatureBaselineDesc =>
      'Unified dashboard displays bot protection status, risk events, block/alert records; completely retains key conversations, tool calls, risk control decisions, supports post-event auditing, behavior tracing, and accountability.';

  @override
  String get onboardingSecurityModelTitle => 'Security Model Configuration';

  @override
  String get onboardingSecurityModelDesc =>
      'ClawdSecbot provides a local security proxy for your Bot interactions, focusing on external tool call risks. We do not collect or store personal data; all configuration and control stay with you.';

  @override
  String get onboardingBotModelTitle => 'Bot Model Registration';

  @override
  String get onboardingBotModelDesc =>
      'ClawdSecbot builds the security proxy using your registered Bot model info and securely forwards requests to the original model to ensure tasks complete.';

  @override
  String get onboardingConfigUpdateTitle => 'Bot Configuration Update';

  @override
  String get onboardingConfigUpdateDesc =>
      'Due to Apple software guidelines, you need to update the configuration to openclaw.json';

  @override
  String get onboardingConfigUpdateInstruction =>
      'Open the web configuration page, find Settings-Config-Models on the left sidebar';

  @override
  String get onboardingConfigUpdateComplete => 'Complete';

  @override
  String get onboardingReuseBotModel => 'Reuse Bot Model Configuration';

  @override
  String get onboardingReuseBotModelHint =>
      'Use the same model configuration as Bot for security detection';

  @override
  String get onboardingFinishTitle => 'All Set';

  @override
  String get onboardingFinishDesc =>
      'Basic setup is complete. Enter the main page to get started.';

  @override
  String get onboardingStatusConfigured => 'Configured';

  @override
  String get onboardingStatusPending => 'Pending';

  @override
  String get onboardingStatusProxyStarted => 'Proxy started';

  @override
  String get onboardingStatusProxyNotStarted => 'Proxy not started';

  @override
  String get onboardingStatusWaitingCallback => 'Waiting for callback';

  @override
  String get onboardingStatusCallbackReceived => 'Callback received';

  @override
  String get onboardingProxyServiceStatus => 'Proxy service status';

  @override
  String get onboardingProxyCallbackStatus => 'Callback status';

  @override
  String get onboardingProxyPortLabel => 'Proxy port';

  @override
  String get onboardingProxyPortInvalid => 'Invalid proxy port';

  @override
  String get onboardingProxyStarting => 'Starting';

  @override
  String onboardingProxyStartedMessage(int port) {
    return 'Proxy started on port $port';
  }

  @override
  String get onboardingProxyCommandLabel => 'Command';

  @override
  String get onboardingProxyCommandDesc => 'Update bot config';

  @override
  String get onboardingProxyPromptLabel => 'Prompt';

  @override
  String get onboardingProxyPromptDesc =>
      'Have OpenClaw execute the prompt and callback the model config to ClawdSecbot';

  @override
  String get onboardingConnectivityHint =>
      'After updating config, send a message in OpenClaw, e.g. \"hello\"';

  @override
  String get onboardingConnectivityStatus => 'Connectivity status';

  @override
  String get onboardingConnectivityWaiting => 'Waiting for message...';

  @override
  String get onboardingConnectivityDetected => 'Message detected';

  @override
  String proxyTokenUsage(
    int promptTokens,
    int completionTokens,
    int totalTokens,
  ) {
    return '[Proxy] Token usage: prompt=$promptTokens, completion=$completionTokens, total=$totalTokens';
  }

  @override
  String get configAccessTitle => 'Directory Access Required';

  @override
  String get configAccessMessage =>
      'To read/write protected folders such as Documents, Desktop, and Downloads, the app needs access to your Home directory (~).';

  @override
  String get configAccessPaths => 'Authorized access includes:';

  @override
  String get selectDirectory => 'Authorize Home Directory';

  @override
  String get clearData => 'Clear Data';

  @override
  String get clearDataConfirmTitle => 'Clear Data';

  @override
  String get clearDataConfirmMessage =>
      'Are you sure you want to clear all logs, token statistics and analysis data? This will not delete model configuration and protection configuration. This action cannot be undone.';

  @override
  String get clearDataSuccess => 'Data cleared successfully';

  @override
  String get clearDataFailed => 'Failed to clear data';

  @override
  String get auditOnlyMode => 'Audit Only Mode';

  @override
  String get auditOnlyModeDesc => 'No risk analysis, only record audit logs';

  @override
  String get auditOnlyModeShort => 'Audit Only';

  @override
  String get auditOnlyModePendingHint =>
      'Analysis in progress, changes will take effect after completion';

  @override
  String get appStoreGuideTitle => 'Configuration Guide';

  @override
  String get appStoreGuideDesc =>
      'Please copy the prompt below and run it in OpenClaw.';

  @override
  String get appStoreGuideCopied => 'Copied to clipboard';

  @override
  String get appStoreGuideCopy => 'Copy';

  @override
  String get appStoreGuideReceived => 'Configuration Received';

  @override
  String get appStoreGuideWaiting => 'Waiting for connection...';

  @override
  String get appStoreGuideProxyStarting => 'Starting proxy service...';

  @override
  String get appStoreGuideContinueHint =>
      'The app will automatically continue once configured.';

  @override
  String get scanStartTitle => 'Asset Scan';

  @override
  String get scanStartDesc => 'Ready to start scanning for assets.';

  @override
  String get scanStartAuthRequired => 'Authorization Required';

  @override
  String get scanStartAuthDesc =>
      'Access to the configuration directory is required to proceed.';

  @override
  String get scanStartAuthBtn => 'Authorize Home Directory';

  @override
  String get scanStartBtn => 'Start Asset Scan';

  @override
  String get newVersionAvailable => 'New Version Available';

  @override
  String versionAvailable(String version) {
    return 'Version $version is now available.';
  }

  @override
  String get download => 'Download';

  @override
  String get later => 'Later';

  @override
  String get restoreConfig => 'Restore Initial Config';

  @override
  String get restoreConfigConfirmTitle => 'Restore Initial Configuration';

  @override
  String get restoreConfigConfirmMessage =>
      'Are you sure you want to restore to initial configuration? This will:\n\n1. Stop current protection\n2. Restore openclaw.json to the state before first protection\n3. Restart openclaw gateway\n\nThis action cannot be undone.';

  @override
  String get restoreConfigSuccess => 'Configuration restored to initial state';

  @override
  String restoreConfigFailed(String error) {
    return 'Failed to restore configuration: $error';
  }

  @override
  String get restoreConfigNoBackup =>
      'Initial backup not found, cannot restore';

  @override
  String get restoreConfigDescription => 'Restore to state before first launch';

  @override
  String get restoringConfig => 'Restoring configuration...';

  @override
  String get generalSettings => 'General';

  @override
  String get dataManagement => 'Data Management';

  @override
  String get clearDataDescription => 'Clear logs, statistics and analysis data';

  @override
  String get permissionsSection => 'Permissions';

  @override
  String get dataSecurity => 'Data Security';

  @override
  String get dataExfiltrationRisk => 'Data Exfiltration Risk';

  @override
  String get sensitiveAccessRisk => 'Sensitive Access Risk';

  @override
  String get emailDeleteRisk => 'Email Delete Risk';

  @override
  String get promptInjectionRisk => 'Prompt Injection Risk';

  @override
  String get scriptExecutionRisk => 'Script Execution Risk';

  @override
  String get generalToolRisk => 'General Tool Risk';

  @override
  String skillAnalysis(String skillName) {
    return 'Analyzed by skill: $skillName';
  }

  @override
  String get skillNameDataExfiltrationGuard => 'Data Exfiltration Guard';

  @override
  String get skillNameFileAccessGuard => 'File Access Guard';

  @override
  String get skillNameEmailDeleteGuard => 'Email Delete Guard';

  @override
  String get skillNamePromptInjectionGuard => 'Prompt Injection Guard';

  @override
  String get skillNameScriptExecutionGuard => 'Script Execution Guard';

  @override
  String get skillNameGeneralToolRiskGuard => 'General Tool Risk Guard';

  @override
  String get securityEventDetail => 'Security Event Detail';

  @override
  String get eventBlocked => 'Blocked';

  @override
  String get eventToolExecution => 'Tool Execution';

  @override
  String get eventOther => 'Other Event';

  @override
  String get eventTime => 'Time';

  @override
  String get eventActionDesc => 'Action';

  @override
  String get eventRiskType => 'Risk Type';

  @override
  String get eventSource => 'Source';

  @override
  String get eventSourceAgent => 'AI Analysis Engine';

  @override
  String get eventSourceHeuristic => 'Heuristic Detection';

  @override
  String get eventType => 'Event Type';

  @override
  String get eventDetail => 'Detail';

  @override
  String get copyEventInfo => 'Copy Event Info';

  @override
  String get refresh => 'Refresh';

  @override
  String get clearAll => 'Clear All';

  @override
  String get viewSkillScanResults => 'Skill Scan Results';

  @override
  String get viewSkillScanResultsTitle => 'Skill Scan Results';

  @override
  String get noSkillScanResults => 'No skill scan results yet';

  @override
  String skillScanResultScannedAt(String time) {
    return 'Scanned: $time';
  }

  @override
  String skillScanResultIssueCount(int count) {
    return '$count issue(s)';
  }
}
