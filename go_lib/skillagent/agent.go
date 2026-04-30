package skillagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino-ext/adk/backend/local"
)

// SkillAgent 是基于 Eino ADK 的技能执行引擎。
// 使用 adk.NewChatModelAgent + skill.NewMiddleware + filesystem.New 实现技能发现、加载和执行。
type SkillAgent struct {
	mu     sync.RWMutex
	config *SkillAgentConfig

	// ADK 组件（在 init 时创建，跨请求复用）
	localBackend         *local.Local
	skillBackend         skill.Backend
	skillMiddleware      adk.ChatModelAgentMiddleware
	filesystemMiddleware adk.ChatModelAgentMiddleware
}

// NewSkillAgent 创建新的 SkillAgent 实例。
// 初始化 local backend、skill middleware 和 filesystem middleware。
func NewSkillAgent(ctx context.Context, config *SkillAgentConfig) (*SkillAgent, error) {
	config.ApplyDefaults()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// 创建 local backend
	localBackend, err := local.NewBackend(ctx, &local.Config{
		ValidateCommand: config.ValidateCommand,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create local backend: %w", err)
	}

	// 创建 skill backend（基于文件系统）
	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: localBackend,
		BaseDir: config.SkillsDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill backend: %w", err)
	}

	// 创建 skill middleware
	skillMw, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: skillBackend,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill middleware: %w", err)
	}

	// 创建 filesystem middleware（提供 ls/read_file/execute 等工具）
	// 当 DisableFilesystemTools 为 true 时跳过创建
	var fsMw adk.ChatModelAgentMiddleware
	if !config.DisableFilesystemTools {
		fsMw, err = filesystem.New(ctx, &filesystem.MiddlewareConfig{
			Backend:        localBackend,
			StreamingShell: localBackend,
			// 禁用写入工具（技能执行场景不需要写文件）
			WriteFileToolConfig: &filesystem.ToolConfig{Disable: true},
			EditFileToolConfig:  &filesystem.ToolConfig{Disable: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create filesystem middleware: %w", err)
		}
	}

	return &SkillAgent{
		config:               config,
		localBackend:         localBackend,
		skillBackend:         skillBackend,
		skillMiddleware:      skillMw,
		filesystemMiddleware: fsMw, // nil when DisableFilesystemTools is true
	}, nil
}

// ListSkills 列出所有已发现的技能元数据。
func (sa *SkillAgent) ListSkills(ctx context.Context) ([]*SkillMetadata, error) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	frontMatters, err := sa.skillBackend.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	result := make([]*SkillMetadata, 0, len(frontMatters))
	for _, fm := range frontMatters {
		result = append(result, &SkillMetadata{
			Name:        fm.Name,
			Description: fm.Description,
		})
	}
	return result, nil
}

// Execute 执行任务，使用 ADK Agent 自动匹配和调用技能。
func (sa *SkillAgent) Execute(ctx context.Context, userInput string, opts ...ExecuteOption) (*ExecutionResult, error) {
	options := applyExecuteOptions(opts)
	startTime := time.Now()

	// 设置超时
	timeout := sa.config.ExecutionTimeout
	if options.Timeout > 0 {
		timeout = options.Timeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 触发技能选择钩子
	if options.ForceSkill != "" && sa.config.Hooks != nil && sa.config.Hooks.OnSkillSelected != nil {
		sa.config.Hooks.OnSkillSelected(options.ForceSkill)
	}

	// 创建 SkillContext
	skillCtx := NewSkillContext(options.ForceSkill, sa.config.SkillsDir, userInput)
	if options.Variables != nil {
		for k, v := range options.Variables {
			skillCtx.SetVariable(k, v)
		}
	}
	execCtx = WithSkillContext(execCtx, skillCtx)

	// 构建系统提示词
	instruction := sa.buildInstruction(options)

	// 构建 ADK Agent（每次执行创建新实例，因为 Instruction 和 tools 可能不同）
	sa.mu.RLock()
	// 构建 middleware handlers
	var handlers []adk.ChatModelAgentMiddleware
	if sa.filesystemMiddleware != nil {
		handlers = append(handlers, sa.filesystemMiddleware)
	}
	handlers = append(handlers, sa.skillMiddleware)

	agent, err := adk.NewChatModelAgent(execCtx, &adk.ChatModelAgentConfig{
		Name:          "skill_agent",
		Description:   "AI agent that executes skills with filesystem and tool capabilities",
		Instruction:   instruction,
		Model:         sa.config.ChatModel,
		MaxIterations: sa.config.MaxStep,
		Handlers:      handlers,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: sa.config.CustomTools,
			},
		},
	})
	sa.mu.RUnlock()

	if err != nil {
		return &ExecutionResult{
			Success:        false,
			ActivatedSkill: options.ForceSkill,
			Error:          fmt.Errorf("failed to create ADK agent: %w", err),
			Duration:       time.Since(startTime),
		}, err
	}

	// 构建用户消息
	userMessage := userInput
	if options.AdditionalContext != "" {
		userMessage = userInput + "\n\nAdditional Context:\n" + options.AdditionalContext
	}

	// 执行 agent
	input := &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage(userMessage),
		},
	}
	iter := agent.Run(execCtx, input)

	// 收集输出
	output, usage, err := collectAgentOutput(execCtx, iter)
	if err != nil {
		if usage.TotalTokens == 0 && output != "" {
			usage = estimateSkillAgentUsage(instruction, userMessage, output)
		}
		if execCtx.Err() == context.DeadlineExceeded {
			return &ExecutionResult{
				Success:          false,
				Output:           output,
				ActivatedSkill:   options.ForceSkill,
				Error:            ErrExecutionTimeout,
				Duration:         time.Since(startTime),
				TokensUsed:       usage.TotalTokens,
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: usage.CompletionTokens,
			}, ErrExecutionTimeout
		}
		return &ExecutionResult{
			Success:          false,
			Output:           output,
			ActivatedSkill:   options.ForceSkill,
			Error:            fmt.Errorf("agent execution failed: %w", err),
			Duration:         time.Since(startTime),
			TokensUsed:       usage.TotalTokens,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
		}, err
	}
	if usage.TotalTokens == 0 {
		usage = estimateSkillAgentUsage(instruction, userMessage, output)
	}

	result := &ExecutionResult{
		Success:          true,
		Output:           output,
		ActivatedSkill:   options.ForceSkill,
		Duration:         time.Since(startTime),
		TokensUsed:       usage.TotalTokens,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
	}

	// 触发完成钩子
	if sa.config.Hooks != nil && sa.config.Hooks.OnComplete != nil {
		sa.config.Hooks.OnComplete(result)
	}

	return result, nil
}

