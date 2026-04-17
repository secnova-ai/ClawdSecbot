// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class AppLocalizationsZh extends AppLocalizations {
  AppLocalizationsZh([String locale = 'zh']) : super(locale);

  @override
  String get appTitle => 'ClawdSecbot';

  @override
  String get showWindow => '显示窗口';

  @override
  String get exit => '退出';

  @override
  String get idleTitle => 'ClawdSecbot 安全卫士';

  @override
  String get idleSubtitle => '扫描您的 Clawdbot 配置以查找安全风险';

  @override
  String get startScan => '开始安全扫描';

  @override
  String get scanning => '扫描中...';

  @override
  String get scanComplete => '扫描完成';

  @override
  String lastScanTime(String time) {
    return '上次检测: $time';
  }

  @override
  String get rescan => '重新扫描';

  @override
  String get rescanConfirmTitle => '确认重新扫描';

  @override
  String rescanConfirmMessage(int count) {
    return '当前有 $count 个Bot资产正在防护中。重新扫描将停止所有已开启的防护。是否继续？';
  }

  @override
  String get continueButton => '继续';

  @override
  String get checkingProtectionStatus => '检查防护状态中...';

  @override
  String get configuration => '配置信息';

  @override
  String get status => '状态';

  @override
  String get found => '已找到';

  @override
  String get notFound => '未找到';

  @override
  String get path => '路径';

  @override
  String get gatewayConfiguration => '网关配置';

  @override
  String get noGatewayConfig => '未找到网关配置';

  @override
  String get port => '端口';

  @override
  String get bind => '绑定地址';

  @override
  String get auth => '认证';

  @override
  String get controlUi => '控制界面';

  @override
  String get enabled => '已启用';

  @override
  String get disabled => '已禁用';

  @override
  String get securityFindings => '安全发现';

  @override
  String get noSecurityIssues => '未发现安全问题';

  @override
  String get secureConfigMessage => '您的 Clawdbot 配置看起来很安全！';

  @override
  String get testGoIntegration => '测试 Go 集成';

  @override
  String get goIntegrationTest => 'Go 集成测试';

  @override
  String get close => '关闭';

  @override
  String errorCallingGo(String error) {
    return '调用 Go 出错: $error';
  }

  @override
  String get settings => '全局设置';

  @override
  String get language => '语言';

  @override
  String get switchLanguage => '切换语言';

  @override
  String get menuHelp => '帮助';

  @override
  String aboutApp(String appName) {
    return '关于 $appName';
  }

  @override
  String get buildNumber => '构建号';

  @override
  String get currentPlatform => '平台';

  @override
  String aboutVersionWithBuild(String version, String build) {
    return '版本 $version ($build)';
  }

  @override
  String get aboutCopyright => 'Copyright © 2026 secnova.ai。保留所有权利。';

  @override
  String get riskNonLoopbackBinding => '非回环地址绑定';

  @override
  String riskNonLoopbackBindingDesc(String bind) {
    return '网关绑定到 \"$bind\"，这允许外部访问。建议仅绑定到 127.0.0.1。';
  }

  @override
  String get riskNoAuth => '未配置认证';

  @override
  String get riskNoAuthDesc => '网关未启用认证。任何拥有网络访问权限的人都可以连接。';

  @override
  String get riskWeakPassword => '认证密码太弱';

  @override
  String get riskWeakPasswordDesc => '密码长度小于 12 个字符。请使用更强的密码。';

  @override
  String get riskAllPluginsAllowed => '允许所有插件';

  @override
  String get riskAllPluginsAllowedDesc => '启用了通配符插件权限。这可能允许不受信任的代码执行。';

  @override
  String get riskControlUiEnabled => '控制界面已启用';

  @override
  String get riskControlUiEnabledDesc => 'Web 控制界面已启用。请确保其已正确加固。';

  @override
  String get riskRunningAsRoot => '以 root 身份运行';

  @override
  String get riskRunningAsRootDesc => '应用程序正在以 root 权限运行。这增加了攻击面。';

  @override
  String get riskConfigPermUnsafe => '配置文件权限不安全';

  @override
  String riskConfigPermUnsafeDesc(String path, String current) {
    return '配置文件权限为 $current，期望为 600。请运行 chmod 600 $path 修复。';
  }

  @override
  String get riskConfigDirPermUnsafe => '配置目录权限不安全';

  @override
  String riskConfigDirPermUnsafeDesc(String path, String current) {
    return '配置目录权限为 $current，期望为 700。请运行 chmod 700 $path 修复。';
  }

  @override
  String get riskSandboxDisabledDefault => '默认沙箱已禁用';

  @override
  String get riskSandboxDisabledDefaultDesc => '默认沙箱模式设置为 \'none\'。建议启用沙箱隔离。';

  @override
  String get riskSandboxDisabledAgent => 'Agent 沙箱已禁用';

  @override
  String riskSandboxDisabledAgentDesc(String agent) {
    return 'Agent \'$agent\' 的沙箱模式设置为 \'none\'.';
  }

  @override
  String get riskLoggingRedactOff => '敏感数据脱敏已禁用';

  @override
  String get riskLoggingRedactOffDesc => '日志脱敏设置为 \'off\'。可能导致敏感数据泄露到日志中。';

  @override
  String get riskLogDirPermUnsafe => '日志目录权限不安全';

  @override
  String get riskLogDirPermUnsafeDesc => '日志目录权限不安全，期望为 700。';

  @override
  String get riskPlaintextSecrets => '配置文件中发现明文密钥';

  @override
  String riskPlaintextSecretsDesc(String pattern) {
    return '在配置文件中发现潜在的明文密钥 (匹配模式: $pattern)。请使用环境变量或密钥管理工具。';
  }

  @override
  String get riskGatewayAuthPasswordMode => '网关启用了密码模式';

  @override
  String get riskGatewayAuthPasswordModeDesc =>
      '网关当前使用密码认证，相比 Token 模式更易被暴力破解。建议切换为 Token 模式。';

  @override
  String get riskGatewayWeakToken => '网关 Token 强度不足';

  @override
  String get riskGatewayWeakTokenDesc => '当前网关 Token 强度不足，建议立即轮换为高强度 Token。';

  @override
  String get riskAuditDisabled => '安全审计日志已禁用';

  @override
  String get riskAuditDisabledDesc => '安全审计日志处于关闭状态，关键高风险操作可能无法追溯。';

  @override
  String get riskAutonomyWorkspaceUnrestricted => '工作区访问范围未限制';

  @override
  String get riskAutonomyWorkspaceUnrestrictedDesc =>
      'Agent 未限制在工作区内访问文件，可能越界读取或写入非预期路径。';

  @override
  String get riskMemoryDirPermUnsafe => 'memory 目录权限不安全';

  @override
  String get riskMemoryDirPermUnsafeDesc =>
      'memory 目录权限过宽，可能导致运行时记忆数据泄露。建议收紧目录权限。';

  @override
  String get riskProcessRunningAsRoot => '进程以 root 身份运行';

  @override
  String get riskProcessRunningAsRootDesc =>
      '检测到进程以 root 身份运行，建议改为普通用户以降低高权限风险。';

  @override
  String get riskSkillAgentRisk => '检测到高风险 Skill';

  @override
  String get riskSkillAgentRiskDesc => '检测到高风险 Skill。若不可信，建议立即删除或禁用。';

  @override
  String get riskTerminalBackendLocal => '终端后端为本地执行';

  @override
  String get riskTerminalBackendLocalDesc =>
      'terminal.backend 为 local，Agent 操作将直接在宿主机执行，缺少远程隔离。';

  @override
  String get riskApprovalsModeDisabled => '审批模式已禁用';

  @override
  String riskApprovalsModeDisabledDesc(String mode) {
    return 'approvals.mode 为 \'$mode\'，高风险操作可能无需交互确认即可执行。';
  }

  @override
  String get riskRedactSecretsDisabled => '密钥脱敏已禁用';

  @override
  String get riskRedactSecretsDisabledDesc =>
      'security.redact_secrets 为 false，敏感凭据可能泄露到日志。';

  @override
  String get riskModelBaseUrlPublic => '自定义模型地址暴露公网';

  @override
  String riskModelBaseUrlPublicDesc(String baseUrl) {
    return 'model.base_url 指向非本地地址：$baseUrl。建议改为本地或受控私网地址。';
  }

  @override
  String get riskOneClickRce => '1-click RCE 远程代码执行漏洞';

  @override
  String riskOneClickRceDesc(String version) {
    return 'OpenClaw存在严重的1-click RCE漏洞（CVSS 10.0），攻击者可通过诱导用户访问恶意网站完成远程代码执行。受影响版本：< 2026.1.24-1，当前版本：$version。建议立即升级至最新版本。';
  }

  @override
  String get riskSkillsNotScanned => 'Skills 未进行提示词注入扫描';

  @override
  String riskSkillsNotScannedDesc(int count, String skills) {
    return '$count 个 Skill 尚未进行提示词注入风险扫描: $skills。点击执行扫描。';
  }

  @override
  String riskSkillSecurityIssue(String skillName) {
    return '风险技能: $skillName';
  }

  @override
  String riskSkillSecurityIssueDesc(String skillName, int issueCount) {
    return '技能 \"$skillName\" 存在 $issueCount 个安全问题。建议删除此技能。';
  }

  @override
  String get riskLevelLow => '低';

  @override
  String get riskLevelMedium => '中';

  @override
  String get riskLevelHigh => '高';

  @override
  String get riskLevelCritical => '严重';

  @override
  String get detectedAssets => '检测到的Bot';

  @override
  String get assetName => 'Bot名称';

  @override
  String get assetType => 'Bot类型';

  @override
  String get version => '版本';

  @override
  String get serviceName => '服务名称';

  @override
  String get processPaths => '进程路径';

  @override
  String get metadata => '元数据';

  @override
  String get mitigate => '修复';

  @override
  String get fixApplied => '修复成功';

  @override
  String get cancel => '取消';

  @override
  String get mitigationDialogTitle => '风险处置';

  @override
  String get mitigationExecute => '执行修复';

  @override
  String get mitigationConfirmAutoFix => '确定要执行自动修复吗？';

  @override
  String get mitigationFieldRequired => '此项必填';

  @override
  String mitigationFieldMinLength(int length) {
    return '最小长度为 $length';
  }

  @override
  String get mitigationFieldInvalidFormat => '格式不正确';

  @override
  String get mitigationFieldInvalidRegex => '无效的校验规则';

  @override
  String get mitigationUnsupportedFieldType => '不支持的字段类型';

  @override
  String get mitigationCommandCopied => '命令已复制到剪贴板';

  @override
  String get aiModelConfig => 'AI模型配置';

  @override
  String get skillScanTitle => 'AI 技能安全分析';

  @override
  String get skillScanScanning => '正在扫描';

  @override
  String get skillScanCompleted => '扫描完成';

  @override
  String get skillScanPreparing => '准备中...';

  @override
  String get skillScanConfigError => '请先配置 AI 模型';

  @override
  String get skillScanAllSafe => '所有技能通过安全检查';

  @override
  String get skillScanRiskDetected => '检测到风险';

  @override
  String get skillScanIssues => '个问题';

  @override
  String get skillScanDelete => '删除';

  @override
  String get skillScanDeleted => '已删除';

  @override
  String get skillScanTrust => '信任';

  @override
  String get skillScanTrusted => '已信任';

  @override
  String get skillScanTrustTitle => '信任技能';

  @override
  String skillScanTrustConfirm(String skillName) {
    return '确定信任 \"$skillName\" 吗？信任后该技能的风险将不再显示在主界面。';
  }

  @override
  String get skillScanFailed => '扫描失败 - 下次扫描时将重试';

  @override
  String get skillScanDeleteTitle => '删除技能';

  @override
  String skillScanDeleteConfirm(String skillName) {
    return '确定要删除 \"$skillName\" 吗？此操作不可撤销。';
  }

  @override
  String get skillScanDone => '完成';

  @override
  String skillScanFailedLoadConfig(String error) {
    return '加载配置失败: $error';
  }

  @override
  String skillScanScanningSkill(String skillName) {
    return '扫描技能: $skillName';
  }

  @override
  String skillScanRiskDetectedLog(String summary) {
    return '检测到风险: $summary';
  }

  @override
  String get skillScanSkillSafe => '技能安全';

  @override
  String skillScanErrorScanning(String error) {
    return '扫描技能出错: $error';
  }

  @override
  String get skillScanAnalysisComplete => '--- 分析完成 ---';

  @override
  String get skillScanSafe => '安全';

  @override
  String get skillScanRiskLevel => '风险等级';

  @override
  String get skillScanSummary => '分析摘要';

  @override
  String get skillScanIssueType => '类型';

  @override
  String get skillScanIssueSeverity => '严重程度';

  @override
  String get skillScanIssueFile => '文件';

  @override
  String get skillScanIssueDesc => '描述';

  @override
  String get skillScanIssueEvidence => '证据';

  @override
  String get skillScanTypePromptInjection => '提示词注入';

  @override
  String get skillScanTypeDataTheft => '数据窃取';

  @override
  String get skillScanTypeCodeExecution => '代码执行';

  @override
  String get skillScanTypeSocialEngineering => '社会工程';

  @override
  String get skillScanTypeSupplyChain => '供应链攻击';

  @override
  String get skillScanTypeOther => '其他风险';

  @override
  String get skillScanNoSkills => '没有发现需要扫描的技能';

  @override
  String get modelConfigTitle => '安全模型';

  @override
  String get modelConfigProvider => '模型供应商';

  @override
  String get modelConfigEndpoint => '端点地址';

  @override
  String get modelConfigEndpointId => '端点 ID';

  @override
  String get modelConfigBaseUrl => '基础 URL';

  @override
  String get modelConfigBaseUrlOptional => '基础 URL (可选)';

  @override
  String get modelConfigApiKey => 'API 密钥';

  @override
  String get modelConfigAccessKey => '访问密钥';

  @override
  String get modelConfigSecretKey => '密钥';

  @override
  String get modelConfigModelName => '模型名称';

  @override
  String get modelConfigSave => '保存';

  @override
  String get modelConfigFillRequired => '请填写所有必填字段';

  @override
  String get modelConfigSaveFailed => '保存配置失败';

  @override
  String get modelConfigRequired => '请先配置 AI 模型才能继续使用';

  @override
  String get modelConfigTesting => '测试连接中...';

  @override
  String get modelConfigSaving => '保存中...';

  @override
  String modelConfigTestFailed(String error) {
    return '连接测试失败: $error';
  }

  @override
  String get oneClickProtection => '一键防护';

  @override
  String get protectionAssetNotRunning => '资产未运行，无法开启防护';

  @override
  String get protectionMonitor => '防护监控';

  @override
  String get protectionStarting => '防护启动中...';

  @override
  String get launchAtStartup => '开机自启';

  @override
  String get auditLog => '审计日志';

  @override
  String get protectionConfirmTitle => '开启防护';

  @override
  String get protectionConfirmMessage => '开启防护会对智能体行为实时分析，保障您的设备安全';

  @override
  String get protectionConfirmButton => '确认开启';

  @override
  String get protectionMonitorTitle => '防护监控中心';

  @override
  String get protectionStatus => '防护状态';

  @override
  String get protectionActive => '防护中';

  @override
  String get protectionInactive => '未防护';

  @override
  String get behaviorAnalysis => '行为分析';

  @override
  String get threatDetection => '威胁检测';

  @override
  String get realTimeMonitor => '实时监控';

  @override
  String get noThreatsDetected => '暂无威胁检测';

  @override
  String get allSystemsNormal => '所有系统运行正常';

  @override
  String get proxyStarting => '正在启动保护代理...';

  @override
  String get proxyStartingDesc => '正在读取配置并启动代理服务器';

  @override
  String get proxyStartFailed => '启动失败';

  @override
  String get retry => '重试';

  @override
  String get analyzing => '分析中...';

  @override
  String get analysisCount => '分析次数';

  @override
  String get messageCountLabel => '消息数量';

  @override
  String get warningCountLabel => '警告次数';

  @override
  String get blockedCount => '拦截次数';

  @override
  String get analysisLogs => '分析日志';

  @override
  String get clear => '清空';

  @override
  String get waitingLogs => '等待分析日志...';

  @override
  String get securityEvents => '安全事件';

  @override
  String get noSecurityEvents => '暂无安全事件';

  @override
  String get latestResult => '最新分析结果';

  @override
  String get maliciousDetected => '检测到恶意指令:';

  @override
  String get dartProxyStarting => '[保护代理] 正在启动代理...';

  @override
  String dartProxyStarted(int port, String provider) {
    return '[保护代理] 已在端口 $port 启动, 提供者: $provider';
  }

  @override
  String dartProxyFailed(String error) {
    return '[保护代理] 启动失败: $error';
  }

  @override
  String dartProxyError(String error) {
    return '[保护代理] 错误: $error';
  }

  @override
  String get dartProxyStopping => '[保护代理] 正在停止代理...';

  @override
  String get dartProxyStopped => '[保护代理] 已停止';

  @override
  String get eventProxyStarting => '正在启动保护代理';

  @override
  String eventProxyStarted(int port, String provider) {
    return '代理已在端口 $port 启动, 提供者: $provider';
  }

  @override
  String eventProxyError(String error) {
    return '启动代理失败: $error';
  }

  @override
  String eventProxyException(String error) {
    return '启动代理异常: $error';
  }

  @override
  String get proxyNewRequest => '[代理] ========== 新请求 ==========';

  @override
  String proxyRequestInfo(String model, int messageCount, String stream) {
    return '[代理] 请求: 模型=$model, 消息数=$messageCount, 流式=$stream';
  }

  @override
  String proxyMessageInfo(int index, String role, String content) {
    return '[代理] 消息[$index] 角色=$role: $content';
  }

  @override
  String get proxyToolActivityDetected =>
      '[代理] ========== 请求中检测到工具活动 ==========';

  @override
  String proxyToolCallsFound(int toolCount, int resultCount) {
    return '[代理] 发现 $toolCount 个工具调用, $resultCount 个工具结果';
  }

  @override
  String get proxyResponseNonStream => '[代理] ========== 响应 (非流式) ==========';

  @override
  String proxyResponseInfo(String model, int choiceCount) {
    return '[代理] 响应: 模型=$model, 选项数=$choiceCount';
  }

  @override
  String proxyResponseContent(String content) {
    return '[代理] 响应内容: $content';
  }

  @override
  String get proxyToolCallsDetected => '[代理] ========== 检测到工具调用 ==========';

  @override
  String proxyToolCallCount(int count) {
    return '[代理] 工具调用数量: $count';
  }

  @override
  String proxyToolCallName(int index, String name) {
    return '[代理] 工具调用[$index]: $name';
  }

  @override
  String proxyToolCallArgs(int index, String args) {
    return '[代理] 工具调用[$index] 参数: $args';
  }

  @override
  String get proxyStartingAnalysis => '[代理] ========== 开始分析 ==========';

  @override
  String proxyStreamFinished(String reason) {
    return '[代理] ========== 流式结束 (原因=$reason) ==========';
  }

  @override
  String get proxyToolCallsInStream => '[代理] ========== 流式中检测到工具调用 ==========';

  @override
  String proxyStreamContentNoTools(String content) {
    return '[代理] 流式内容 (无工具调用): $content';
  }

  @override
  String get proxyAgentNotAvailable => '[代理] 防护代理不可用，允许请求';

  @override
  String get proxySendingAnalysis => '[代理] 发送到防护代理进行分析...';

  @override
  String proxyOriginalTask(String task) {
    return '[代理] 原始用户任务: $task';
  }

  @override
  String proxyMessageCountLog(int count) {
    return '[代理] 消息数量: $count';
  }

  @override
  String proxyAnalyzeMessage(int index, String role, String content) {
    return '[代理] 分析消息[$index] 角色=$role: $content';
  }

  @override
  String proxyAnalysisError(String error) {
    return '[代理] 分析错误: $error，允许请求';
  }

  @override
  String get proxyAnalysisResult => '[代理] ========== 分析结果 ==========';

  @override
  String proxyRiskLevel(String level) {
    return '[代理] 风险等级: $level';
  }

  @override
  String proxyConfidence(int confidence) {
    return '[代理] 置信度: $confidence%';
  }

  @override
  String proxySuggestedAction(String action) {
    return '[代理] 建议操作: $action';
  }

  @override
  String proxyReason(String reason) {
    return '[代理] 原因: $reason';
  }

  @override
  String proxyMaliciousInstruction(String instruction) {
    return '[代理] 恶意指令: $instruction';
  }

  @override
  String proxyTraceableQuote(String quote) {
    return '[代理] 可追溯引用: $quote';
  }

  @override
  String get proxyBlocking => '[代理] *** 拦截请求 *** 检测到风险!';

  @override
  String get proxyWarning => '[代理] *** 警告 *** 存在潜在风险，但允许请求';

  @override
  String get proxyAllowed => '[代理] *** 允许 *** 请求安全';

  @override
  String get proxyRestartingGateway => '[代理] 正在重启 openclaw 网关...';

  @override
  String proxyGatewayRestartError(String error) {
    return '[代理] 网关重启错误: $error';
  }

  @override
  String get proxyGatewayRestartSuccess => '[代理] 网关重启成功';

  @override
  String get proxyGatewayRestartSkippedAppstore => '[代理] 跳过网关重启 (App Store 版本)';

  @override
  String proxyServerError(String error) {
    return '[代理] 服务器错误: $error';
  }

  @override
  String proxyStarted(int port, String target, String provider) {
    return '[代理] 已在端口 $port 启动, 转发至 $target (提供商: $provider)';
  }

  @override
  String proxyConfigUpdateFailed(String error) {
    return '[代理] 警告: 配置更新失败: $error';
  }

  @override
  String proxyConfigUpdated(String provider, String url) {
    return '[代理] 已更新 $provider 提供商 baseUrl 为 $url';
  }

  @override
  String get configUpdated => '配置更新成功';

  @override
  String proxyGatewayRestartFailed(String error) {
    return '[代理] 警告: 网关重启失败: $error';
  }

  @override
  String get proxyStopping => '[代理] 正在停止...';

  @override
  String proxyConfigRestoreFailed(String error) {
    return '[代理] 警告: 配置恢复失败: $error';
  }

  @override
  String proxyConfigRestored(String provider, String url) {
    return '[代理] 已恢复 $provider 提供商 baseUrl 为 $url';
  }

  @override
  String get proxyStopped => '[代理] 已停止';

  @override
  String protectionAgentAnalyzing(int count) {
    return '[防护代理] 正在分析 $count 条消息...';
  }

  @override
  String get protectionAgentSendingLLM => '[防护代理] 发送到 LLM 进行分析...';

  @override
  String protectionAgentError(String error) {
    return '[防护代理] 错误: $error';
  }

  @override
  String protectionAgentRawResponse(String response) {
    return '[防护代理] 原始响应: $response';
  }

  @override
  String protectionAgentWarning(String warning) {
    return '[防护代理] 警告: $warning';
  }

  @override
  String protectionAgentResult(String level, int confidence) {
    return '[防护代理] 风险等级: $level, 置信度: $confidence%';
  }

  @override
  String protectionAgentReason(String reason) {
    return '[防护代理] 原因: $reason';
  }

  @override
  String protectionAgentSuggestedAction(String action) {
    return '[防护代理] 建议操作: $action';
  }

  @override
  String toolValidatorBlocked(String reason) {
    return '[工具验证] *** 拦截 *** $reason';
  }

  @override
  String toolValidatorPassed(String toolName) {
    return '[工具验证] ✓ 通过: $toolName';
  }

  @override
  String dartAnalysisError(String error) {
    return '[分析] 错误: $error';
  }

  @override
  String eventAnalysisError(String error) {
    return '分析错误: $error';
  }

  @override
  String get eventAnalysisCancelled => '分析已取消';

  @override
  String get eventProxyStopped => '保护代理已停止';

  @override
  String get eventRequestBlocked => '请求已拦截';

  @override
  String get eventSecurityWarning => '安全警告';

  @override
  String get eventRequestAllowed => '请求已允许';

  @override
  String get eventAnalysisStarted => '开始分析';

  @override
  String get eventToolCallsDetected => '检测到工具调用';

  @override
  String eventToolBlocked(String status, String reason) {
    return '工具调用已拦截 [$status]: $reason';
  }

  @override
  String eventToolWarning(String status, String reason) {
    return '工具调用警告 [$status]: $reason';
  }

  @override
  String eventToolWarningAudit(String status, String reason) {
    return '工具调用风险(审计模式) [$status]: $reason';
  }

  @override
  String eventToolAllowed(String reason) {
    return '工具调用已放行: $reason';
  }

  @override
  String eventToolBlockedWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return '工具调用已拦截 <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolWarningWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return '工具调用警告 <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolWarningAuditWithRisk(
    String riskTag,
    String status,
    String reason,
  ) {
    return '工具调用风险(审计) <$riskTag> [$status]: $reason';
  }

  @override
  String eventToolAllowedWithRisk(String riskTag, String reason) {
    return '工具调用已放行 <$riskTag>: $reason';
  }

  @override
  String eventQuotaExceeded(String limitType, int current, int limit) {
    return '配额超限 [$limitType]: $current/$limit';
  }

  @override
  String eventServerError(String error) {
    return '服务器错误: $error';
  }

  @override
  String get totalTokens => 'Token总量';

  @override
  String get promptTokens => '输入Token';

  @override
  String get completionTokens => '输出Token';

  @override
  String get toolCallCount => '工具调用';

  @override
  String get tokenTrend => 'Token消耗趋势';

  @override
  String get toolCallTrend => '工具调用趋势';

  @override
  String get noDataYet => '暂无数据';

  @override
  String get analysisTokens => '防护Token总量';

  @override
  String get analysisPromptTokens => '防护输入Token';

  @override
  String get analysisCompletionTokens => '防护输出Token';

  @override
  String get analysisTokenTooltip => '此部分为安全防护分析产生的Token消耗，不计入主要业务流程';

  @override
  String get protectionConfigTitle => '防护配置';

  @override
  String get securityPromptTab => '智能规则';

  @override
  String get tokenLimitTab => 'Token限制';

  @override
  String get permissionTab => '权限设置';

  @override
  String get botModelTab => 'Bot模型';

  @override
  String get customSecurityPromptTitle => '自定义安全提示词';

  @override
  String get customSecurityPromptDesc => '配置您关注的安全规则，防护分析时将优先考虑这些规则';

  @override
  String get customSecurityPromptPlaceholder =>
      '例如：\n- 禁止访问 /etc/passwd 文件\n- 禁止执行 rm -rf 命令\n- 禁止访问敏感目录 /home/user/.ssh/\n- 重点关注数据库连接相关操作';

  @override
  String get customSecurityPromptTip =>
      '您输入的内容将被包裹在 <USER_DEFINED></USER_DEFINED> 标签中，追加到分析系统提示词后面，模型分析时会以您关注的安全规则为主。';

  @override
  String get tokenLimitTitle => 'Token使用限制';

  @override
  String get tokenLimitDesc => '配置Token使用上限，超过限制时代理将终止会话';

  @override
  String get singleSessionTokenLimit => '单轮会话Token上限';

  @override
  String get singleSessionTokenLimitPlaceholder => '留空或0表示不限制';

  @override
  String get dailyTokenLimit => '当日总Token上限';

  @override
  String get dailyTokenLimitPlaceholder => '留空或0表示不限制';

  @override
  String get tokenLimitTip => '当Token使用超过设定上限时，代理会返回超限错误并终止当前会话，防止过度消耗资源。';

  @override
  String get tokenUnitK => '千';

  @override
  String get tokenUnitM => '百万';

  @override
  String get tokenPresetLabel => '快捷选择';

  @override
  String get tokenNoLimit => '不限制';

  @override
  String get tokenPreset50K => '5万';

  @override
  String get tokenPreset100K => '10万';

  @override
  String get tokenPreset300K => '30万';

  @override
  String get tokenPreset500K => '50万';

  @override
  String get tokenPreset1M => '100万';

  @override
  String get tokenPreset10M => '1000万';

  @override
  String get tokenPreset50M => '5000万';

  @override
  String get tokenPreset100M => '1亿';

  @override
  String get pathPermissionTitle => '路径访问权限';

  @override
  String get pathPermissionDesc => '配置允许或禁止被代理的智能体访问的文件路径';

  @override
  String get pathPermissionPlaceholder => '例如: /etc/passwd, /home/user/.ssh/';

  @override
  String get networkPermissionTitle => '网络访问权限';

  @override
  String get networkPermissionDesc => '配置允许或禁止访问的网段、域名';

  @override
  String get networkPermissionDescSandbox => '沙箱模式下仅支持 *（所有地址）或 localhost 作为主机';

  @override
  String get networkPermissionPlaceholder =>
      '例如: 192.168.1.0/24, *.internal.com';

  @override
  String get networkPermissionPlaceholderSandbox =>
      '例如: *:*, localhost:8080, localhost:*';

  @override
  String get networkAddressInvalidForSandbox =>
      '沙箱模式限制：主机只能为 * 或 localhost，不支持具体IP地址、CIDR或域名';

  @override
  String get networkOutboundTitle => '出栈 (Outbound)';

  @override
  String get networkOutboundDesc => '控制进程主动发起的对外连接';

  @override
  String get networkInboundTitle => '入栈 (Inbound)';

  @override
  String get networkInboundDesc => '控制外部对进程发起的连接';

  @override
  String get shellPermissionTitle => 'Shell命令权限';

  @override
  String get shellPermissionDesc => '配置允许或禁止执行的Shell命令';

  @override
  String get shellPermissionPlaceholder => '例如: rm, chmod, sudo';

  @override
  String get blacklistMode => '黑名单';

  @override
  String get whitelistMode => '白名单';

  @override
  String get permissionNote => '注意：权限设置功能需要启用沙箱防护才能生效。启用后，网关进程将在受限环境中运行。';

  @override
  String get shepherdRulesTab => '用户规则';

  @override
  String get shepherdRulesTitle => '用户自定义规则';

  @override
  String get shepherdRulesDesc => '配置工具调用黑名单和敏感操作，用于增强防护';

  @override
  String get shepherdBlacklistTitle => '工具调用黑名单';

  @override
  String get shepherdBlacklistDesc => '禁止调用的工具名称，例如：delete_user, drop_table';

  @override
  String get shepherdBlacklistPlaceholder => '输入工具名称，回车添加';

  @override
  String get shepherdSensitiveTitle => '需要用户确认';

  @override
  String get shepherdSensitiveDesc => '定义哪些操作属于敏感操作，例如：delete, remove';

  @override
  String get shepherdSensitivePlaceholder => '输入敏感操作关键词，回车添加';

  @override
  String get shepherdRulesTip => '这些规则将直接应用于 ShepherdGate 防护逻辑，对所有请求生效。';

  @override
  String get securitySkillsTitle => '安全技能';

  @override
  String get securitySkillsDesc => '系统内置的安全防护技能，自动应用于工具调用风险分析';

  @override
  String get sandboxProtection => '沙箱防护';

  @override
  String get sandboxProtectionDesc => '限制网关进程的系统资源访问，强制执行权限设置规则';

  @override
  String get saveConfig => '保存配置';

  @override
  String get configSavedRestartRequired => '配置已保存，需要重启防护以应用新配置';

  @override
  String get restartNow => '立即重启';

  @override
  String get restarting => '正在重启...';

  @override
  String get protectionConfigBtn => '防护配置';

  @override
  String get auditLogTitle => '审计日志';

  @override
  String get auditLogTotal => '分析次数';

  @override
  String get auditLogRisk => '风险次数';

  @override
  String get auditLogBlocked => '拦截次数';

  @override
  String get auditLogWarned => '警告';

  @override
  String get auditLogAllowed => '允许次数';

  @override
  String get auditLogSearchHint => '搜索请求、回复、风险说明与消息/工具 JSON...';

  @override
  String get auditLogSearchTooltip =>
      '在请求正文、模型输出、风险说明以及 messages、tool_calls 的 JSON 原文中做子串匹配（不搜请求 ID 等结构化字段）。';

  @override
  String get auditLogRiskOnly => '仅显示风险';

  @override
  String get auditLogNoLogs => '暂无审计日志';

  @override
  String get auditLogRefresh => '刷新';

  @override
  String get auditLogClearAll => '清空全部';

  @override
  String get auditLogClearConfirmTitle => '清空所有日志';

  @override
  String get auditLogClearConfirmMessage => '确定要清空所有审计日志吗？此操作无法撤销。';

  @override
  String get auditLogCancel => '取消';

  @override
  String get auditLogClear => '清空';

  @override
  String get auditLogDetail => '日志详情';

  @override
  String get auditLogId => '日志 ID';

  @override
  String get auditLogTimestamp => '时间';

  @override
  String get auditLogRequestId => '请求 ID';

  @override
  String get auditLogModel => '模型';

  @override
  String get auditLogAction => '动作';

  @override
  String get auditLogRiskLevel => '风险等级';

  @override
  String get auditLogConfidence => '置信度';

  @override
  String get auditLogRiskReason => '风险原因';

  @override
  String get auditLogDuration => '耗时';

  @override
  String get auditLogTokens => 'Token数';

  @override
  String get auditLogRequestContent => '用户请求';

  @override
  String get auditLogToolCalls => '工具调用';

  @override
  String get auditLogOutputContent => '最终响应内容';

  @override
  String get auditLogToolArguments => '参数';

  @override
  String get auditLogToolResult => '结果';

  @override
  String get auditLogSensitive => '敏感';

  @override
  String get auditLogExport => '导出';

  @override
  String get auditLogExportFailed => '导出失败';

  @override
  String auditLogPageInfo(int current, int total) {
    return '第 $current 页，共 $total 页';
  }

  @override
  String auditLogEntryTotal(int count) {
    return '共 $count 条';
  }

  @override
  String auditLogEntryRange(int start, int end) {
    return '第 $start-$end 条';
  }

  @override
  String auditLogItemSerial(int n) {
    return '第 $n 条';
  }

  @override
  String get auditLogSearchSubmitHint => '按回车搜索';

  @override
  String get auditLogActionAllow => '已允许';

  @override
  String get auditLogActionWarn => '有风险';

  @override
  String get auditLogActionBlock => '已拦截';

  @override
  String get auditLogActionHardBlock => '强制拦截';

  @override
  String get initializingProtectionMonitor => '正在初始化防护监控';

  @override
  String get initDatabase => '初始化数据库';

  @override
  String get startCallbackBridge => '启动通信桥接';

  @override
  String get loadStatistics => '加载统计数据';

  @override
  String get startListener => '启动监听服务';

  @override
  String initFailed(String error) {
    return '初始化失败: $error';
  }

  @override
  String get configureAiModelFirst => '请先配置 AI 模型（点击左下角设置图标）';

  @override
  String get welcomeSlogan => '欢迎使用 ClawdSecbot, 专注于 AI Bot 的安全防护';

  @override
  String get authDeniedExit => '未授权目录访问，应用将退出';

  @override
  String get onboardingScanReady => '引导已完成，可以开始扫描';

  @override
  String get onboardingProtectionEnabled => '已完成首次引导';

  @override
  String get onboardingCongratsTitle => '恭喜';

  @override
  String get onboardingPersonalDone => '引导完成，正在返回主页面...';

  @override
  String get onboardingTitle => '快速开始';

  @override
  String get onboardingStepModelTitle => '配置代理模型';

  @override
  String get onboardingStepModelDesc => '配置用于代理服务的模型';

  @override
  String get onboardingStepProxyTitle => '启动代理服务';

  @override
  String get onboardingStepProxyDesc => '启动本地代理并创建监听';

  @override
  String get onboardingStepConnectivityTitle => '修改Bot配置';

  @override
  String get onboardingStepConnectivityDesc => '更新Bot配置并验证代理联通性';

  @override
  String get onboardingActionConfigureModel => '配置代理模型';

  @override
  String get onboardingActionStartProxy => '启动Proxy';

  @override
  String get onboardingActionExecuteCommand => '执行命令';

  @override
  String get onboardingActionCopyCommand => '复制命令';

  @override
  String get onboardingActionCopyPrompt => '复制提示词';

  @override
  String get onboardingCommandResultTitle => '执行结果';

  @override
  String get onboardingCommandSuccess => '命令执行成功';

  @override
  String get onboardingActionBack => '上一步';

  @override
  String get onboardingActionNext => '下一步';

  @override
  String get onboardingActionSaveNext => '保存并下一步';

  @override
  String get onboardingActionFinish => '完成引导';

  @override
  String get onboardingActionSaveFinish => '保存并完成';

  @override
  String get onboardingActionEnterApp => '进入主页面';

  @override
  String get onboardingWelcomeTitle => '欢迎使用';

  @override
  String get onboardingWelcomeDesc => '完成以下引导后即可开始使用安全防护能力。';

  @override
  String get onboardingQuickStartTitle => '核心功能一览';

  @override
  String get onboardingQuickStartDesc => 'ClawdSecbot —— AI Bot 时代的守护神';

  @override
  String get onboardingFeatureInjectTitle => '实时防护与意图偏离检测';

  @override
  String get onboardingFeatureInjectDesc =>
      '持续监听 Bot 对话与指令,识别注入、绕过、恶意引导等风险;判断业务意图偏离;高危请求自动拦截/质询/降权,确保安全边界内运行。';

  @override
  String get onboardingFeaturePermissionTitle => '权限设置与工具/技能管控';

  @override
  String get onboardingFeaturePermissionDesc =>
      '为 Bot 工具/插件/技能提供细粒度权限:可用能力、场景、参数上限;高危操作(如敏感数据访问、批量变更)设专属风控与二次确认,降低误用滥用风险。';

  @override
  String get onboardingFeatureBaselineTitle => '防护监控与审计追溯';

  @override
  String get onboardingFeatureBaselineDesc =>
      '统一面板展示 Bot 防护状态、风险事件、拦截/告警记录;完整留存关键对话、工具调用、风控决策,支持事后审计、行为追溯与追责。';

  @override
  String get onboardingSecurityModelTitle => '安全模型配置';

  @override
  String get onboardingSecurityModelDesc =>
      'ClawdSecbot 会在本机为您的 Bot 交互提供安全代理,重点分析外部工具调用风险.我们不会收集或保存您的个人隐私数据,所有配置与控制权都在您手中.';

  @override
  String get onboardingBotModelTitle => 'Bot模型登记';

  @override
  String get onboardingBotModelDesc =>
      'ClawdSecbot 会通过您登记的 Bot 模型信息来建立安全代理,并将请求安全转发至原始模型,确保各项任务正常完成.';

  @override
  String get onboardingConfigUpdateTitle => 'Bot配置更新';

  @override
  String get onboardingConfigUpdateDesc =>
      '由于 Apple 的系统安全规范限制, ClawdSecbot 无法直接修改其他应用的配置文件. 请按照提示手动更新 openclaw.json 使配置更新.';

  @override
  String get onboardingConfigUpdateInstruction =>
      '打开web页面配置框，找到左侧Settings-Config-Models';

  @override
  String get onboardingConfigUpdateComplete => '完成';

  @override
  String get onboardingReuseBotModel => '复用Bot模型配置';

  @override
  String get onboardingReuseBotModelHint => '使用与Bot相同的模型配置作为安全检测模型';

  @override
  String get onboardingFinishTitle => '恭喜完成';

  @override
  String get onboardingFinishDesc => '基础配置已完成，进入主页面开始使用。';

  @override
  String get onboardingStatusConfigured => '已完成配置';

  @override
  String get onboardingStatusPending => '待完成';

  @override
  String get onboardingStatusProxyStarted => '代理已启动';

  @override
  String get onboardingStatusProxyNotStarted => '代理未启动';

  @override
  String get onboardingStatusWaitingCallback => '等待回调确认';

  @override
  String get onboardingStatusCallbackReceived => '已收到回调';

  @override
  String get onboardingProxyServiceStatus => '代理服务状态';

  @override
  String get onboardingProxyCallbackStatus => '回调状态';

  @override
  String get onboardingProxyPortLabel => '代理端口';

  @override
  String get onboardingProxyPortInvalid => '代理端口无效';

  @override
  String get onboardingProxyStarting => '启动中';

  @override
  String onboardingProxyStartedMessage(int port) {
    return '代理服务已启动，监听端口: $port';
  }

  @override
  String get onboardingProxyCommandLabel => '执行命令';

  @override
  String get onboardingProxyCommandDesc => '更新Bot配置';

  @override
  String get onboardingProxyPromptLabel => '提示词';

  @override
  String get onboardingProxyPromptDesc => '让OpenClaw执行提示词并把模型配置回调给 ClawdSecbot';

  @override
  String get onboardingConnectivityHint =>
      '更新配置后，在OpenClaw中发送一条消息，例如：\"hello\"';

  @override
  String get onboardingConnectivityStatus => '联通性状态';

  @override
  String get onboardingConnectivityWaiting => '等待检测消息...';

  @override
  String get onboardingConnectivityDetected => '已检测到消息';

  @override
  String proxyTokenUsage(
    int promptTokens,
    int completionTokens,
    int totalTokens,
  ) {
    return '[代理] Token使用: 输入=$promptTokens, 输出=$completionTokens, 总计=$totalTokens';
  }

  @override
  String get configAccessTitle => '需要目录访问权限';

  @override
  String get configAccessMessage =>
      '为了读取/写入 Documents、Desktop、Downloads 等受保护文件夹，应用需要授权访问您的主目录 (~).';

  @override
  String get configAccessPaths => '授权后可访问目录：';

  @override
  String get selectDirectory => '授权主目录';

  @override
  String get clearData => '清空数据';

  @override
  String get clearDataConfirmTitle => '清空数据';

  @override
  String get clearDataConfirmMessage =>
      '确定要清空所有日志、Token统计和分析数据吗？此操作不会删除模型配置和防护配置等运行时数据。此操作无法撤销。';

  @override
  String get clearDataSuccess => '数据已清空';

  @override
  String get clearDataFailed => '清空数据失败';

  @override
  String get auditOnlyMode => '仅审计模式';

  @override
  String get auditOnlyModeDesc => '不进行风险研判，仅记录审计日志';

  @override
  String get auditOnlyModeShort => '仅审计';

  @override
  String get auditOnlyModePendingHint => '正在研判中，变更将在研判结束后生效';

  @override
  String get appStoreGuideTitle => '配置引导';

  @override
  String get appStoreGuideDesc => '请复制以下提示词并在 OpenClaw 中执行。';

  @override
  String get appStoreGuideCopied => '已复制到剪贴板';

  @override
  String get appStoreGuideCopy => '复制';

  @override
  String get appStoreGuideReceived => '配置已接收';

  @override
  String get appStoreGuideWaiting => '等待连接...';

  @override
  String get appStoreGuideProxyStarting => '代理服务启动中，请稍候...';

  @override
  String get appStoreGuideContinueHint => '配置完成后应用将自动继续。';

  @override
  String get scanStartTitle => '资产扫描';

  @override
  String get scanStartDesc => '准备开始扫描资产。';

  @override
  String get scanStartAuthRequired => '需要授权';

  @override
  String get scanStartAuthDesc => '需要访问配置目录的权限才能继续。';

  @override
  String get scanStartAuthBtn => '授权主目录';

  @override
  String get scanStartBtn => '开始资产扫描';

  @override
  String get newVersionAvailable => '发现新版本';

  @override
  String versionAvailable(String version) {
    return '版本 $version 现已发布。';
  }

  @override
  String get download => '下载';

  @override
  String get later => '稍后';

  @override
  String get restoreConfig => '恢复初始配置';

  @override
  String get restoreConfigConfirmTitle => '恢复初始配置';

  @override
  String get restoreConfigConfirmMessage =>
      '确定要恢复到初始配置吗？这将:\n\n1. 停止当前防护\n2. 恢复 openclaw.json 到首次启动防护前的状态\n3. 重启 openclaw 网关\n\n此操作无法撤销。';

  @override
  String get exitRestoreTitle => '退出前恢复 Bot 服务';

  @override
  String exitRestoreMessage(int count) {
    return '当前有 $count 个 Bot 资产仍在使用 ClawdSecbot 代理流量。退出后如果不恢复默认配置，这些 Bot 可能无法继续工作。\n\n确认后，ClawdSecbot 会停止代理、防护失效，并尽量恢复 Bot 自身的未防护服务。';
  }

  @override
  String get exitRestoreConfirm => '恢复并退出';

  @override
  String get exitRestoreInProgress => '正在恢复 Bot 默认服务，请稍候...';

  @override
  String get exitRestoreFailedTitle => '恢复失败，已取消退出';

  @override
  String exitRestoreFailedMessage(String details) {
    return '以下资产恢复失败，为避免 Bot 退出后无法继续工作，本次不会退出应用：\n\n$details';
  }

  @override
  String get restoreConfigSuccess => '配置已恢复到初始状态';

  @override
  String restoreConfigFailed(String error) {
    return '恢复配置失败: $error';
  }

  @override
  String get restoreConfigNoBackup => '初始备份不存在，无法恢复';

  @override
  String get restoreConfigDescription => '恢复到首次启动前的状态';

  @override
  String get restoringConfig => '正在恢复配置...';

  @override
  String get generalSettings => '通用设置';

  @override
  String get scheduledScanSetting => '定时扫描设置';

  @override
  String get scheduledScanDescription => '按设定间隔自动执行安全扫描';

  @override
  String get scheduledScanOff => '关闭';

  @override
  String get scheduledScanCustom => '自定义';

  @override
  String get scheduledScanCustomHint => '请输入正整数并选择时间单位';

  @override
  String get scheduledScanCustomValueHint => '输入数值';

  @override
  String get scheduledScanInvalidCustomValue => '请输入大于 0 的数字';

  @override
  String get scheduledScanOption60Seconds => '60秒';

  @override
  String get scheduledScanOption5Minutes => '5分钟';

  @override
  String get scheduledScanOption1Hour => '1小时';

  @override
  String get scheduledScanUnitSeconds => '秒';

  @override
  String get scheduledScanUnitMinutes => '分钟';

  @override
  String get scheduledScanUnitHours => '小时';

  @override
  String scheduledScanEvery(int value, String unit) {
    return '每 $value $unit';
  }

  @override
  String get dataManagement => '数据管理';

  @override
  String get clearDataDescription => '清除日志、统计和分析数据';

  @override
  String get permissionsSection => '权限';

  @override
  String get dataSecurity => '数据安全';

  @override
  String get dataExfiltrationRisk => '数据外泄风险';

  @override
  String get sensitiveAccessRisk => '敏感访问风险';

  @override
  String get emailDeleteRisk => '邮件删除风险';

  @override
  String get promptInjectionRisk => '提示注入风险';

  @override
  String get scriptExecutionRisk => '脚本执行风险';

  @override
  String get generalToolRisk => '通用工具风险';

  @override
  String skillAnalysis(String skillName) {
    return '基于智能技能 $skillName 分析';
  }

  @override
  String get skillNameDataExfiltrationGuard => '数据泄露防护';

  @override
  String get skillNameFileAccessGuard => '文件访问防护';

  @override
  String get skillNameEmailDeleteGuard => '邮件删除防护';

  @override
  String get skillNamePromptInjectionGuard => '提示注入防护';

  @override
  String get skillNameScriptExecutionGuard => '脚本执行防护';

  @override
  String get skillNameGeneralToolRiskGuard => '通用工具风险防护';

  @override
  String get securityEventDetail => '安全事件详情';

  @override
  String get eventBlocked => '已拦截';

  @override
  String get eventToolExecution => '工具执行';

  @override
  String get eventOther => '其他事件';

  @override
  String get eventTime => '时间';

  @override
  String get eventActionDesc => '动作描述';

  @override
  String get eventRiskType => '风险类型';

  @override
  String get eventSource => '来源';

  @override
  String get eventSourceAgent => 'AI 分析引擎';

  @override
  String get eventSourceHeuristic => '启发式检测';

  @override
  String get eventType => '事件类型';

  @override
  String get eventDetail => '详细信息';

  @override
  String get copyEventInfo => '复制事件信息';

  @override
  String get refresh => '刷新';

  @override
  String get clearAll => '清空';

  @override
  String get viewSkillScanResults => '技能检测历史';

  @override
  String get viewSkillScanResultsTitle => '技能检测历史';

  @override
  String get rescanSecurityDiscovery => '安全发现';

  @override
  String get rescanAll => '所有信息';

  @override
  String get deleteRiskSkill => '删除技能';

  @override
  String deleteRiskSkillConfirm(String skill) {
    return '确认删除技能 \"$skill\" 吗？';
  }

  @override
  String get deleteRiskSkillSuccess => '技能删除成功';

  @override
  String get deleteRiskSkillAlreadyMissing => '技能目录已不存在，按已删除处理';

  @override
  String get deleteRiskSkillFailed => '技能删除失败';

  @override
  String get deleteRiskSkillUnavailable => '缺少技能路径或哈希，无法删除';

  @override
  String get noSkillScanResults => '暂无技能扫描记录';

  @override
  String skillScanResultScannedAt(String time) {
    return '扫描时间: $time';
  }

  @override
  String skillScanResultIssueCount(int count) {
    return '$count 个问题';
  }
}
