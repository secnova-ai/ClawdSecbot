package shepherd

import (
	"embed"
	"strings"
)

//go:embed bundled_prompts/*.md
var bundledPromptsFS embed.FS

var (
	userInputSystemPromptTemplate     = mustReadBundledPrompt("bundled_prompts/user_input_system.md")
	toolCallGuardSystemPromptTemplate = mustReadBundledPrompt("bundled_prompts/tool_call_guard_system.md")
	finalResultSystemPromptTemplate   = mustReadBundledPrompt("bundled_prompts/final_result_system.md")
)

func mustReadBundledPrompt(path string) string {
	data, err := bundledPromptsFS.ReadFile(path)
	if err != nil {
		panic("read bundled shepherd prompt failed: " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}

func renderPromptTemplate(template string, replacements ...string) string {
	return strings.NewReplacer(replacements...).Replace(template)
}