// ExecuteWithSkill 使用指定技能执行任务。
func (sa *SkillAgent) ExecuteWithSkill(ctx context.Context, skillName string, userInput string) (*ExecutionResult, error) {
	return sa.Execute(ctx, userInput, WithForceSkill(skillName))
}

// ExecuteStream 执行任务并返回流式事件通道。
func (sa *SkillAgent) ExecuteStream(ctx context.Context, userInput string, opts ...ExecuteOption) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent, 100)

	go func() {
		defer close(eventCh)
		emitter := NewStreamEventEmitter(eventCh)

		// 发射技能发现事件
		skills, err := sa.ListSkills(ctx)
		if err == nil {
			emitter.EmitSkillDiscovery(skills)
		}

		// 执行并捕获结果
		result, err := sa.Execute(ctx, userInput, opts...)

		if err != nil {
			emitter.EmitError(err)
		}

		if result != nil {
			if result.Output != "" {
				emitter.EmitFinalOutput(result.Output)
			}
			emitter.EmitComplete(result.Success, len(result.ToolCallHistory))
		} else {
			emitter.EmitComplete(false, 0)
		}
	}()

	return eventCh, nil
}

// buildInstruction 构建 ADK Agent 的 Instruction 系统提示词。
func (sa *SkillAgent) buildInstruction(options *executeOptions) string {
	var parts []string

	if sa.config.SystemPromptPrefix != "" {
		parts = append(parts, sa.config.SystemPromptPrefix)
	}

	// 如果指定了技能，在提示词中说明
	if options.ForceSkill != "" {
		parts = append(parts, fmt.Sprintf("You are executing the skill: %s", options.ForceSkill))
	}

	if sa.config.SystemPromptSuffix != "" {
		parts = append(parts, sa.config.SystemPromptSuffix)
	}

	return strings.Join(parts, "\n\n")
}

// Close 释放 agent 持有的资源。
func (sa *SkillAgent) Close() error {
	return nil
}

// SkillsDir 返回技能目录路径。
func (sa *SkillAgent) SkillsDir() string {
	return sa.config.SkillsDir
}

// GetSkillBackend 返回 skill backend，供外部模块查询技能列表使用。
func (sa *SkillAgent) GetSkillBackend() skill.Backend {
	return sa.skillBackend
}

// RebuildMiddlewares 重建所有 middleware（配置变更时调用）。
func (sa *SkillAgent) RebuildMiddlewares(ctx context.Context) error {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	// 创建新的 local backend
	localBackend, err := local.NewBackend(ctx, &local.Config{
		ValidateCommand: sa.config.ValidateCommand,
	})
	if err != nil {
		return fmt.Errorf("failed to create local backend: %w", err)
	}

	// 创建新的 skill backend
	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: localBackend,
		BaseDir: sa.config.SkillsDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create skill backend: %w", err)
	}

	// 创建新的 skill middleware
	skillMw, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: skillBackend,
	})
	if err != nil {
		return fmt.Errorf("failed to create skill middleware: %w", err)
	}

	// 创建新的 filesystem middleware
	// 当 DisableFilesystemTools 为 true 时跳过创建
	var fsMw adk.ChatModelAgentMiddleware
	if !sa.config.DisableFilesystemTools {
		fsMw, err = filesystem.New(ctx, &filesystem.MiddlewareConfig{
			Backend:             localBackend,
			StreamingShell:      localBackend,
			WriteFileToolConfig: &filesystem.ToolConfig{Disable: true},
			EditFileToolConfig:  &filesystem.ToolConfig{Disable: true},
		})
		if err != nil {
			return fmt.Errorf("failed to create filesystem middleware: %w", err)
		}
	}

	sa.localBackend = localBackend
	sa.skillBackend = skillBackend
	sa.skillMiddleware = skillMw
	sa.filesystemMiddleware = fsMw // nil when DisableFilesystemTools is true

	return nil
}

// UpdateConfig 更新配置并重建 middleware。
func (sa *SkillAgent) UpdateConfig(ctx context.Context, config *SkillAgentConfig) error {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	sa.mu.Lock()
	sa.config = config
	sa.mu.Unlock()

	return sa.RebuildMiddlewares(ctx)
}

// buildToolsList 构建当前可用的工具列表。
func buildToolsList(customTools []tool.BaseTool) []tool.BaseTool {
	if len(customTools) == 0 {
		return nil
	}
	tools := make([]tool.BaseTool, len(customTools))
	copy(tools, customTools)
	return tools
}
