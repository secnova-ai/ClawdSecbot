import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations? of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations);
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('zh'),
  ];

  /// No description provided for @appTitle.
  ///
  /// In zh, this message translates to:
  /// **'ClawdSecbot'**
  String get appTitle;

  /// No description provided for @showWindow.
  ///
  /// In zh, this message translates to:
  /// **'显示窗口'**
  String get showWindow;

  /// No description provided for @exit.
  ///
  /// In zh, this message translates to:
  /// **'退出'**
  String get exit;

  /// No description provided for @idleTitle.
  ///
  /// In zh, this message translates to:
  /// **'ClawdSecbot 安全卫士'**
  String get idleTitle;

  /// No description provided for @idleSubtitle.
  ///
  /// In zh, this message translates to:
  /// **'扫描您的 Clawdbot 配置以查找安全风险'**
  String get idleSubtitle;

  /// No description provided for @startScan.
  ///
  /// In zh, this message translates to:
  /// **'开始安全扫描'**
  String get startScan;

  /// No description provided for @scanning.
  ///
  /// In zh, this message translates to:
  /// **'扫描中...'**
  String get scanning;

  /// No description provided for @scanComplete.
  ///
  /// In zh, this message translates to:
  /// **'扫描完成'**
  String get scanComplete;

  /// No description provided for @lastScanTime.
  ///
  /// In zh, this message translates to:
  /// **'上次检测: {time}'**
  String lastScanTime(String time);

  /// No description provided for @rescan.
  ///
  /// In zh, this message translates to:
  /// **'重新扫描'**
  String get rescan;

  /// No description provided for @rescanConfirmTitle.
  ///
  /// In zh, this message translates to:
  /// **'确认重新扫描'**
  String get rescanConfirmTitle;

  /// No description provided for @rescanConfirmMessage.
  ///
  /// In zh, this message translates to:
  /// **'当前有 {count} 个Bot资产正在防护中。重新扫描将停止所有已开启的防护。是否继续？'**
  String rescanConfirmMessage(int count);

  /// No description provided for @continueButton.
  ///
  /// In zh, this message translates to:
  /// **'继续'**
  String get continueButton;

  /// No description provided for @checkingProtectionStatus.
  ///
  /// In zh, this message translates to:
  /// **'检查防护状态中...'**
  String get checkingProtectionStatus;

  /// No description provided for @configuration.
  ///
  /// In zh, this message translates to:
  /// **'配置信息'**
  String get configuration;

  /// No description provided for @status.
  ///
  /// In zh, this message translates to:
  /// **'状态'**
  String get status;

  /// No description provided for @found.
  ///
  /// In zh, this message translates to:
  /// **'已找到'**
  String get found;

  /// No description provided for @notFound.
  ///
  /// In zh, this message translates to:
  /// **'未找到'**
  String get notFound;

  /// No description provided for @path.
  ///
  /// In zh, this message translates to:
  /// **'路径'**
  String get path;

  /// No description provided for @gatewayConfiguration.
  ///
  /// In zh, this message translates to:
  /// **'网关配置'**
  String get gatewayConfiguration;

  /// No description provided for @noGatewayConfig.
  ///
  /// In zh, this message translates to:
  /// **'未找到网关配置'**
  String get noGatewayConfig;

  /// No description provided for @port.
  ///
  /// In zh, this message translates to:
  /// **'端口'**
  String get port;

  /// No description provided for @bind.
  ///
  /// In zh, this message translates to:
  /// **'绑定地址'**
  String get bind;

  /// No description provided for @auth.
  ///
  /// In zh, this message translates to:
  /// **'认证'**
  String get auth;

  /// No description provided for @controlUi.
  ///
  /// In zh, this message translates to:
  /// **'控制界面'**
  String get controlUi;

  /// No description provided for @enabled.
  ///
  /// In zh, this message translates to:
  /// **'已启用'**
  String get enabled;

  /// No description provided for @disabled.
  ///
  /// In zh, this message translates to:
  /// **'已禁用'**
  String get disabled;

  /// No description provided for @securityFindings.
  ///
  /// In zh, this message translates to:
  /// **'安全发现'**
  String get securityFindings;

  /// No description provided for @noSecurityIssues.
  ///
  /// In zh, this message translates to:
  /// **'未发现安全问题'**
  String get noSecurityIssues;

  /// No description provided for @secureConfigMessage.
  ///
  /// In zh, this message translates to:
  /// **'您的 Clawdbot 配置看起来很安全！'**
  String get secureConfigMessage;

  /// No description provided for @testGoIntegration.
  ///
  /// In zh, this message translates to:
  /// **'测试 Go 集成'**
  String get testGoIntegration;

  /// No description provided for @goIntegrationTest.
  ///
  /// In zh, this message translates to:
  /// **'Go 集成测试'**
  String get goIntegrationTest;

  /// No description provided for @close.
  ///
  /// In zh, this message translates to:
  /// **'关闭'**
  String get close;

  /// No description provided for @errorCallingGo.
  ///
  /// In zh, this message translates to:
  /// **'调用 Go 出错: {error}'**
  String errorCallingGo(String error);

  /// No description provided for @settings.
  ///
  /// In zh, this message translates to:
  /// **'全局设置'**
  String get settings;

  /// No description provided for @language.
  ///
  /// In zh, this message translates to:
  /// **'语言'**
  String get language;

  /// No description provided for @switchLanguage.
  ///
  /// In zh, this message translates to:
  /// **'切换语言'**
  String get switchLanguage;

  /// No description provided for @menuHelp.
  ///
  /// In zh, this message translates to:
  /// **'帮助'**
  String get menuHelp;

  /// No description provided for @aboutApp.
  ///
  /// In zh, this message translates to:
  /// **'关于 {appName}'**
  String aboutApp(String appName);

  /// No description provided for @buildNumber.
  ///
  /// In zh, this message translates to:
  /// **'构建号'**
  String get buildNumber;

  /// No description provided for @currentPlatform.
  ///
  /// In zh, this message translates to:
  /// **'平台'**
  String get currentPlatform;

  /// No description provided for @aboutVersionWithBuild.
  ///
  /// In zh, this message translates to:
  /// **'版本 {version} ({build})'**
  String aboutVersionWithBuild(String version, String build);

  /// No description provided for @aboutCopyright.
  ///
  /// In zh, this message translates to:
  /// **'Copyright © 2026 secnova.ai。保留所有权利。'**
  String get aboutCopyright;

  /// No description provided for @riskNonLoopbackBinding.
  ///
  /// In zh, this message translates to:
  /// **'非回环地址绑定'**
  String get riskNonLoopbackBinding;

  /// No description provided for @riskNonLoopbackBindingDesc.
  ///
  /// In zh, this message translates to:
  /// **'网关绑定到 \"{bind}\"，这允许外部访问。建议仅绑定到 127.0.0.1。'**
  String riskNonLoopbackBindingDesc(String bind);

  /// No description provided for @riskNoAuth.
  ///
  /// In zh, this message translates to:
  /// **'未配置认证'**
  String get riskNoAuth;

  /// No description provided for @riskNoAuthDesc.
  ///
  /// In zh, this message translates to:
  /// **'网关未启用认证。任何拥有网络访问权限的人都可以连接。'**
  String get riskNoAuthDesc;

  /// No description provided for @riskWeakPassword.
  ///
  /// In zh, this message translates to:
  /// **'认证密码太弱'**
  String get riskWeakPassword;

  /// No description provided for @riskWeakPasswordDesc.
  ///
  /// In zh, this message translates to:
  /// **'密码长度小于 12 个字符。请使用更强的密码。'**
  String get riskWeakPasswordDesc;

  /// No description provided for @riskAllPluginsAllowed.
  ///
  /// In zh, this message translates to:
  /// **'允许所有插件'**
  String get riskAllPluginsAllowed;

  /// No description provided for @riskAllPluginsAllowedDesc.
  ///
  /// In zh, this message translates to:
  /// **'启用了通配符插件权限。这可能允许不受信任的代码执行。'**
  String get riskAllPluginsAllowedDesc;

  /// No description provided for @riskControlUiEnabled.
  ///
  /// In zh, this message translates to:
  /// **'控制界面已启用'**
  String get riskControlUiEnabled;

  /// No description provided for @riskControlUiEnabledDesc.
  ///
  /// In zh, this message translates to:
  /// **'Web 控制界面已启用。请确保其已正确加固。'**
  String get riskControlUiEnabledDesc;

  /// No description provided for @riskRunningAsRoot.
  ///
  /// In zh, this message translates to:
  /// **'以 root 身份运行'**
  String get riskRunningAsRoot;

  /// No description provided for @riskRunningAsRootDesc.
  ///
  /// In zh, this message translates to:
  /// **'应用程序正在以 root 权限运行。这增加了攻击面。'**
  String get riskRunningAsRootDesc;

  /// No description provided for @riskConfigPermUnsafe.
  ///
  /// In zh, this message translates to:
  /// **'配置文件权限不安全'**
  String get riskConfigPermUnsafe;

  /// No description provided for @riskConfigPermUnsafeDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置文件权限为 {current}，期望为 600。请运行 chmod 600 {path} 修复。'**
  String riskConfigPermUnsafeDesc(String path, String current);

  /// No description provided for @riskConfigDirPermUnsafe.
  ///
  /// In zh, this message translates to:
  /// **'配置目录权限不安全'**
  String get riskConfigDirPermUnsafe;

  /// No description provided for @riskConfigDirPermUnsafeDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置目录权限为 {current}，期望为 700。请运行 chmod 700 {path} 修复。'**
  String riskConfigDirPermUnsafeDesc(String path, String current);

  /// No description provided for @riskSandboxDisabledDefault.
  ///
  /// In zh, this message translates to:
  /// **'默认沙箱已禁用'**
  String get riskSandboxDisabledDefault;

  /// No description provided for @riskSandboxDisabledDefaultDesc.
  ///
  /// In zh, this message translates to:
  /// **'默认沙箱模式设置为 \'none\'。建议启用沙箱隔离。'**
  String get riskSandboxDisabledDefaultDesc;

  /// No description provided for @riskSandboxDisabledAgent.
  ///
  /// In zh, this message translates to:
  /// **'Agent 沙箱已禁用'**
  String get riskSandboxDisabledAgent;

  /// No description provided for @riskSandboxDisabledAgentDesc.
  ///
  /// In zh, this message translates to:
  /// **'Agent \'{agent}\' 的沙箱模式设置为 \'none\'.'**
  String riskSandboxDisabledAgentDesc(String agent);

  /// No description provided for @riskLoggingRedactOff.
  ///
  /// In zh, this message translates to:
  /// **'敏感数据脱敏已禁用'**
  String get riskLoggingRedactOff;

  /// No description provided for @riskLoggingRedactOffDesc.
  ///
  /// In zh, this message translates to:
  /// **'日志脱敏设置为 \'off\'。可能导致敏感数据泄露到日志中。'**
  String get riskLoggingRedactOffDesc;

  /// No description provided for @riskLogDirPermUnsafe.
  ///
  /// In zh, this message translates to:
  /// **'日志目录权限不安全'**
  String get riskLogDirPermUnsafe;

  /// No description provided for @riskLogDirPermUnsafeDesc.
  ///
  /// In zh, this message translates to:
  /// **'日志目录权限不安全，期望为 700。'**
  String get riskLogDirPermUnsafeDesc;

  /// No description provided for @riskPlaintextSecrets.
  ///
  /// In zh, this message translates to:
  /// **'配置文件中发现明文密钥'**
  String get riskPlaintextSecrets;

  /// No description provided for @riskPlaintextSecretsDesc.
  ///
  /// In zh, this message translates to:
  /// **'在配置文件中发现潜在的明文密钥 (匹配模式: {pattern})。请使用环境变量或密钥管理工具。'**
  String riskPlaintextSecretsDesc(String pattern);

  /// No description provided for @riskGatewayAuthPasswordMode.
  ///
  /// In zh, this message translates to:
  /// **'网关启用了密码模式'**
  String get riskGatewayAuthPasswordMode;

  /// No description provided for @riskGatewayAuthPasswordModeDesc.
  ///
  /// In zh, this message translates to:
  /// **'网关当前使用密码认证，相比 Token 模式更易被暴力破解。建议切换为 Token 模式。'**
  String get riskGatewayAuthPasswordModeDesc;

  /// No description provided for @riskGatewayWeakToken.
  ///
  /// In zh, this message translates to:
  /// **'网关 Token 强度不足'**
  String get riskGatewayWeakToken;

  /// No description provided for @riskGatewayWeakTokenDesc.
  ///
  /// In zh, this message translates to:
  /// **'当前网关 Token 强度不足，建议立即轮换为高强度 Token。'**
  String get riskGatewayWeakTokenDesc;

  /// No description provided for @riskAuditDisabled.
  ///
  /// In zh, this message translates to:
  /// **'安全审计日志已禁用'**
  String get riskAuditDisabled;

  /// No description provided for @riskAuditDisabledDesc.
  ///
  /// In zh, this message translates to:
  /// **'安全审计日志处于关闭状态，关键高风险操作可能无法追溯。'**
  String get riskAuditDisabledDesc;

  /// No description provided for @riskAutonomyWorkspaceUnrestricted.
  ///
  /// In zh, this message translates to:
  /// **'工作区访问范围未限制'**
  String get riskAutonomyWorkspaceUnrestricted;

  /// No description provided for @riskAutonomyWorkspaceUnrestrictedDesc.
  ///
  /// In zh, this message translates to:
  /// **'Agent 未限制在工作区内访问文件，可能越界读取或写入非预期路径。'**
  String get riskAutonomyWorkspaceUnrestrictedDesc;

  /// No description provided for @riskMemoryDirPermUnsafe.
  ///
  /// In zh, this message translates to:
  /// **'memory 目录权限不安全'**
  String get riskMemoryDirPermUnsafe;

  /// No description provided for @riskMemoryDirPermUnsafeDesc.
  ///
  /// In zh, this message translates to:
  /// **'memory 目录权限过宽，可能导致运行时记忆数据泄露。建议收紧目录权限。'**
  String get riskMemoryDirPermUnsafeDesc;

  /// No description provided for @riskProcessRunningAsRoot.
  ///
  /// In zh, this message translates to:
  /// **'进程以 root 身份运行'**
  String get riskProcessRunningAsRoot;

  /// No description provided for @riskProcessRunningAsRootDesc.
  ///
  /// In zh, this message translates to:
  /// **'检测到进程以 root 身份运行，建议改为普通用户以降低高权限风险。'**
  String get riskProcessRunningAsRootDesc;

  /// No description provided for @riskSkillAgentRisk.
  ///
  /// In zh, this message translates to:
  /// **'检测到高风险 Skill'**
  String get riskSkillAgentRisk;

  /// No description provided for @riskSkillAgentRiskDesc.
  ///
  /// In zh, this message translates to:
  /// **'检测到高风险 Skill。若不可信，建议立即删除或禁用。'**
  String get riskSkillAgentRiskDesc;

  /// No description provided for @riskTerminalBackendLocal.
  ///
  /// In zh, this message translates to:
  /// **'终端后端为本地执行'**
  String get riskTerminalBackendLocal;

  /// No description provided for @riskTerminalBackendLocalDesc.
  ///
  /// In zh, this message translates to:
  /// **'terminal.backend 为 local，Agent 操作将直接在宿主机执行，缺少远程隔离。'**
  String get riskTerminalBackendLocalDesc;

  /// No description provided for @riskApprovalsModeDisabled.
  ///
  /// In zh, this message translates to:
  /// **'审批模式已禁用'**
  String get riskApprovalsModeDisabled;

  /// No description provided for @riskApprovalsModeDisabledDesc.
  ///
  /// In zh, this message translates to:
  /// **'approvals.mode 为 \'{mode}\'，高风险操作可能无需交互确认即可执行。'**
  String riskApprovalsModeDisabledDesc(String mode);

  /// No description provided for @riskRedactSecretsDisabled.
  ///
  /// In zh, this message translates to:
  /// **'密钥脱敏已禁用'**
  String get riskRedactSecretsDisabled;

  /// No description provided for @riskRedactSecretsDisabledDesc.
  ///
  /// In zh, this message translates to:
  /// **'security.redact_secrets 为 false，敏感凭据可能泄露到日志。'**
  String get riskRedactSecretsDisabledDesc;

  /// No description provided for @riskModelBaseUrlPublic.
  ///
  /// In zh, this message translates to:
  /// **'自定义模型地址暴露公网'**
  String get riskModelBaseUrlPublic;

  /// No description provided for @riskModelBaseUrlPublicDesc.
  ///
  /// In zh, this message translates to:
  /// **'model.base_url 指向非本地地址：{baseUrl}。建议改为本地或受控私网地址。'**
  String riskModelBaseUrlPublicDesc(String baseUrl);

  /// No description provided for @riskOneClickRce.
  ///
  /// In zh, this message translates to:
  /// **'1-click RCE 远程代码执行漏洞'**
  String get riskOneClickRce;

  /// No description provided for @riskOneClickRceDesc.
  ///
  /// In zh, this message translates to:
  /// **'OpenClaw存在严重的1-click RCE漏洞（CVSS 10.0），攻击者可通过诱导用户访问恶意网站完成远程代码执行。受影响版本：< 2026.1.24-1，当前版本：{version}。建议立即升级至最新版本。'**
  String riskOneClickRceDesc(String version);

  /// No description provided for @riskSkillsNotScanned.
  ///
  /// In zh, this message translates to:
  /// **'Skills 未进行提示词注入扫描'**
  String get riskSkillsNotScanned;

  /// No description provided for @riskSkillsNotScannedDesc.
  ///
  /// In zh, this message translates to:
  /// **'{count} 个 Skill 尚未进行提示词注入风险扫描: {skills}。点击执行扫描。'**
  String riskSkillsNotScannedDesc(int count, String skills);

  /// No description provided for @riskSkillSecurityIssue.
  ///
  /// In zh, this message translates to:
  /// **'风险技能: {skillName}'**
  String riskSkillSecurityIssue(String skillName);

  /// No description provided for @riskSkillSecurityIssueDesc.
  ///
  /// In zh, this message translates to:
  /// **'技能 \"{skillName}\" 存在 {issueCount} 个安全问题。建议删除此技能。'**
  String riskSkillSecurityIssueDesc(String skillName, int issueCount);

  /// No description provided for @riskLevelLow.
  ///
  /// In zh, this message translates to:
  /// **'低'**
  String get riskLevelLow;

  /// No description provided for @riskLevelMedium.
  ///
  /// In zh, this message translates to:
  /// **'中'**
  String get riskLevelMedium;

  /// No description provided for @riskLevelHigh.
  ///
  /// In zh, this message translates to:
  /// **'高'**
  String get riskLevelHigh;

  /// No description provided for @riskLevelCritical.
  ///
  /// In zh, this message translates to:
  /// **'严重'**
  String get riskLevelCritical;

  /// No description provided for @detectedAssets.
  ///
  /// In zh, this message translates to:
  /// **'检测到的Bot'**
  String get detectedAssets;

  /// No description provided for @assetName.
  ///
  /// In zh, this message translates to:
  /// **'Bot名称'**
  String get assetName;

  /// No description provided for @assetType.
  ///
  /// In zh, this message translates to:
  /// **'Bot类型'**
  String get assetType;

  /// No description provided for @version.
  ///
  /// In zh, this message translates to:
  /// **'版本'**
  String get version;

  /// No description provided for @serviceName.
  ///
  /// In zh, this message translates to:
  /// **'服务名称'**
  String get serviceName;

  /// No description provided for @processPaths.
  ///
  /// In zh, this message translates to:
  /// **'进程路径'**
  String get processPaths;

  /// No description provided for @metadata.
  ///
  /// In zh, this message translates to:
  /// **'元数据'**
  String get metadata;

  /// No description provided for @mitigate.
  ///
  /// In zh, this message translates to:
  /// **'修复'**
  String get mitigate;

  /// No description provided for @fixApplied.
  ///
  /// In zh, this message translates to:
  /// **'修复成功'**
  String get fixApplied;

  /// No description provided for @cancel.
  ///
  /// In zh, this message translates to:
  /// **'取消'**
  String get cancel;

  /// No description provided for @mitigationDialogTitle.
  ///
  /// In zh, this message translates to:
  /// **'风险处置'**
  String get mitigationDialogTitle;

  /// No description provided for @mitigationExecute.
  ///
  /// In zh, this message translates to:
  /// **'执行修复'**
  String get mitigationExecute;

  /// No description provided for @mitigationConfirmAutoFix.
  ///
  /// In zh, this message translates to:
  /// **'确定要执行自动修复吗？'**
  String get mitigationConfirmAutoFix;

  /// No description provided for @mitigationFieldRequired.
  ///
  /// In zh, this message translates to:
  /// **'此项必填'**
  String get mitigationFieldRequired;

  /// No description provided for @mitigationFieldMinLength.
  ///
  /// In zh, this message translates to:
  /// **'最小长度为 {length}'**
  String mitigationFieldMinLength(int length);

  /// No description provided for @mitigationFieldInvalidFormat.
  ///
  /// In zh, this message translates to:
  /// **'格式不正确'**
  String get mitigationFieldInvalidFormat;

  /// No description provided for @mitigationFieldInvalidRegex.
  ///
  /// In zh, this message translates to:
  /// **'无效的校验规则'**
  String get mitigationFieldInvalidRegex;

  /// No description provided for @mitigationUnsupportedFieldType.
  ///
  /// In zh, this message translates to:
  /// **'不支持的字段类型'**
  String get mitigationUnsupportedFieldType;

  /// No description provided for @mitigationCommandCopied.
  ///
  /// In zh, this message translates to:
  /// **'命令已复制到剪贴板'**
  String get mitigationCommandCopied;

  /// No description provided for @aiModelConfig.
  ///
  /// In zh, this message translates to:
  /// **'AI模型配置'**
  String get aiModelConfig;

  /// No description provided for @skillScanTitle.
  ///
  /// In zh, this message translates to:
  /// **'AI 技能安全分析'**
  String get skillScanTitle;

  /// No description provided for @skillScanScanning.
  ///
  /// In zh, this message translates to:
  /// **'正在扫描'**
  String get skillScanScanning;

  /// No description provided for @skillScanCompleted.
  ///
  /// In zh, this message translates to:
  /// **'扫描完成'**
  String get skillScanCompleted;

  /// No description provided for @skillScanPreparing.
  ///
  /// In zh, this message translates to:
  /// **'准备中...'**
  String get skillScanPreparing;

  /// No description provided for @skillScanConfigError.
  ///
  /// In zh, this message translates to:
  /// **'请先配置 AI 模型'**
  String get skillScanConfigError;

  /// No description provided for @skillScanAllSafe.
  ///
  /// In zh, this message translates to:
  /// **'所有技能通过安全检查'**
  String get skillScanAllSafe;

  /// No description provided for @skillScanRiskDetected.
  ///
  /// In zh, this message translates to:
  /// **'检测到风险'**
  String get skillScanRiskDetected;

  /// No description provided for @skillScanIssues.
  ///
  /// In zh, this message translates to:
  /// **'个问题'**
  String get skillScanIssues;

  /// No description provided for @skillScanDelete.
  ///
  /// In zh, this message translates to:
  /// **'删除'**
  String get skillScanDelete;

  /// No description provided for @skillScanDeleted.
  ///
  /// In zh, this message translates to:
  /// **'已删除'**
  String get skillScanDeleted;

  /// No description provided for @skillScanTrust.
  ///
  /// In zh, this message translates to:
  /// **'信任'**
  String get skillScanTrust;

  /// No description provided for @skillScanTrusted.
  ///
  /// In zh, this message translates to:
  /// **'已信任'**
  String get skillScanTrusted;

  /// No description provided for @skillScanTrustTitle.
  ///
  /// In zh, this message translates to:
  /// **'信任技能'**
  String get skillScanTrustTitle;

  /// No description provided for @skillScanTrustConfirm.
  ///
  /// In zh, this message translates to:
  /// **'确定信任 \"{skillName}\" 吗？信任后该技能的风险将不再显示在主界面。'**
  String skillScanTrustConfirm(String skillName);

  /// No description provided for @skillScanFailed.
  ///
  /// In zh, this message translates to:
  /// **'扫描失败 - 下次扫描时将重试'**
  String get skillScanFailed;

  /// No description provided for @skillScanDeleteTitle.
  ///
  /// In zh, this message translates to:
  /// **'删除技能'**
  String get skillScanDeleteTitle;

  /// No description provided for @skillScanDeleteConfirm.
  ///
  /// In zh, this message translates to:
  /// **'确定要删除 \"{skillName}\" 吗？此操作不可撤销。'**
  String skillScanDeleteConfirm(String skillName);

  /// No description provided for @skillScanDone.
  ///
  /// In zh, this message translates to:
  /// **'完成'**
  String get skillScanDone;

  /// No description provided for @skillScanFailedLoadConfig.
  ///
  /// In zh, this message translates to:
  /// **'加载配置失败: {error}'**
  String skillScanFailedLoadConfig(String error);

  /// No description provided for @skillScanScanningSkill.
  ///
  /// In zh, this message translates to:
  /// **'扫描技能: {skillName}'**
  String skillScanScanningSkill(String skillName);

  /// No description provided for @skillScanRiskDetectedLog.
  ///
  /// In zh, this message translates to:
  /// **'检测到风险: {summary}'**
  String skillScanRiskDetectedLog(String summary);

  /// No description provided for @skillScanSkillSafe.
  ///
  /// In zh, this message translates to:
  /// **'技能安全'**
  String get skillScanSkillSafe;

  /// No description provided for @skillScanErrorScanning.
  ///
  /// In zh, this message translates to:
  /// **'扫描技能出错: {error}'**
  String skillScanErrorScanning(String error);

  /// No description provided for @skillScanAnalysisComplete.
  ///
  /// In zh, this message translates to:
  /// **'--- 分析完成 ---'**
  String get skillScanAnalysisComplete;

  /// No description provided for @skillScanSafe.
  ///
  /// In zh, this message translates to:
  /// **'安全'**
  String get skillScanSafe;

  /// No description provided for @skillScanRiskLevel.
  ///
  /// In zh, this message translates to:
  /// **'风险等级'**
  String get skillScanRiskLevel;

  /// No description provided for @skillScanSummary.
  ///
  /// In zh, this message translates to:
  /// **'分析摘要'**
  String get skillScanSummary;

  /// No description provided for @skillScanIssueType.
  ///
  /// In zh, this message translates to:
  /// **'类型'**
  String get skillScanIssueType;

  /// No description provided for @skillScanIssueSeverity.
  ///
  /// In zh, this message translates to:
  /// **'严重程度'**
  String get skillScanIssueSeverity;

  /// No description provided for @skillScanIssueFile.
  ///
  /// In zh, this message translates to:
  /// **'文件'**
  String get skillScanIssueFile;

  /// No description provided for @skillScanIssueDesc.
  ///
  /// In zh, this message translates to:
  /// **'描述'**
  String get skillScanIssueDesc;

  /// No description provided for @skillScanIssueEvidence.
  ///
  /// In zh, this message translates to:
  /// **'证据'**
  String get skillScanIssueEvidence;

  /// No description provided for @skillScanTypePromptInjection.
  ///
  /// In zh, this message translates to:
  /// **'提示词注入'**
  String get skillScanTypePromptInjection;

  /// No description provided for @skillScanTypeDataTheft.
  ///
  /// In zh, this message translates to:
  /// **'数据窃取'**
  String get skillScanTypeDataTheft;

  /// No description provided for @skillScanTypeCodeExecution.
  ///
  /// In zh, this message translates to:
  /// **'代码执行'**
  String get skillScanTypeCodeExecution;

  /// No description provided for @skillScanTypeSocialEngineering.
  ///
  /// In zh, this message translates to:
  /// **'社会工程'**
  String get skillScanTypeSocialEngineering;

  /// No description provided for @skillScanTypeSupplyChain.
  ///
  /// In zh, this message translates to:
  /// **'供应链攻击'**
  String get skillScanTypeSupplyChain;

  /// No description provided for @skillScanTypeOther.
  ///
  /// In zh, this message translates to:
  /// **'其他风险'**
  String get skillScanTypeOther;

  /// No description provided for @skillScanNoSkills.
  ///
  /// In zh, this message translates to:
  /// **'没有发现需要扫描的技能'**
  String get skillScanNoSkills;

  /// No description provided for @modelConfigTitle.
  ///
  /// In zh, this message translates to:
  /// **'安全模型'**
  String get modelConfigTitle;

  /// No description provided for @modelConfigProvider.
  ///
  /// In zh, this message translates to:
  /// **'模型供应商'**
  String get modelConfigProvider;

  /// No description provided for @modelConfigEndpoint.
  ///
  /// In zh, this message translates to:
  /// **'端点地址'**
  String get modelConfigEndpoint;

  /// No description provided for @modelConfigEndpointId.
  ///
  /// In zh, this message translates to:
  /// **'端点 ID'**
  String get modelConfigEndpointId;

  /// No description provided for @modelConfigBaseUrl.
  ///
  /// In zh, this message translates to:
  /// **'基础 URL'**
  String get modelConfigBaseUrl;

  /// No description provided for @modelConfigBaseUrlOptional.
  ///
  /// In zh, this message translates to:
  /// **'基础 URL (可选)'**
  String get modelConfigBaseUrlOptional;

  /// No description provided for @modelConfigApiKey.
  ///
  /// In zh, this message translates to:
  /// **'API 密钥'**
  String get modelConfigApiKey;

  /// No description provided for @modelConfigAccessKey.
  ///
  /// In zh, this message translates to:
  /// **'访问密钥'**
  String get modelConfigAccessKey;

  /// No description provided for @modelConfigSecretKey.
  ///
  /// In zh, this message translates to:
  /// **'密钥'**
  String get modelConfigSecretKey;

  /// No description provided for @modelConfigModelName.
  ///
  /// In zh, this message translates to:
  /// **'模型名称'**
  String get modelConfigModelName;

  /// No description provided for @modelConfigSave.
  ///
  /// In zh, this message translates to:
  /// **'保存'**
  String get modelConfigSave;

  /// No description provided for @modelConfigFillRequired.
  ///
  /// In zh, this message translates to:
  /// **'请填写所有必填字段'**
  String get modelConfigFillRequired;

  /// No description provided for @modelConfigSaveFailed.
  ///
  /// In zh, this message translates to:
  /// **'保存配置失败'**
  String get modelConfigSaveFailed;

  /// No description provided for @modelConfigRequired.
  ///
  /// In zh, this message translates to:
  /// **'请先配置 AI 模型才能继续使用'**
  String get modelConfigRequired;

  /// No description provided for @modelConfigTesting.
  ///
  /// In zh, this message translates to:
  /// **'测试连接中...'**
  String get modelConfigTesting;

  /// No description provided for @modelConfigValidateConnection.
  ///
  /// In zh, this message translates to:
  /// **'验证连通性'**
  String get modelConfigValidateConnection;

  /// No description provided for @modelConfigTestSuccess.
  ///
  /// In zh, this message translates to:
  /// **'连通性验证通过'**
  String get modelConfigTestSuccess;

  /// No description provided for @modelConfigSaving.
  ///
  /// In zh, this message translates to:
  /// **'保存中...'**
  String get modelConfigSaving;

  /// No description provided for @modelConfigTestFailed.
  ///
  /// In zh, this message translates to:
  /// **'连接测试失败: {error}'**
  String modelConfigTestFailed(String error);

  /// No description provided for @oneClickProtection.
  ///
  /// In zh, this message translates to:
  /// **'一键防护'**
  String get oneClickProtection;

  /// No description provided for @protectionAssetNotRunning.
  ///
  /// In zh, this message translates to:
  /// **'资产未运行，无法开启防护'**
  String get protectionAssetNotRunning;

  /// No description provided for @protectionMonitor.
  ///
  /// In zh, this message translates to:
  /// **'防护监控'**
  String get protectionMonitor;

  /// No description provided for @protectionStarting.
  ///
  /// In zh, this message translates to:
  /// **'防护启动中...'**
  String get protectionStarting;

  /// No description provided for @stopProtection.
  ///
  /// In zh, this message translates to:
  /// **'停止防护'**
  String get stopProtection;

  /// No description provided for @protectionStopping.
  ///
  /// In zh, this message translates to:
  /// **'停止中...'**
  String get protectionStopping;

  /// No description provided for @stopProtectionSuccess.
  ///
  /// In zh, this message translates to:
  /// **'防护已停止'**
  String get stopProtectionSuccess;

  /// No description provided for @stopProtectionFailed.
  ///
  /// In zh, this message translates to:
  /// **'停止防护失败：{error}'**
  String stopProtectionFailed(String error);

  /// No description provided for @launchAtStartup.
  ///
  /// In zh, this message translates to:
  /// **'开机自启'**
  String get launchAtStartup;

  /// No description provided for @auditLog.
  ///
  /// In zh, this message translates to:
  /// **'审计日志'**
  String get auditLog;

  /// No description provided for @protectionConfirmTitle.
  ///
  /// In zh, this message translates to:
  /// **'开启防护'**
  String get protectionConfirmTitle;

  /// No description provided for @protectionConfirmMessage.
  ///
  /// In zh, this message translates to:
  /// **'开启防护会对智能体行为实时分析，保障您的设备安全'**
  String get protectionConfirmMessage;

  /// No description provided for @protectionConfirmButton.
  ///
  /// In zh, this message translates to:
  /// **'确认开启'**
  String get protectionConfirmButton;

  /// No description provided for @protectionMonitorTitle.
  ///
  /// In zh, this message translates to:
  /// **'防护监控中心'**
  String get protectionMonitorTitle;

  /// No description provided for @protectionStatus.
  ///
  /// In zh, this message translates to:
  /// **'防护状态'**
  String get protectionStatus;

  /// No description provided for @protectionActive.
  ///
  /// In zh, this message translates to:
  /// **'防护中'**
  String get protectionActive;

  /// No description provided for @protectionInactive.
  ///
  /// In zh, this message translates to:
  /// **'未防护'**
  String get protectionInactive;

  /// No description provided for @behaviorAnalysis.
  ///
  /// In zh, this message translates to:
  /// **'行为分析'**
  String get behaviorAnalysis;

  /// No description provided for @threatDetection.
  ///
  /// In zh, this message translates to:
  /// **'威胁检测'**
  String get threatDetection;

  /// No description provided for @realTimeMonitor.
  ///
  /// In zh, this message translates to:
  /// **'实时监控'**
  String get realTimeMonitor;

  /// No description provided for @noThreatsDetected.
  ///
  /// In zh, this message translates to:
  /// **'暂无威胁检测'**
  String get noThreatsDetected;

  /// No description provided for @allSystemsNormal.
  ///
  /// In zh, this message translates to:
  /// **'所有系统运行正常'**
  String get allSystemsNormal;

  /// No description provided for @proxyStarting.
  ///
  /// In zh, this message translates to:
  /// **'正在启动保护代理...'**
  String get proxyStarting;

  /// No description provided for @proxyStartingDesc.
  ///
  /// In zh, this message translates to:
  /// **'正在读取配置并启动代理服务器'**
  String get proxyStartingDesc;

  /// No description provided for @proxyStartFailed.
  ///
  /// In zh, this message translates to:
  /// **'启动失败'**
  String get proxyStartFailed;

  /// No description provided for @retry.
  ///
  /// In zh, this message translates to:
  /// **'重试'**
  String get retry;

  /// No description provided for @analyzing.
  ///
  /// In zh, this message translates to:
  /// **'分析中...'**
  String get analyzing;

  /// No description provided for @analysisCount.
  ///
  /// In zh, this message translates to:
  /// **'分析次数'**
  String get analysisCount;

  /// No description provided for @messageCountLabel.
  ///
  /// In zh, this message translates to:
  /// **'消息数量'**
  String get messageCountLabel;

  /// No description provided for @warningCountLabel.
  ///
  /// In zh, this message translates to:
  /// **'警告次数'**
  String get warningCountLabel;

  /// No description provided for @blockedCount.
  ///
  /// In zh, this message translates to:
  /// **'拦截次数'**
  String get blockedCount;

  /// No description provided for @analysisLogs.
  ///
  /// In zh, this message translates to:
  /// **'分析日志'**
  String get analysisLogs;

  /// No description provided for @clear.
  ///
  /// In zh, this message translates to:
  /// **'清空'**
  String get clear;

  /// No description provided for @waitingLogs.
  ///
  /// In zh, this message translates to:
  /// **'等待分析日志...'**
  String get waitingLogs;

  /// No description provided for @securityEvents.
  ///
  /// In zh, this message translates to:
  /// **'安全事件'**
  String get securityEvents;

  /// No description provided for @noSecurityEvents.
  ///
  /// In zh, this message translates to:
  /// **'暂无安全事件'**
  String get noSecurityEvents;

  /// No description provided for @latestResult.
  ///
  /// In zh, this message translates to:
  /// **'最新分析结果'**
  String get latestResult;

  /// No description provided for @maliciousDetected.
  ///
  /// In zh, this message translates to:
  /// **'检测到恶意指令:'**
  String get maliciousDetected;

  /// No description provided for @dartProxyStarting.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 正在启动代理...'**
  String get dartProxyStarting;

  /// No description provided for @dartProxyStarted.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 已在端口 {port} 启动, 提供者: {provider}'**
  String dartProxyStarted(int port, String provider);

  /// No description provided for @dartProxyFailed.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 启动失败: {error}'**
  String dartProxyFailed(String error);

  /// No description provided for @dartProxyError.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 错误: {error}'**
  String dartProxyError(String error);

  /// No description provided for @dartProxyStopping.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 正在停止代理...'**
  String get dartProxyStopping;

  /// No description provided for @dartProxyStopped.
  ///
  /// In zh, this message translates to:
  /// **'[保护代理] 已停止'**
  String get dartProxyStopped;

  /// No description provided for @eventProxyStarting.
  ///
  /// In zh, this message translates to:
  /// **'正在启动保护代理'**
  String get eventProxyStarting;

  /// No description provided for @eventProxyStarted.
  ///
  /// In zh, this message translates to:
  /// **'代理已在端口 {port} 启动, 提供者: {provider}'**
  String eventProxyStarted(int port, String provider);

  /// No description provided for @eventProxyError.
  ///
  /// In zh, this message translates to:
  /// **'启动代理失败: {error}'**
  String eventProxyError(String error);

  /// No description provided for @eventProxyException.
  ///
  /// In zh, this message translates to:
  /// **'启动代理异常: {error}'**
  String eventProxyException(String error);

  /// No description provided for @proxyNewRequest.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 新请求 =========='**
  String get proxyNewRequest;

  /// No description provided for @proxyRequestInfo.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 请求: 模型={model}, 消息数={messageCount}, 流式={stream}'**
  String proxyRequestInfo(String model, int messageCount, String stream);

  /// No description provided for @proxyMessageInfo.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 消息[{index}] 角色={role}: {content}'**
  String proxyMessageInfo(int index, String role, String content);

  /// No description provided for @proxyToolActivityDetected.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 请求中检测到工具活动 =========='**
  String get proxyToolActivityDetected;

  /// No description provided for @proxyToolCallsFound.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 发现 {toolCount} 个工具调用, {resultCount} 个工具结果'**
  String proxyToolCallsFound(int toolCount, int resultCount);

  /// No description provided for @proxyResponseNonStream.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 响应 (非流式) =========='**
  String get proxyResponseNonStream;

  /// No description provided for @proxyResponseInfo.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 响应: 模型={model}, 选项数={choiceCount}'**
  String proxyResponseInfo(String model, int choiceCount);

  /// No description provided for @proxyResponseContent.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 响应内容: {content}'**
  String proxyResponseContent(String content);

  /// No description provided for @proxyToolCallsDetected.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 检测到工具调用 =========='**
  String get proxyToolCallsDetected;

  /// No description provided for @proxyToolCallCount.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 工具调用数量: {count}'**
  String proxyToolCallCount(int count);

  /// No description provided for @proxyToolCallName.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 工具调用[{index}]: {name}'**
  String proxyToolCallName(int index, String name);

  /// No description provided for @proxyToolCallArgs.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 工具调用[{index}] 参数: {args}'**
  String proxyToolCallArgs(int index, String args);

  /// No description provided for @proxyStartingAnalysis.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 开始分析 =========='**
  String get proxyStartingAnalysis;

  /// No description provided for @proxyStreamFinished.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 流式结束 (原因={reason}) =========='**
  String proxyStreamFinished(String reason);

  /// No description provided for @proxyToolCallsInStream.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 流式中检测到工具调用 =========='**
  String get proxyToolCallsInStream;

  /// No description provided for @proxyStreamContentNoTools.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 流式内容 (无工具调用): {content}'**
  String proxyStreamContentNoTools(String content);

  /// No description provided for @proxyAgentNotAvailable.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 防护代理不可用，允许请求'**
  String get proxyAgentNotAvailable;

  /// No description provided for @proxySendingAnalysis.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 发送到防护代理进行分析...'**
  String get proxySendingAnalysis;

  /// No description provided for @proxyOriginalTask.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 原始用户任务: {task}'**
  String proxyOriginalTask(String task);

  /// No description provided for @proxyMessageCountLog.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 消息数量: {count}'**
  String proxyMessageCountLog(int count);

  /// No description provided for @proxyAnalyzeMessage.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 分析消息[{index}] 角色={role}: {content}'**
  String proxyAnalyzeMessage(int index, String role, String content);

  /// No description provided for @proxyAnalysisError.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 分析错误: {error}，允许请求'**
  String proxyAnalysisError(String error);

  /// No description provided for @proxyAnalysisResult.
  ///
  /// In zh, this message translates to:
  /// **'[代理] ========== 分析结果 =========='**
  String get proxyAnalysisResult;

  /// No description provided for @proxyRiskLevel.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 风险等级: {level}'**
  String proxyRiskLevel(String level);

  /// No description provided for @proxyConfidence.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 置信度: {confidence}%'**
  String proxyConfidence(int confidence);

  /// No description provided for @proxySuggestedAction.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 建议操作: {action}'**
  String proxySuggestedAction(String action);

  /// No description provided for @proxyReason.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 原因: {reason}'**
  String proxyReason(String reason);

  /// No description provided for @proxyMaliciousInstruction.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 恶意指令: {instruction}'**
  String proxyMaliciousInstruction(String instruction);

  /// No description provided for @proxyTraceableQuote.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 可追溯引用: {quote}'**
  String proxyTraceableQuote(String quote);

  /// No description provided for @proxyBlocking.
  ///
  /// In zh, this message translates to:
  /// **'[代理] *** 拦截请求 *** 检测到风险!'**
  String get proxyBlocking;

  /// No description provided for @proxyWarning.
  ///
  /// In zh, this message translates to:
  /// **'[代理] *** 警告 *** 存在潜在风险，但允许请求'**
  String get proxyWarning;

  /// No description provided for @proxyAllowed.
  ///
  /// In zh, this message translates to:
  /// **'[代理] *** 允许 *** 请求安全'**
  String get proxyAllowed;

  /// No description provided for @proxyRestartingGateway.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 正在重启 openclaw 网关...'**
  String get proxyRestartingGateway;

  /// No description provided for @proxyGatewayRestartError.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 网关重启错误: {error}'**
  String proxyGatewayRestartError(String error);

  /// No description provided for @proxyGatewayRestartSuccess.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 网关重启成功'**
  String get proxyGatewayRestartSuccess;

  /// No description provided for @proxyGatewayRestartSkippedAppstore.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 跳过网关重启 (App Store 版本)'**
  String get proxyGatewayRestartSkippedAppstore;

  /// No description provided for @proxyServerError.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 服务器错误: {error}'**
  String proxyServerError(String error);

  /// No description provided for @proxyStarted.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 已在端口 {port} 启动, 转发至 {target} (提供商: {provider})'**
  String proxyStarted(int port, String target, String provider);

  /// No description provided for @proxyConfigUpdateFailed.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 警告: 配置更新失败: {error}'**
  String proxyConfigUpdateFailed(String error);

  /// No description provided for @proxyConfigUpdated.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 已更新 {provider} 提供商 baseUrl 为 {url}'**
  String proxyConfigUpdated(String provider, String url);

  /// No description provided for @configUpdated.
  ///
  /// In zh, this message translates to:
  /// **'配置更新成功'**
  String get configUpdated;

  /// No description provided for @proxyGatewayRestartFailed.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 警告: 网关重启失败: {error}'**
  String proxyGatewayRestartFailed(String error);

  /// No description provided for @proxyStopping.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 正在停止...'**
  String get proxyStopping;

  /// No description provided for @proxyConfigRestoreFailed.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 警告: 配置恢复失败: {error}'**
  String proxyConfigRestoreFailed(String error);

  /// No description provided for @proxyConfigRestored.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 已恢复 {provider} 提供商 baseUrl 为 {url}'**
  String proxyConfigRestored(String provider, String url);

  /// No description provided for @proxyStopped.
  ///
  /// In zh, this message translates to:
  /// **'[代理] 已停止'**
  String get proxyStopped;

  /// No description provided for @protectionAgentAnalyzing.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 正在分析 {count} 条消息...'**
  String protectionAgentAnalyzing(int count);

  /// No description provided for @protectionAgentSendingLLM.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 发送到 LLM 进行分析...'**
  String get protectionAgentSendingLLM;

  /// No description provided for @protectionAgentError.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 错误: {error}'**
  String protectionAgentError(String error);

  /// No description provided for @protectionAgentRawResponse.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 原始响应: {response}'**
  String protectionAgentRawResponse(String response);

  /// No description provided for @protectionAgentWarning.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 警告: {warning}'**
  String protectionAgentWarning(String warning);

  /// No description provided for @protectionAgentResult.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 风险等级: {level}, 置信度: {confidence}%'**
  String protectionAgentResult(String level, int confidence);

  /// No description provided for @protectionAgentReason.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 原因: {reason}'**
  String protectionAgentReason(String reason);

  /// No description provided for @protectionAgentSuggestedAction.
  ///
  /// In zh, this message translates to:
  /// **'[防护代理] 建议操作: {action}'**
  String protectionAgentSuggestedAction(String action);

  /// No description provided for @toolValidatorBlocked.
  ///
  /// In zh, this message translates to:
  /// **'[工具验证] *** 拦截 *** {reason}'**
  String toolValidatorBlocked(String reason);

  /// No description provided for @toolValidatorPassed.
  ///
  /// In zh, this message translates to:
  /// **'[工具验证] ✓ 通过: {toolName}'**
  String toolValidatorPassed(String toolName);

  /// No description provided for @dartAnalysisError.
  ///
  /// In zh, this message translates to:
  /// **'[分析] 错误: {error}'**
  String dartAnalysisError(String error);

  /// No description provided for @eventAnalysisError.
  ///
  /// In zh, this message translates to:
  /// **'分析错误: {error}'**
  String eventAnalysisError(String error);

  /// No description provided for @eventAnalysisCancelled.
  ///
  /// In zh, this message translates to:
  /// **'分析已取消'**
  String get eventAnalysisCancelled;

  /// No description provided for @eventProxyStopped.
  ///
  /// In zh, this message translates to:
  /// **'保护代理已停止'**
  String get eventProxyStopped;

  /// No description provided for @eventRequestBlocked.
  ///
  /// In zh, this message translates to:
  /// **'请求已拦截'**
  String get eventRequestBlocked;

  /// No description provided for @eventSecurityWarning.
  ///
  /// In zh, this message translates to:
  /// **'安全警告'**
  String get eventSecurityWarning;

  /// No description provided for @eventRequestAllowed.
  ///
  /// In zh, this message translates to:
  /// **'请求已允许'**
  String get eventRequestAllowed;

  /// No description provided for @eventAnalysisStarted.
  ///
  /// In zh, this message translates to:
  /// **'开始分析'**
  String get eventAnalysisStarted;

  /// No description provided for @eventToolCallsDetected.
  ///
  /// In zh, this message translates to:
  /// **'检测到工具调用'**
  String get eventToolCallsDetected;

  /// No description provided for @eventToolBlocked.
  ///
  /// In zh, this message translates to:
  /// **'工具调用已拦截 [{status}]: {reason}'**
  String eventToolBlocked(String status, String reason);

  /// No description provided for @eventToolWarning.
  ///
  /// In zh, this message translates to:
  /// **'工具调用警告 [{status}]: {reason}'**
  String eventToolWarning(String status, String reason);

  /// No description provided for @eventToolWarningAudit.
  ///
  /// In zh, this message translates to:
  /// **'工具调用风险(审计模式) [{status}]: {reason}'**
  String eventToolWarningAudit(String status, String reason);

  /// No description provided for @eventToolAllowed.
  ///
  /// In zh, this message translates to:
  /// **'工具调用已放行: {reason}'**
  String eventToolAllowed(String reason);

  /// No description provided for @eventToolBlockedWithRisk.
  ///
  /// In zh, this message translates to:
  /// **'工具调用已拦截 <{riskTag}> [{status}]: {reason}'**
  String eventToolBlockedWithRisk(String riskTag, String status, String reason);

  /// No description provided for @eventToolWarningWithRisk.
  ///
  /// In zh, this message translates to:
  /// **'工具调用警告 <{riskTag}> [{status}]: {reason}'**
  String eventToolWarningWithRisk(String riskTag, String status, String reason);

  /// No description provided for @eventToolWarningAuditWithRisk.
  ///
  /// In zh, this message translates to:
  /// **'工具调用风险(审计) <{riskTag}> [{status}]: {reason}'**
  String eventToolWarningAuditWithRisk(
    String riskTag,
    String status,
    String reason,
  );

  /// No description provided for @eventToolAllowedWithRisk.
  ///
  /// In zh, this message translates to:
  /// **'工具调用已放行 <{riskTag}>: {reason}'**
  String eventToolAllowedWithRisk(String riskTag, String reason);

  /// No description provided for @eventQuotaExceeded.
  ///
  /// In zh, this message translates to:
  /// **'配额超限 [{limitType}]: {current}/{limit}'**
  String eventQuotaExceeded(String limitType, int current, int limit);

  /// No description provided for @eventServerError.
  ///
  /// In zh, this message translates to:
  /// **'服务器错误: {error}'**
  String eventServerError(String error);

  /// No description provided for @totalTokens.
  ///
  /// In zh, this message translates to:
  /// **'Token总量'**
  String get totalTokens;

  /// No description provided for @promptTokens.
  ///
  /// In zh, this message translates to:
  /// **'输入Token'**
  String get promptTokens;

  /// No description provided for @completionTokens.
  ///
  /// In zh, this message translates to:
  /// **'输出Token'**
  String get completionTokens;

  /// No description provided for @toolCallCount.
  ///
  /// In zh, this message translates to:
  /// **'工具调用'**
  String get toolCallCount;

  /// No description provided for @tokenTrend.
  ///
  /// In zh, this message translates to:
  /// **'Token消耗趋势'**
  String get tokenTrend;

  /// No description provided for @toolCallTrend.
  ///
  /// In zh, this message translates to:
  /// **'工具调用趋势'**
  String get toolCallTrend;

  /// No description provided for @noDataYet.
  ///
  /// In zh, this message translates to:
  /// **'暂无数据'**
  String get noDataYet;

  /// No description provided for @analysisTokens.
  ///
  /// In zh, this message translates to:
  /// **'防护Token总量'**
  String get analysisTokens;

  /// No description provided for @analysisPromptTokens.
  ///
  /// In zh, this message translates to:
  /// **'防护输入Token'**
  String get analysisPromptTokens;

  /// No description provided for @analysisCompletionTokens.
  ///
  /// In zh, this message translates to:
  /// **'防护输出Token'**
  String get analysisCompletionTokens;

  /// No description provided for @analysisTokenTooltip.
  ///
  /// In zh, this message translates to:
  /// **'此部分为安全防护分析产生的Token消耗，不计入主要业务流程'**
  String get analysisTokenTooltip;

  /// No description provided for @protectionConfigTitle.
  ///
  /// In zh, this message translates to:
  /// **'防护配置'**
  String get protectionConfigTitle;

  /// No description provided for @securityPromptTab.
  ///
  /// In zh, this message translates to:
  /// **'智能规则'**
  String get securityPromptTab;

  /// No description provided for @tokenLimitTab.
  ///
  /// In zh, this message translates to:
  /// **'Token限制'**
  String get tokenLimitTab;

  /// No description provided for @permissionTab.
  ///
  /// In zh, this message translates to:
  /// **'权限设置'**
  String get permissionTab;

  /// No description provided for @botModelTab.
  ///
  /// In zh, this message translates to:
  /// **'Bot模型'**
  String get botModelTab;

  /// No description provided for @customSecurityPromptTitle.
  ///
  /// In zh, this message translates to:
  /// **'自定义安全提示词'**
  String get customSecurityPromptTitle;

  /// No description provided for @customSecurityPromptDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置您关注的安全规则，防护分析时将优先考虑这些规则'**
  String get customSecurityPromptDesc;

  /// No description provided for @customSecurityPromptPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'例如：\n- 禁止访问 /etc/passwd 文件\n- 禁止执行 rm -rf 命令\n- 禁止访问敏感目录 /home/user/.ssh/\n- 重点关注数据库连接相关操作'**
  String get customSecurityPromptPlaceholder;

  /// No description provided for @customSecurityPromptTip.
  ///
  /// In zh, this message translates to:
  /// **'您输入的内容将被包裹在 <USER_DEFINED></USER_DEFINED> 标签中，追加到分析系统提示词后面，模型分析时会以您关注的安全规则为主。'**
  String get customSecurityPromptTip;

  /// No description provided for @tokenLimitTitle.
  ///
  /// In zh, this message translates to:
  /// **'Token使用限制'**
  String get tokenLimitTitle;

  /// No description provided for @tokenLimitDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置Token使用上限，超过限制时代理将终止会话'**
  String get tokenLimitDesc;

  /// No description provided for @singleSessionTokenLimit.
  ///
  /// In zh, this message translates to:
  /// **'单轮会话Token上限'**
  String get singleSessionTokenLimit;

  /// No description provided for @singleSessionTokenLimitPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'留空或0表示不限制'**
  String get singleSessionTokenLimitPlaceholder;

  /// No description provided for @dailyTokenLimit.
  ///
  /// In zh, this message translates to:
  /// **'当日总Token上限'**
  String get dailyTokenLimit;

  /// No description provided for @dailyTokenLimitPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'留空或0表示不限制'**
  String get dailyTokenLimitPlaceholder;

  /// No description provided for @tokenLimitTip.
  ///
  /// In zh, this message translates to:
  /// **'当Token使用超过设定上限时，代理会返回超限错误并终止当前会话，防止过度消耗资源。'**
  String get tokenLimitTip;

  /// No description provided for @tokenUnitK.
  ///
  /// In zh, this message translates to:
  /// **'千'**
  String get tokenUnitK;

  /// No description provided for @tokenUnitM.
  ///
  /// In zh, this message translates to:
  /// **'百万'**
  String get tokenUnitM;

  /// No description provided for @tokenUnitBase.
  ///
  /// In zh, this message translates to:
  /// **'个'**
  String get tokenUnitBase;

  /// No description provided for @tokenPresetLabel.
  ///
  /// In zh, this message translates to:
  /// **'快捷选择'**
  String get tokenPresetLabel;

  /// No description provided for @tokenNoLimit.
  ///
  /// In zh, this message translates to:
  /// **'不限制'**
  String get tokenNoLimit;

  /// No description provided for @tokenPreset50K.
  ///
  /// In zh, this message translates to:
  /// **'5万'**
  String get tokenPreset50K;

  /// No description provided for @tokenPreset100K.
  ///
  /// In zh, this message translates to:
  /// **'10万'**
  String get tokenPreset100K;

  /// No description provided for @tokenPreset300K.
  ///
  /// In zh, this message translates to:
  /// **'30万'**
  String get tokenPreset300K;

  /// No description provided for @tokenPreset500K.
  ///
  /// In zh, this message translates to:
  /// **'50万'**
  String get tokenPreset500K;

  /// No description provided for @tokenPreset1M.
  ///
  /// In zh, this message translates to:
  /// **'100万'**
  String get tokenPreset1M;

  /// No description provided for @tokenPreset10M.
  ///
  /// In zh, this message translates to:
  /// **'1000万'**
  String get tokenPreset10M;

  /// No description provided for @tokenPreset50M.
  ///
  /// In zh, this message translates to:
  /// **'5000万'**
  String get tokenPreset50M;

  /// No description provided for @tokenPreset100M.
  ///
  /// In zh, this message translates to:
  /// **'1亿'**
  String get tokenPreset100M;

  /// No description provided for @pathPermissionTitle.
  ///
  /// In zh, this message translates to:
  /// **'路径访问权限'**
  String get pathPermissionTitle;

  /// No description provided for @pathPermissionDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置允许或禁止被代理的智能体访问的文件路径'**
  String get pathPermissionDesc;

  /// No description provided for @pathPermissionPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'例如: /etc/passwd, /home/user/.ssh/'**
  String get pathPermissionPlaceholder;

  /// No description provided for @networkPermissionTitle.
  ///
  /// In zh, this message translates to:
  /// **'网络访问权限'**
  String get networkPermissionTitle;

  /// No description provided for @networkPermissionDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置允许或禁止访问的网段、域名'**
  String get networkPermissionDesc;

  /// No description provided for @networkPermissionDescSandbox.
  ///
  /// In zh, this message translates to:
  /// **'沙箱模式下仅支持 *（所有地址）或 localhost 作为主机'**
  String get networkPermissionDescSandbox;

  /// No description provided for @networkPermissionPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'例如: 192.168.1.0/24, *.internal.com'**
  String get networkPermissionPlaceholder;

  /// No description provided for @networkPermissionPlaceholderSandbox.
  ///
  /// In zh, this message translates to:
  /// **'例如: *:*, localhost:8080, localhost:*'**
  String get networkPermissionPlaceholderSandbox;

  /// No description provided for @networkAddressInvalidForSandbox.
  ///
  /// In zh, this message translates to:
  /// **'沙箱模式限制：主机只能为 * 或 localhost，不支持具体IP地址、CIDR或域名'**
  String get networkAddressInvalidForSandbox;

  /// No description provided for @networkOutboundTitle.
  ///
  /// In zh, this message translates to:
  /// **'出栈 (Outbound)'**
  String get networkOutboundTitle;

  /// No description provided for @networkOutboundDesc.
  ///
  /// In zh, this message translates to:
  /// **'控制进程主动发起的对外连接'**
  String get networkOutboundDesc;

  /// No description provided for @networkInboundTitle.
  ///
  /// In zh, this message translates to:
  /// **'入栈 (Inbound)'**
  String get networkInboundTitle;

  /// No description provided for @networkInboundDesc.
  ///
  /// In zh, this message translates to:
  /// **'控制外部对进程发起的连接'**
  String get networkInboundDesc;

  /// No description provided for @shellPermissionTitle.
  ///
  /// In zh, this message translates to:
  /// **'Shell命令权限'**
  String get shellPermissionTitle;

  /// No description provided for @shellPermissionDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置允许或禁止执行的Shell命令'**
  String get shellPermissionDesc;

  /// No description provided for @shellPermissionPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'例如: rm, chmod, sudo'**
  String get shellPermissionPlaceholder;

  /// No description provided for @blacklistMode.
  ///
  /// In zh, this message translates to:
  /// **'黑名单'**
  String get blacklistMode;

  /// No description provided for @whitelistMode.
  ///
  /// In zh, this message translates to:
  /// **'白名单'**
  String get whitelistMode;

  /// No description provided for @permissionNote.
  ///
  /// In zh, this message translates to:
  /// **'注意：权限设置功能需要启用沙箱防护才能生效。启用后，网关进程将在受限环境中运行。'**
  String get permissionNote;

  /// No description provided for @shepherdRulesTab.
  ///
  /// In zh, this message translates to:
  /// **'用户规则'**
  String get shepherdRulesTab;

  /// No description provided for @shepherdRulesTitle.
  ///
  /// In zh, this message translates to:
  /// **'用户自定义规则'**
  String get shepherdRulesTitle;

  /// No description provided for @shepherdRulesDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置工具调用黑名单和敏感操作，用于增强防护'**
  String get shepherdRulesDesc;

  /// No description provided for @shepherdBlacklistTitle.
  ///
  /// In zh, this message translates to:
  /// **'工具调用黑名单'**
  String get shepherdBlacklistTitle;

  /// No description provided for @shepherdBlacklistDesc.
  ///
  /// In zh, this message translates to:
  /// **'禁止调用的工具名称，例如：delete_user, drop_table'**
  String get shepherdBlacklistDesc;

  /// No description provided for @shepherdBlacklistPlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'输入工具名称，回车添加'**
  String get shepherdBlacklistPlaceholder;

  /// No description provided for @shepherdSensitiveTitle.
  ///
  /// In zh, this message translates to:
  /// **'需要用户确认'**
  String get shepherdSensitiveTitle;

  /// No description provided for @shepherdSensitiveDesc.
  ///
  /// In zh, this message translates to:
  /// **'定义哪些操作属于敏感操作，例如：delete, remove'**
  String get shepherdSensitiveDesc;

  /// No description provided for @shepherdSensitivePlaceholder.
  ///
  /// In zh, this message translates to:
  /// **'输入敏感操作关键词，回车添加'**
  String get shepherdSensitivePlaceholder;

  /// No description provided for @shepherdRulesTip.
  ///
  /// In zh, this message translates to:
  /// **'这些规则将直接应用于 ShepherdGate 防护逻辑，对所有请求生效。'**
  String get shepherdRulesTip;

  /// No description provided for @securitySkillsTitle.
  ///
  /// In zh, this message translates to:
  /// **'安全技能'**
  String get securitySkillsTitle;

  /// No description provided for @securitySkillsDesc.
  ///
  /// In zh, this message translates to:
  /// **'系统内置的安全防护技能，自动应用于工具调用风险分析'**
  String get securitySkillsDesc;

  /// No description provided for @sandboxProtection.
  ///
  /// In zh, this message translates to:
  /// **'沙箱防护'**
  String get sandboxProtection;

  /// No description provided for @sandboxProtectionDesc.
  ///
  /// In zh, this message translates to:
  /// **'限制网关进程的系统资源访问，强制执行权限设置规则'**
  String get sandboxProtectionDesc;

  /// No description provided for @saveConfig.
  ///
  /// In zh, this message translates to:
  /// **'保存配置'**
  String get saveConfig;

  /// No description provided for @configSavedRestartRequired.
  ///
  /// In zh, this message translates to:
  /// **'配置已保存，需要重启防护以应用新配置'**
  String get configSavedRestartRequired;

  /// No description provided for @restartNow.
  ///
  /// In zh, this message translates to:
  /// **'立即重启'**
  String get restartNow;

  /// No description provided for @restarting.
  ///
  /// In zh, this message translates to:
  /// **'正在重启...'**
  String get restarting;

  /// No description provided for @protectionConfigBtn.
  ///
  /// In zh, this message translates to:
  /// **'防护配置'**
  String get protectionConfigBtn;

  /// No description provided for @auditLogTitle.
  ///
  /// In zh, this message translates to:
  /// **'审计日志'**
  String get auditLogTitle;

  /// No description provided for @auditLogTotal.
  ///
  /// In zh, this message translates to:
  /// **'分析次数'**
  String get auditLogTotal;

  /// No description provided for @auditLogRisk.
  ///
  /// In zh, this message translates to:
  /// **'风险次数'**
  String get auditLogRisk;

  /// No description provided for @auditLogBlocked.
  ///
  /// In zh, this message translates to:
  /// **'拦截次数'**
  String get auditLogBlocked;

  /// No description provided for @auditLogWarned.
  ///
  /// In zh, this message translates to:
  /// **'警告'**
  String get auditLogWarned;

  /// No description provided for @auditLogAllowed.
  ///
  /// In zh, this message translates to:
  /// **'允许次数'**
  String get auditLogAllowed;

  /// 审计日志搜索框占位说明
  ///
  /// In zh, this message translates to:
  /// **'搜索请求、回复、风险说明与消息/工具 JSON...'**
  String get auditLogSearchHint;

  /// 审计日志搜索框图标悬停帮助
  ///
  /// In zh, this message translates to:
  /// **'在请求正文、模型输出、风险说明以及 messages、tool_calls 的 JSON 原文中做子串匹配（不搜请求 ID 等结构化字段）。'**
  String get auditLogSearchTooltip;

  /// No description provided for @auditLogRiskOnly.
  ///
  /// In zh, this message translates to:
  /// **'仅显示风险'**
  String get auditLogRiskOnly;

  /// No description provided for @auditLogNoLogs.
  ///
  /// In zh, this message translates to:
  /// **'暂无审计日志'**
  String get auditLogNoLogs;

  /// No description provided for @auditLogRefresh.
  ///
  /// In zh, this message translates to:
  /// **'刷新'**
  String get auditLogRefresh;

  /// No description provided for @auditLogClearAll.
  ///
  /// In zh, this message translates to:
  /// **'清空全部'**
  String get auditLogClearAll;

  /// No description provided for @auditLogClearConfirmTitle.
  ///
  /// In zh, this message translates to:
  /// **'清空所有日志'**
  String get auditLogClearConfirmTitle;

  /// No description provided for @auditLogClearConfirmMessage.
  ///
  /// In zh, this message translates to:
  /// **'确定要清空所有审计日志吗？此操作无法撤销。'**
  String get auditLogClearConfirmMessage;

  /// No description provided for @auditLogCancel.
  ///
  /// In zh, this message translates to:
  /// **'取消'**
  String get auditLogCancel;

  /// No description provided for @auditLogClear.
  ///
  /// In zh, this message translates to:
  /// **'清空'**
  String get auditLogClear;

  /// No description provided for @auditLogDetail.
  ///
  /// In zh, this message translates to:
  /// **'日志详情'**
  String get auditLogDetail;

  /// No description provided for @auditLogId.
  ///
  /// In zh, this message translates to:
  /// **'日志 ID'**
  String get auditLogId;

  /// No description provided for @auditLogTimestamp.
  ///
  /// In zh, this message translates to:
  /// **'时间'**
  String get auditLogTimestamp;

  /// No description provided for @auditLogRequestId.
  ///
  /// In zh, this message translates to:
  /// **'请求 ID'**
  String get auditLogRequestId;

  /// No description provided for @auditLogModel.
  ///
  /// In zh, this message translates to:
  /// **'模型'**
  String get auditLogModel;

  /// No description provided for @auditLogAction.
  ///
  /// In zh, this message translates to:
  /// **'动作'**
  String get auditLogAction;

  /// No description provided for @auditLogRiskLevel.
  ///
  /// In zh, this message translates to:
  /// **'风险等级'**
  String get auditLogRiskLevel;

  /// No description provided for @auditLogConfidence.
  ///
  /// In zh, this message translates to:
  /// **'置信度'**
  String get auditLogConfidence;

  /// No description provided for @auditLogRiskReason.
  ///
  /// In zh, this message translates to:
  /// **'风险原因'**
  String get auditLogRiskReason;

  /// No description provided for @auditLogDuration.
  ///
  /// In zh, this message translates to:
  /// **'耗时'**
  String get auditLogDuration;

  /// No description provided for @auditLogTokens.
  ///
  /// In zh, this message translates to:
  /// **'Token数'**
  String get auditLogTokens;

  /// No description provided for @auditLogRequestContent.
  ///
  /// In zh, this message translates to:
  /// **'用户请求'**
  String get auditLogRequestContent;

  /// No description provided for @auditLogToolCalls.
  ///
  /// In zh, this message translates to:
  /// **'工具调用'**
  String get auditLogToolCalls;

  /// No description provided for @auditLogOutputContent.
  ///
  /// In zh, this message translates to:
  /// **'最终响应内容'**
  String get auditLogOutputContent;

  /// No description provided for @auditLogToolArguments.
  ///
  /// In zh, this message translates to:
  /// **'参数'**
  String get auditLogToolArguments;

  /// No description provided for @auditLogToolResult.
  ///
  /// In zh, this message translates to:
  /// **'结果'**
  String get auditLogToolResult;

  /// No description provided for @auditLogSensitive.
  ///
  /// In zh, this message translates to:
  /// **'敏感'**
  String get auditLogSensitive;

  /// No description provided for @auditLogExport.
  ///
  /// In zh, this message translates to:
  /// **'导出'**
  String get auditLogExport;

  /// No description provided for @auditLogExportFailed.
  ///
  /// In zh, this message translates to:
  /// **'导出失败'**
  String get auditLogExportFailed;

  /// No description provided for @auditLogPageInfo.
  ///
  /// In zh, this message translates to:
  /// **'第 {current} 页，共 {total} 页'**
  String auditLogPageInfo(int current, int total);

  /// No description provided for @auditLogEntryTotal.
  ///
  /// In zh, this message translates to:
  /// **'共 {count} 条'**
  String auditLogEntryTotal(int count);

  /// No description provided for @auditLogEntryRange.
  ///
  /// In zh, this message translates to:
  /// **'第 {start}-{end} 条'**
  String auditLogEntryRange(int start, int end);

  /// No description provided for @auditLogItemSerial.
  ///
  /// In zh, this message translates to:
  /// **'第 {n} 条'**
  String auditLogItemSerial(int n);

  /// No description provided for @auditLogSearchSubmitHint.
  ///
  /// In zh, this message translates to:
  /// **'按回车搜索'**
  String get auditLogSearchSubmitHint;

  /// No description provided for @auditLogActionAllow.
  ///
  /// In zh, this message translates to:
  /// **'已允许'**
  String get auditLogActionAllow;

  /// No description provided for @auditLogActionWarn.
  ///
  /// In zh, this message translates to:
  /// **'有风险'**
  String get auditLogActionWarn;

  /// No description provided for @auditLogActionBlock.
  ///
  /// In zh, this message translates to:
  /// **'已拦截'**
  String get auditLogActionBlock;

  /// No description provided for @auditLogActionHardBlock.
  ///
  /// In zh, this message translates to:
  /// **'强制拦截'**
  String get auditLogActionHardBlock;

  /// No description provided for @initializingProtectionMonitor.
  ///
  /// In zh, this message translates to:
  /// **'正在初始化防护监控'**
  String get initializingProtectionMonitor;

  /// No description provided for @initDatabase.
  ///
  /// In zh, this message translates to:
  /// **'初始化数据库'**
  String get initDatabase;

  /// No description provided for @startCallbackBridge.
  ///
  /// In zh, this message translates to:
  /// **'启动通信桥接'**
  String get startCallbackBridge;

  /// No description provided for @loadStatistics.
  ///
  /// In zh, this message translates to:
  /// **'加载统计数据'**
  String get loadStatistics;

  /// No description provided for @startListener.
  ///
  /// In zh, this message translates to:
  /// **'启动监听服务'**
  String get startListener;

  /// No description provided for @initFailed.
  ///
  /// In zh, this message translates to:
  /// **'初始化失败: {error}'**
  String initFailed(String error);

  /// No description provided for @configureAiModelFirst.
  ///
  /// In zh, this message translates to:
  /// **'请先配置 AI 模型（点击左下角设置图标）'**
  String get configureAiModelFirst;

  /// No description provided for @welcomeSlogan.
  ///
  /// In zh, this message translates to:
  /// **'欢迎使用 ClawdSecbot, 专注于 AI Bot 的安全防护'**
  String get welcomeSlogan;

  /// No description provided for @authDeniedExit.
  ///
  /// In zh, this message translates to:
  /// **'未授权目录访问，应用将退出'**
  String get authDeniedExit;

  /// No description provided for @onboardingScanReady.
  ///
  /// In zh, this message translates to:
  /// **'引导已完成，可以开始扫描'**
  String get onboardingScanReady;

  /// No description provided for @onboardingProtectionEnabled.
  ///
  /// In zh, this message translates to:
  /// **'已完成首次引导'**
  String get onboardingProtectionEnabled;

  /// No description provided for @onboardingCongratsTitle.
  ///
  /// In zh, this message translates to:
  /// **'恭喜'**
  String get onboardingCongratsTitle;

  /// No description provided for @onboardingPersonalDone.
  ///
  /// In zh, this message translates to:
  /// **'引导完成，正在返回主页面...'**
  String get onboardingPersonalDone;

  /// No description provided for @onboardingTitle.
  ///
  /// In zh, this message translates to:
  /// **'快速开始'**
  String get onboardingTitle;

  /// No description provided for @onboardingStepModelTitle.
  ///
  /// In zh, this message translates to:
  /// **'配置代理模型'**
  String get onboardingStepModelTitle;

  /// No description provided for @onboardingStepModelDesc.
  ///
  /// In zh, this message translates to:
  /// **'配置用于代理服务的模型'**
  String get onboardingStepModelDesc;

  /// No description provided for @onboardingStepProxyTitle.
  ///
  /// In zh, this message translates to:
  /// **'启动代理服务'**
  String get onboardingStepProxyTitle;

  /// No description provided for @onboardingStepProxyDesc.
  ///
  /// In zh, this message translates to:
  /// **'启动本地代理并创建监听'**
  String get onboardingStepProxyDesc;

  /// No description provided for @onboardingStepConnectivityTitle.
  ///
  /// In zh, this message translates to:
  /// **'修改Bot配置'**
  String get onboardingStepConnectivityTitle;

  /// No description provided for @onboardingStepConnectivityDesc.
  ///
  /// In zh, this message translates to:
  /// **'更新Bot配置并验证代理联通性'**
  String get onboardingStepConnectivityDesc;

  /// No description provided for @onboardingActionConfigureModel.
  ///
  /// In zh, this message translates to:
  /// **'配置代理模型'**
  String get onboardingActionConfigureModel;

  /// No description provided for @onboardingActionStartProxy.
  ///
  /// In zh, this message translates to:
  /// **'启动Proxy'**
  String get onboardingActionStartProxy;

  /// No description provided for @onboardingActionExecuteCommand.
  ///
  /// In zh, this message translates to:
  /// **'执行命令'**
  String get onboardingActionExecuteCommand;

  /// No description provided for @onboardingActionCopyCommand.
  ///
  /// In zh, this message translates to:
  /// **'复制命令'**
  String get onboardingActionCopyCommand;

  /// No description provided for @onboardingActionCopyPrompt.
  ///
  /// In zh, this message translates to:
  /// **'复制提示词'**
  String get onboardingActionCopyPrompt;

  /// No description provided for @onboardingCommandResultTitle.
  ///
  /// In zh, this message translates to:
  /// **'执行结果'**
  String get onboardingCommandResultTitle;

  /// No description provided for @onboardingCommandSuccess.
  ///
  /// In zh, this message translates to:
  /// **'命令执行成功'**
  String get onboardingCommandSuccess;

  /// No description provided for @onboardingActionBack.
  ///
  /// In zh, this message translates to:
  /// **'上一步'**
  String get onboardingActionBack;

  /// No description provided for @onboardingActionNext.
  ///
  /// In zh, this message translates to:
  /// **'下一步'**
  String get onboardingActionNext;

  /// No description provided for @onboardingActionSaveNext.
  ///
  /// In zh, this message translates to:
  /// **'保存并下一步'**
  String get onboardingActionSaveNext;

  /// No description provided for @onboardingActionFinish.
  ///
  /// In zh, this message translates to:
  /// **'完成引导'**
  String get onboardingActionFinish;

  /// No description provided for @onboardingActionSaveFinish.
  ///
  /// In zh, this message translates to:
  /// **'保存并完成'**
  String get onboardingActionSaveFinish;

  /// No description provided for @onboardingActionEnterApp.
  ///
  /// In zh, this message translates to:
  /// **'进入主页面'**
  String get onboardingActionEnterApp;

  /// No description provided for @onboardingWelcomeTitle.
  ///
  /// In zh, this message translates to:
  /// **'欢迎使用'**
  String get onboardingWelcomeTitle;

  /// No description provided for @onboardingWelcomeDesc.
  ///
  /// In zh, this message translates to:
  /// **'完成以下引导后即可开始使用安全防护能力。'**
  String get onboardingWelcomeDesc;

  /// No description provided for @onboardingQuickStartTitle.
  ///
  /// In zh, this message translates to:
  /// **'核心功能一览'**
  String get onboardingQuickStartTitle;

  /// No description provided for @onboardingQuickStartDesc.
  ///
  /// In zh, this message translates to:
  /// **'ClawdSecbot —— AI Bot 时代的守护神'**
  String get onboardingQuickStartDesc;

  /// No description provided for @onboardingFeatureInjectTitle.
  ///
  /// In zh, this message translates to:
  /// **'实时防护与意图偏离检测'**
  String get onboardingFeatureInjectTitle;

  /// No description provided for @onboardingFeatureInjectDesc.
  ///
  /// In zh, this message translates to:
  /// **'持续监听 Bot 对话与指令,识别注入、绕过、恶意引导等风险;判断业务意图偏离;高危请求自动拦截/质询/降权,确保安全边界内运行。'**
  String get onboardingFeatureInjectDesc;

  /// No description provided for @onboardingFeaturePermissionTitle.
  ///
  /// In zh, this message translates to:
  /// **'权限设置与工具/技能管控'**
  String get onboardingFeaturePermissionTitle;

  /// No description provided for @onboardingFeaturePermissionDesc.
  ///
  /// In zh, this message translates to:
  /// **'为 Bot 工具/插件/技能提供细粒度权限:可用能力、场景、参数上限;高危操作(如敏感数据访问、批量变更)设专属风控与二次确认,降低误用滥用风险。'**
  String get onboardingFeaturePermissionDesc;

  /// No description provided for @onboardingFeatureBaselineTitle.
  ///
  /// In zh, this message translates to:
  /// **'防护监控与审计追溯'**
  String get onboardingFeatureBaselineTitle;

  /// No description provided for @onboardingFeatureBaselineDesc.
  ///
  /// In zh, this message translates to:
  /// **'统一面板展示 Bot 防护状态、风险事件、拦截/告警记录;完整留存关键对话、工具调用、风控决策,支持事后审计、行为追溯与追责。'**
  String get onboardingFeatureBaselineDesc;

  /// No description provided for @onboardingSecurityModelTitle.
  ///
  /// In zh, this message translates to:
  /// **'安全模型配置'**
  String get onboardingSecurityModelTitle;

  /// No description provided for @onboardingSecurityModelDesc.
  ///
  /// In zh, this message translates to:
  /// **'ClawdSecbot 会在本机为您的 Bot 交互提供安全代理,重点分析外部工具调用风险.我们不会收集或保存您的个人隐私数据,所有配置与控制权都在您手中.'**
  String get onboardingSecurityModelDesc;

  /// No description provided for @onboardingBotModelTitle.
  ///
  /// In zh, this message translates to:
  /// **'Bot模型登记'**
  String get onboardingBotModelTitle;

  /// No description provided for @onboardingBotModelDesc.
  ///
  /// In zh, this message translates to:
  /// **'ClawdSecbot 会通过您登记的 Bot 模型信息来建立安全代理,并将请求安全转发至原始模型,确保各项任务正常完成.'**
  String get onboardingBotModelDesc;

  /// No description provided for @onboardingConfigUpdateTitle.
  ///
  /// In zh, this message translates to:
  /// **'Bot配置更新'**
  String get onboardingConfigUpdateTitle;

  /// No description provided for @onboardingConfigUpdateDesc.
  ///
  /// In zh, this message translates to:
  /// **'由于 Apple 的系统安全规范限制, ClawdSecbot 无法直接修改其他应用的配置文件. 请按照提示手动更新 openclaw.json 使配置更新.'**
  String get onboardingConfigUpdateDesc;

  /// No description provided for @onboardingConfigUpdateInstruction.
  ///
  /// In zh, this message translates to:
  /// **'打开web页面配置框，找到左侧Settings-Config-Models'**
  String get onboardingConfigUpdateInstruction;

  /// No description provided for @onboardingConfigUpdateComplete.
  ///
  /// In zh, this message translates to:
  /// **'完成'**
  String get onboardingConfigUpdateComplete;

  /// No description provided for @onboardingReuseBotModel.
  ///
  /// In zh, this message translates to:
  /// **'复用Bot模型配置'**
  String get onboardingReuseBotModel;

  /// No description provided for @onboardingReuseBotModelHint.
  ///
  /// In zh, this message translates to:
  /// **'使用与Bot相同的模型配置作为安全检测模型'**
  String get onboardingReuseBotModelHint;

  /// No description provided for @onboardingFinishTitle.
  ///
  /// In zh, this message translates to:
  /// **'恭喜完成'**
  String get onboardingFinishTitle;

  /// No description provided for @onboardingFinishDesc.
  ///
  /// In zh, this message translates to:
  /// **'基础配置已完成，进入主页面开始使用。'**
  String get onboardingFinishDesc;

  /// No description provided for @onboardingStatusConfigured.
  ///
  /// In zh, this message translates to:
  /// **'已完成配置'**
  String get onboardingStatusConfigured;

  /// No description provided for @onboardingStatusPending.
  ///
  /// In zh, this message translates to:
  /// **'待完成'**
  String get onboardingStatusPending;

  /// No description provided for @onboardingStatusProxyStarted.
  ///
  /// In zh, this message translates to:
  /// **'代理已启动'**
  String get onboardingStatusProxyStarted;

  /// No description provided for @onboardingStatusProxyNotStarted.
  ///
  /// In zh, this message translates to:
  /// **'代理未启动'**
  String get onboardingStatusProxyNotStarted;

  /// No description provided for @onboardingStatusWaitingCallback.
  ///
  /// In zh, this message translates to:
  /// **'等待回调确认'**
  String get onboardingStatusWaitingCallback;

  /// No description provided for @onboardingStatusCallbackReceived.
  ///
  /// In zh, this message translates to:
  /// **'已收到回调'**
  String get onboardingStatusCallbackReceived;

  /// No description provided for @onboardingProxyServiceStatus.
  ///
  /// In zh, this message translates to:
  /// **'代理服务状态'**
  String get onboardingProxyServiceStatus;

  /// No description provided for @onboardingProxyCallbackStatus.
  ///
  /// In zh, this message translates to:
  /// **'回调状态'**
  String get onboardingProxyCallbackStatus;

  /// No description provided for @onboardingProxyPortLabel.
  ///
  /// In zh, this message translates to:
  /// **'代理端口'**
  String get onboardingProxyPortLabel;

  /// No description provided for @onboardingProxyPortInvalid.
  ///
  /// In zh, this message translates to:
  /// **'代理端口无效'**
  String get onboardingProxyPortInvalid;

  /// No description provided for @onboardingProxyStarting.
  ///
  /// In zh, this message translates to:
  /// **'启动中'**
  String get onboardingProxyStarting;

  /// No description provided for @onboardingProxyStartedMessage.
  ///
  /// In zh, this message translates to:
  /// **'代理服务已启动，监听端口: {port}'**
  String onboardingProxyStartedMessage(int port);

  /// No description provided for @onboardingProxyCommandLabel.
  ///
  /// In zh, this message translates to:
  /// **'执行命令'**
  String get onboardingProxyCommandLabel;

  /// No description provided for @onboardingProxyCommandDesc.
  ///
  /// In zh, this message translates to:
  /// **'更新Bot配置'**
  String get onboardingProxyCommandDesc;

  /// No description provided for @onboardingProxyPromptLabel.
  ///
  /// In zh, this message translates to:
  /// **'提示词'**
  String get onboardingProxyPromptLabel;

  /// No description provided for @onboardingProxyPromptDesc.
  ///
  /// In zh, this message translates to:
  /// **'让OpenClaw执行提示词并把模型配置回调给 ClawdSecbot'**
  String get onboardingProxyPromptDesc;

  /// No description provided for @onboardingConnectivityHint.
  ///
  /// In zh, this message translates to:
  /// **'更新配置后，在OpenClaw中发送一条消息，例如：\"hello\"'**
  String get onboardingConnectivityHint;

  /// No description provided for @onboardingConnectivityStatus.
  ///
  /// In zh, this message translates to:
  /// **'联通性状态'**
  String get onboardingConnectivityStatus;

  /// No description provided for @onboardingConnectivityWaiting.
  ///
  /// In zh, this message translates to:
  /// **'等待检测消息...'**
  String get onboardingConnectivityWaiting;

  /// No description provided for @onboardingConnectivityDetected.
  ///
  /// In zh, this message translates to:
  /// **'已检测到消息'**
  String get onboardingConnectivityDetected;

  /// No description provided for @proxyTokenUsage.
  ///
  /// In zh, this message translates to:
  /// **'[代理] Token使用: 输入={promptTokens}, 输出={completionTokens}, 总计={totalTokens}'**
  String proxyTokenUsage(
    int promptTokens,
    int completionTokens,
    int totalTokens,
  );

  /// No description provided for @configAccessTitle.
  ///
  /// In zh, this message translates to:
  /// **'需要目录访问权限'**
  String get configAccessTitle;

  /// No description provided for @configAccessMessage.
  ///
  /// In zh, this message translates to:
  /// **'为了读取/写入 Documents、Desktop、Downloads 等受保护文件夹，应用需要授权访问您的主目录 (~).'**
  String get configAccessMessage;

  /// No description provided for @configAccessPaths.
  ///
  /// In zh, this message translates to:
  /// **'授权后可访问目录：'**
  String get configAccessPaths;

  /// No description provided for @selectDirectory.
  ///
  /// In zh, this message translates to:
  /// **'授权主目录'**
  String get selectDirectory;

  /// No description provided for @clearData.
  ///
  /// In zh, this message translates to:
  /// **'清空数据'**
  String get clearData;

  /// No description provided for @clearDataConfirmTitle.
  ///
  /// In zh, this message translates to:
  /// **'清空数据'**
  String get clearDataConfirmTitle;

  /// No description provided for @clearDataConfirmMessage.
  ///
  /// In zh, this message translates to:
  /// **'确定要清空所有日志、Token统计和分析数据吗？此操作不会删除模型配置和防护配置等运行时数据。此操作无法撤销。'**
  String get clearDataConfirmMessage;

  /// No description provided for @clearDataSuccess.
  ///
  /// In zh, this message translates to:
  /// **'数据已清空'**
  String get clearDataSuccess;

  /// No description provided for @clearDataFailed.
  ///
  /// In zh, this message translates to:
  /// **'清空数据失败'**
  String get clearDataFailed;

  /// No description provided for @auditOnlyMode.
  ///
  /// In zh, this message translates to:
  /// **'仅审计模式'**
  String get auditOnlyMode;

  /// No description provided for @auditOnlyModeDesc.
  ///
  /// In zh, this message translates to:
  /// **'不进行风险研判，仅记录审计日志'**
  String get auditOnlyModeDesc;

  /// No description provided for @auditOnlyModeShort.
  ///
  /// In zh, this message translates to:
  /// **'仅审计'**
  String get auditOnlyModeShort;

  /// No description provided for @auditOnlyModePendingHint.
  ///
  /// In zh, this message translates to:
  /// **'正在研判中，变更将在研判结束后生效'**
  String get auditOnlyModePendingHint;

  /// No description provided for @appStoreGuideTitle.
  ///
  /// In zh, this message translates to:
  /// **'配置引导'**
  String get appStoreGuideTitle;

  /// No description provided for @appStoreGuideDesc.
  ///
  /// In zh, this message translates to:
  /// **'请复制以下提示词并在 OpenClaw 中执行。'**
  String get appStoreGuideDesc;

  /// No description provided for @appStoreGuideCopied.
  ///
  /// In zh, this message translates to:
  /// **'已复制到剪贴板'**
  String get appStoreGuideCopied;

  /// No description provided for @appStoreGuideCopy.
  ///
  /// In zh, this message translates to:
  /// **'复制'**
  String get appStoreGuideCopy;

  /// No description provided for @appStoreGuideReceived.
  ///
  /// In zh, this message translates to:
  /// **'配置已接收'**
  String get appStoreGuideReceived;

  /// No description provided for @appStoreGuideWaiting.
  ///
  /// In zh, this message translates to:
  /// **'等待连接...'**
  String get appStoreGuideWaiting;

  /// No description provided for @appStoreGuideProxyStarting.
  ///
  /// In zh, this message translates to:
  /// **'代理服务启动中，请稍候...'**
  String get appStoreGuideProxyStarting;

  /// No description provided for @appStoreGuideContinueHint.
  ///
  /// In zh, this message translates to:
  /// **'配置完成后应用将自动继续。'**
  String get appStoreGuideContinueHint;

  /// No description provided for @scanStartTitle.
  ///
  /// In zh, this message translates to:
  /// **'资产扫描'**
  String get scanStartTitle;

  /// No description provided for @scanStartDesc.
  ///
  /// In zh, this message translates to:
  /// **'准备开始扫描资产。'**
  String get scanStartDesc;

  /// No description provided for @scanStartAuthRequired.
  ///
  /// In zh, this message translates to:
  /// **'需要授权'**
  String get scanStartAuthRequired;

  /// No description provided for @scanStartAuthDesc.
  ///
  /// In zh, this message translates to:
  /// **'需要访问配置目录的权限才能继续。'**
  String get scanStartAuthDesc;

  /// No description provided for @scanStartAuthBtn.
  ///
  /// In zh, this message translates to:
  /// **'授权主目录'**
  String get scanStartAuthBtn;

  /// No description provided for @scanStartBtn.
  ///
  /// In zh, this message translates to:
  /// **'开始资产扫描'**
  String get scanStartBtn;

  /// No description provided for @newVersionAvailable.
  ///
  /// In zh, this message translates to:
  /// **'发现新版本'**
  String get newVersionAvailable;

  /// No description provided for @versionAvailable.
  ///
  /// In zh, this message translates to:
  /// **'版本 {version} 现已发布。'**
  String versionAvailable(String version);

  /// No description provided for @download.
  ///
  /// In zh, this message translates to:
  /// **'下载'**
  String get download;

  /// No description provided for @later.
  ///
  /// In zh, this message translates to:
  /// **'稍后'**
  String get later;

  /// No description provided for @restoreConfig.
  ///
  /// In zh, this message translates to:
  /// **'恢复初始配置'**
  String get restoreConfig;

  /// No description provided for @restoreConfigConfirmTitle.
  ///
  /// In zh, this message translates to:
  /// **'恢复初始配置'**
  String get restoreConfigConfirmTitle;

  /// No description provided for @restoreConfigConfirmMessage.
  ///
  /// In zh, this message translates to:
  /// **'确定要恢复到初始配置吗？这将:\n\n1. 停止当前防护\n2. 恢复 openclaw.json 到首次启动防护前的状态\n3. 重启 openclaw 网关\n\n此操作无法撤销。'**
  String get restoreConfigConfirmMessage;

  /// No description provided for @exitRestoreTitle.
  ///
  /// In zh, this message translates to:
  /// **'退出前恢复 Bot 服务'**
  String get exitRestoreTitle;

  /// No description provided for @exitRestoreMessage.
  ///
  /// In zh, this message translates to:
  /// **'当前有 {count} 个 Bot 资产仍在使用 ClawdSecbot 代理流量。退出后如果不恢复默认配置，这些 Bot 可能无法继续工作。\n\n确认后，ClawdSecbot 会停止代理、防护失效，并尽量恢复 Bot 自身的未防护服务。'**
  String exitRestoreMessage(int count);

  /// No description provided for @exitRestoreConfirm.
  ///
  /// In zh, this message translates to:
  /// **'恢复并退出'**
  String get exitRestoreConfirm;

  /// No description provided for @exitRestoreExitOnly.
  ///
  /// In zh, this message translates to:
  /// **'仅退出'**
  String get exitRestoreExitOnly;

  /// No description provided for @exitRestoreInProgress.
  ///
  /// In zh, this message translates to:
  /// **'正在恢复 Bot 默认服务，请稍候...'**
  String get exitRestoreInProgress;

  /// No description provided for @exitRestoreFailedTitle.
  ///
  /// In zh, this message translates to:
  /// **'恢复失败，已取消退出'**
  String get exitRestoreFailedTitle;

  /// No description provided for @exitRestoreFailedMessage.
  ///
  /// In zh, this message translates to:
  /// **'以下资产恢复失败，为避免 Bot 退出后无法继续工作，本次不会退出应用：\n\n{details}'**
  String exitRestoreFailedMessage(String details);

  /// No description provided for @restoreConfigSuccess.
  ///
  /// In zh, this message translates to:
  /// **'配置已恢复到初始状态'**
  String get restoreConfigSuccess;

  /// No description provided for @restoreConfigFailed.
  ///
  /// In zh, this message translates to:
  /// **'恢复配置失败: {error}'**
  String restoreConfigFailed(String error);

  /// No description provided for @restoreConfigNoBackup.
  ///
  /// In zh, this message translates to:
  /// **'初始备份不存在，无法恢复'**
  String get restoreConfigNoBackup;

  /// No description provided for @restoreConfigDescription.
  ///
  /// In zh, this message translates to:
  /// **'恢复到首次启动前的状态'**
  String get restoreConfigDescription;

  /// No description provided for @restoringConfig.
  ///
  /// In zh, this message translates to:
  /// **'正在恢复配置...'**
  String get restoringConfig;

  /// No description provided for @generalSettings.
  ///
  /// In zh, this message translates to:
  /// **'通用设置'**
  String get generalSettings;

  /// No description provided for @scheduledScanSetting.
  ///
  /// In zh, this message translates to:
  /// **'定时扫描设置'**
  String get scheduledScanSetting;

  /// No description provided for @scheduledScanDescription.
  ///
  /// In zh, this message translates to:
  /// **'按设定间隔自动执行安全扫描'**
  String get scheduledScanDescription;

  /// No description provided for @scheduledScanOff.
  ///
  /// In zh, this message translates to:
  /// **'关闭'**
  String get scheduledScanOff;

  /// No description provided for @scheduledScanCustom.
  ///
  /// In zh, this message translates to:
  /// **'自定义'**
  String get scheduledScanCustom;

  /// No description provided for @scheduledScanCustomHint.
  ///
  /// In zh, this message translates to:
  /// **'请输入正整数并选择时间单位'**
  String get scheduledScanCustomHint;

  /// No description provided for @scheduledScanCustomValueHint.
  ///
  /// In zh, this message translates to:
  /// **'输入数值'**
  String get scheduledScanCustomValueHint;

  /// No description provided for @scheduledScanInvalidCustomValue.
  ///
  /// In zh, this message translates to:
  /// **'请输入大于 0 的数字'**
  String get scheduledScanInvalidCustomValue;

  /// No description provided for @scheduledScanOption60Seconds.
  ///
  /// In zh, this message translates to:
  /// **'60秒'**
  String get scheduledScanOption60Seconds;

  /// No description provided for @scheduledScanOption5Minutes.
  ///
  /// In zh, this message translates to:
  /// **'5分钟'**
  String get scheduledScanOption5Minutes;

  /// No description provided for @scheduledScanOption1Hour.
  ///
  /// In zh, this message translates to:
  /// **'1小时'**
  String get scheduledScanOption1Hour;

  /// No description provided for @scheduledScanUnitSeconds.
  ///
  /// In zh, this message translates to:
  /// **'秒'**
  String get scheduledScanUnitSeconds;

  /// No description provided for @scheduledScanUnitMinutes.
  ///
  /// In zh, this message translates to:
  /// **'分钟'**
  String get scheduledScanUnitMinutes;

  /// No description provided for @scheduledScanUnitHours.
  ///
  /// In zh, this message translates to:
  /// **'小时'**
  String get scheduledScanUnitHours;

  /// No description provided for @scheduledScanEvery.
  ///
  /// In zh, this message translates to:
  /// **'每 {value} {unit}'**
  String scheduledScanEvery(int value, String unit);

  /// No description provided for @dataManagement.
  ///
  /// In zh, this message translates to:
  /// **'数据管理'**
  String get dataManagement;

  /// No description provided for @clearDataDescription.
  ///
  /// In zh, this message translates to:
  /// **'清除日志、统计和分析数据'**
  String get clearDataDescription;

  /// No description provided for @permissionsSection.
  ///
  /// In zh, this message translates to:
  /// **'权限'**
  String get permissionsSection;

  /// No description provided for @dataSecurity.
  ///
  /// In zh, this message translates to:
  /// **'数据安全'**
  String get dataSecurity;

  /// No description provided for @dataExfiltrationRisk.
  ///
  /// In zh, this message translates to:
  /// **'数据外泄风险'**
  String get dataExfiltrationRisk;

  /// No description provided for @sensitiveAccessRisk.
  ///
  /// In zh, this message translates to:
  /// **'敏感访问风险'**
  String get sensitiveAccessRisk;

  /// No description provided for @emailDeleteRisk.
  ///
  /// In zh, this message translates to:
  /// **'邮件删除风险'**
  String get emailDeleteRisk;

  /// No description provided for @promptInjectionRisk.
  ///
  /// In zh, this message translates to:
  /// **'提示注入风险'**
  String get promptInjectionRisk;

  /// No description provided for @scriptExecutionRisk.
  ///
  /// In zh, this message translates to:
  /// **'脚本执行风险'**
  String get scriptExecutionRisk;

  /// No description provided for @generalToolRisk.
  ///
  /// In zh, this message translates to:
  /// **'通用工具风险'**
  String get generalToolRisk;

  /// No description provided for @skillAnalysis.
  ///
  /// In zh, this message translates to:
  /// **'基于智能技能 {skillName} 分析'**
  String skillAnalysis(String skillName);

  /// No description provided for @skillNameDataExfiltrationGuard.
  ///
  /// In zh, this message translates to:
  /// **'数据泄露防护'**
  String get skillNameDataExfiltrationGuard;

  /// No description provided for @skillNameFileAccessGuard.
  ///
  /// In zh, this message translates to:
  /// **'文件访问防护'**
  String get skillNameFileAccessGuard;

  /// No description provided for @skillNameEmailDeleteGuard.
  ///
  /// In zh, this message translates to:
  /// **'邮件删除防护'**
  String get skillNameEmailDeleteGuard;

  /// No description provided for @skillNamePromptInjectionGuard.
  ///
  /// In zh, this message translates to:
  /// **'提示注入防护'**
  String get skillNamePromptInjectionGuard;

  /// No description provided for @skillNameScriptExecutionGuard.
  ///
  /// In zh, this message translates to:
  /// **'脚本执行防护'**
  String get skillNameScriptExecutionGuard;

  /// No description provided for @skillNameGeneralToolRiskGuard.
  ///
  /// In zh, this message translates to:
  /// **'通用工具风险防护'**
  String get skillNameGeneralToolRiskGuard;

  /// No description provided for @securityEventDetail.
  ///
  /// In zh, this message translates to:
  /// **'安全事件详情'**
  String get securityEventDetail;

  /// No description provided for @eventBlocked.
  ///
  /// In zh, this message translates to:
  /// **'已拦截'**
  String get eventBlocked;

  /// No description provided for @eventToolExecution.
  ///
  /// In zh, this message translates to:
  /// **'工具执行'**
  String get eventToolExecution;

  /// No description provided for @eventOther.
  ///
  /// In zh, this message translates to:
  /// **'其他事件'**
  String get eventOther;

  /// No description provided for @eventTypeWarning.
  ///
  /// In zh, this message translates to:
  /// **'告警'**
  String get eventTypeWarning;

  /// No description provided for @riskTypeQuota.
  ///
  /// In zh, this message translates to:
  /// **'配额限制'**
  String get riskTypeQuota;

  /// No description provided for @riskTypeSandboxBlocked.
  ///
  /// In zh, this message translates to:
  /// **'沙箱拦截'**
  String get riskTypeSandboxBlocked;

  /// No description provided for @riskTypeNeedsConfirmation.
  ///
  /// In zh, this message translates to:
  /// **'待确认'**
  String get riskTypeNeedsConfirmation;

  /// No description provided for @eventTime.
  ///
  /// In zh, this message translates to:
  /// **'时间'**
  String get eventTime;

  /// No description provided for @eventActionDesc.
  ///
  /// In zh, this message translates to:
  /// **'动作描述'**
  String get eventActionDesc;

  /// No description provided for @eventRiskType.
  ///
  /// In zh, this message translates to:
  /// **'风险类型'**
  String get eventRiskType;

  /// No description provided for @eventSource.
  ///
  /// In zh, this message translates to:
  /// **'来源'**
  String get eventSource;

  /// No description provided for @eventSourceAgent.
  ///
  /// In zh, this message translates to:
  /// **'AI 分析引擎'**
  String get eventSourceAgent;

  /// No description provided for @eventSourceHeuristic.
  ///
  /// In zh, this message translates to:
  /// **'启发式检测'**
  String get eventSourceHeuristic;

  /// No description provided for @eventType.
  ///
  /// In zh, this message translates to:
  /// **'事件类型'**
  String get eventType;

  /// No description provided for @eventDetail.
  ///
  /// In zh, this message translates to:
  /// **'详细信息'**
  String get eventDetail;

  /// No description provided for @copyEventInfo.
  ///
  /// In zh, this message translates to:
  /// **'复制事件信息'**
  String get copyEventInfo;

  /// No description provided for @refresh.
  ///
  /// In zh, this message translates to:
  /// **'刷新'**
  String get refresh;

  /// No description provided for @clearAll.
  ///
  /// In zh, this message translates to:
  /// **'清空'**
  String get clearAll;

  /// No description provided for @viewSkillScanResults.
  ///
  /// In zh, this message translates to:
  /// **'技能检测历史'**
  String get viewSkillScanResults;

  /// No description provided for @viewSkillScanResultsTitle.
  ///
  /// In zh, this message translates to:
  /// **'技能检测历史'**
  String get viewSkillScanResultsTitle;

  /// No description provided for @rescanSecurityDiscovery.
  ///
  /// In zh, this message translates to:
  /// **'安全发现'**
  String get rescanSecurityDiscovery;

  /// No description provided for @rescanAll.
  ///
  /// In zh, this message translates to:
  /// **'所有信息'**
  String get rescanAll;

  /// No description provided for @deleteRiskSkill.
  ///
  /// In zh, this message translates to:
  /// **'删除技能'**
  String get deleteRiskSkill;

  /// No description provided for @deleteRiskSkillConfirm.
  ///
  /// In zh, this message translates to:
  /// **'确认删除技能 \"{skill}\" 吗？'**
  String deleteRiskSkillConfirm(String skill);

  /// No description provided for @deleteRiskSkillSuccess.
  ///
  /// In zh, this message translates to:
  /// **'技能删除成功'**
  String get deleteRiskSkillSuccess;

  /// No description provided for @deleteRiskSkillAlreadyMissing.
  ///
  /// In zh, this message translates to:
  /// **'技能目录已不存在，按已删除处理'**
  String get deleteRiskSkillAlreadyMissing;

  /// No description provided for @deleteRiskSkillFailed.
  ///
  /// In zh, this message translates to:
  /// **'技能删除失败'**
  String get deleteRiskSkillFailed;

  /// No description provided for @deleteRiskSkillUnavailable.
  ///
  /// In zh, this message translates to:
  /// **'缺少技能路径或哈希，无法删除'**
  String get deleteRiskSkillUnavailable;

  /// No description provided for @noSkillScanResults.
  ///
  /// In zh, this message translates to:
  /// **'暂无技能扫描记录'**
  String get noSkillScanResults;

  /// No description provided for @skillScanResultScannedAt.
  ///
  /// In zh, this message translates to:
  /// **'扫描时间: {time}'**
  String skillScanResultScannedAt(String time);

  /// No description provided for @skillScanResultIssueCount.
  ///
  /// In zh, this message translates to:
  /// **'{count} 个问题'**
  String skillScanResultIssueCount(int count);
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
