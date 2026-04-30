package skillagent

import (
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
)

// 默认配置值
const (
	DefaultMaxStep         = 50
	DefaultExecutionTimout = 10 * time.Minute
)

// SkillAgentConfig 包含 SkillAgent 的配置
type SkillAgentConfig struct {
	// ChatModel 是用户提供的 eino ChatModel 实例（必需）。
	// 必须支持 model.WithTools 调用选项。
	ChatModel model.ChatModel

	// SkillsDir 是包含技能的目录路径。
	// 每个包含 SKILL.md 文件的子目录视为一个技能。
	SkillsDir string

	// MaxStep 是 ADK Agent 的最大迭代次数。
	// Default: 50
	MaxStep int

	// CustomTools 是供所有技能使用的额外工具。
	// 这些工具会通过 ToolsConfig 传递给 ADK Agent。
	CustomTools []tool.BaseTool

	// ValidateCommand 是传递给 local backend 的命令校验函数。
	// 用于在 filesystem middleware 的 execute 工具执行前拦截非法命令。
	// 返回 non-nil error 时拒绝执行。
	ValidateCommand func(string) error

	// ExecutionTimeout 是技能执行的最大持续时间。
	// Default: 10 minutes
	ExecutionTimeout time.Duration

	// Hooks 包含生命周期回调钩子
	Hooks *SkillHooks

	// SystemPromptPrefix 会添加到技能指令之前作为系统提示词前缀。
	// 用于添加全局上下文或约束条件。
	SystemPromptPrefix string

	// SystemPromptSuffix 会添加到技能指令之后作为系统提示词后缀。
	// 用于添加输出格式要求。
	SystemPromptSuffix string

	// DisableFilesystemTools 禁用 filesystem middleware 提供的通用文件系统工具（ls, read_file, execute 等）。
	// 当设置为 true 时，agent 只能使用 skill middleware 和 CustomTools 中的工具。
	// 适用于需要严格控制文件访问范围的场景（如 Skill 安全扫描）。
	DisableFilesystemTools bool
}

// SkillHooks 包含技能执行的生命周期回调钩子
type SkillHooks struct {
	// OnSkillDiscovered 在扫描发现技能时调用
	OnSkillDiscovered func(metadata *SkillMetadata)

	// OnSkillActivated 在技能激活时调用
	OnSkillActivated func(manifest *SkillManifest)

	// OnSkillSelected 在技能被选中执行时调用
	OnSkillSelected func(skillName string)

	// OnToolCall 在每次工具调用前调用
	OnToolCall func(toolName string, arguments string)

	// OnToolResult 在每次工具调用后调用
	OnToolResult func(toolName string, result string, err error)

	// OnAgentThinking 在 agent 推理时调用（如果可用）
	OnAgentThinking func(thought string)

	// OnComplete 在技能执行完成时调用
	OnComplete func(result *ExecutionResult)

	// OnError 在执行出错时调用
	OnError func(err error)
}

// Validate 校验配置
func (c *SkillAgentConfig) Validate() error {
	if c.ChatModel == nil {
		return NewValidationError("ChatModel", "ChatModel is required")
	}

	if c.SkillsDir == "" {
		return NewValidationError("SkillsDir", "SkillsDir is required")
	}

	return nil
}

// ApplyDefaults 为未设置的字段应用默认值
func (c *SkillAgentConfig) ApplyDefaults() {
	if c.MaxStep <= 0 {
		c.MaxStep = DefaultMaxStep
	}
	if c.ExecutionTimeout <= 0 {
		c.ExecutionTimeout = DefaultExecutionTimout
	}
}

// ExecuteOption 是 Execute 方法的函数式选项
type ExecuteOption func(*executeOptions)

// executeOptions 包含单次执行的选项
type executeOptions struct {
	// ForceSkill 强制使用指定技能执行（跳过自动匹配）
	ForceSkill string

	// AdditionalContext 提供额外上下文，包含在提示词中
	AdditionalContext string

	// Timeout 覆盖默认执行超时
	Timeout time.Duration

	// Variables 提供模板渲染变量
	Variables map[string]interface{}
}

// WithForceSkill 强制使用指定技能执行
func WithForceSkill(skillName string) ExecuteOption {
	return func(o *executeOptions) {
		o.ForceSkill = skillName
	}
}

// WithAdditionalContext 添加额外上下文
func WithAdditionalContext(context string) ExecuteOption {
	return func(o *executeOptions) {
		o.AdditionalContext = context
	}
}

// WithTimeout 设置自定义超时
func WithTimeout(timeout time.Duration) ExecuteOption {
	return func(o *executeOptions) {
		o.Timeout = timeout
	}
}

// WithVariables 提供模板渲染变量
func WithVariables(vars map[string]interface{}) ExecuteOption {
	return func(o *executeOptions) {
		o.Variables = vars
	}
}

// applyExecuteOptions 应用执行选项并返回最终选项
func applyExecuteOptions(opts []ExecuteOption) *executeOptions {
	options := &executeOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// ExecutionResult 表示技能执行结果
type ExecutionResult struct {
	// Success 表示执行是否成功完成
	Success bool `json:"success"`

	// Output 是技能执行的最终输出
	Output string `json:"output"`

	// ActivatedSkill 是被执行的技能名称
	ActivatedSkill string `json:"activated_skill,omitempty"`

	// ToolCallHistory 包含执行期间的工具调用历史
	ToolCallHistory []ToolCallRecord `json:"tool_call_history,omitempty"`

	// Error 包含执行失败时的错误
	Error error `json:"error,omitempty"`

	// Duration 是总执行时间
	Duration time.Duration `json:"duration,omitempty"`

	// TokensUsed 是估算的 token 使用量（如果可用）
	TokensUsed int `json:"tokens_used,omitempty"`

	// PromptTokens is the input token usage during execution.
	PromptTokens int `json:"prompt_tokens,omitempty"`

	// CompletionTokens is the output token usage during execution.
	CompletionTokens int `json:"completion_tokens,omitempty"`
}

// ToolCallRecord 表示执行期间的单次工具调用记录
type ToolCallRecord struct {
	// ToolName 是被调用的工具名称
	ToolName string `json:"tool_name"`

	// Arguments 是传递给工具的 JSON 参数
	Arguments string `json:"arguments"`

	// Result 是工具返回的结果
	Result string `json:"result"`

	// Error 是工具调用失败时的错误
	Error string `json:"error,omitempty"`

	// Duration 是工具调用耗时
	Duration time.Duration `json:"duration"`

	// Timestamp 是工具调用时间
	Timestamp time.Time `json:"timestamp"`
}
