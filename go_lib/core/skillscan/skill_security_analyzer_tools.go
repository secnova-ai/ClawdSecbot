package skillscan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ListSkillFilesTool lists all files in the skill directory
type ListSkillFilesTool struct {
	skillPath string
}

// NewListSkillFilesTool creates a new ListSkillFilesTool
func NewListSkillFilesTool(skillPath string) *ListSkillFilesTool {
	return &ListSkillFilesTool{skillPath: skillPath}
}

// Info returns the tool information
func (t *ListSkillFilesTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "list_skill_files",
		Desc:        "List all files and directories in the skill folder. Returns a JSON array of file paths.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

// InvokableRun executes the tool
func (t *ListSkillFilesTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	type FileEntry struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}

	var entries []FileEntry
	rootInfo, statErr := os.Stat(t.skillPath)
	if statErr != nil {
		return "", fmt.Errorf("failed to stat skill path: %w", statErr)
	}
	if !rootInfo.IsDir() {
		entries = append(entries, FileEntry{
			Path:  filepath.Base(t.skillPath),
			IsDir: false,
			Size:  rootInfo.Size(),
		})
		result, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal result: %w", err)
		}
		return string(result), nil
	}

	err := filepath.Walk(t.skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(t.skillPath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		entries = append(entries, FileEntry{
			Path:  relPath,
			IsDir: info.IsDir(),
			Size:  info.Size(),
		})

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	result, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(result), nil
}

// ReadSkillFileTool reads content from a file in the skill directory
type ReadSkillFileTool struct {
	skillPath string
}

// NewReadSkillFileTool creates a new ReadSkillFileTool
func NewReadSkillFileTool(skillPath string) *ReadSkillFileTool {
	return &ReadSkillFileTool{skillPath: skillPath}
}

// Info returns the tool information
func (t *ReadSkillFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_skill_file",
		Desc: "Read the content of a file in the skill folder. Only files within the skill directory can be read.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The relative path of the file to read (relative to skill root directory)",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun executes the tool
func (t *ReadSkillFileTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		FilePath string `json:"file_path"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	// Security check: prevent path traversal
	cleanPath := filepath.Clean(args.FilePath)
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return "", fmt.Errorf("path traversal is not allowed")
	}

	rootInfo, rootErr := os.Stat(t.skillPath)
	if rootErr != nil {
		return "", fmt.Errorf("failed to stat skill path: %w", rootErr)
	}

	fullPath := ""
	if !rootInfo.IsDir() {
		baseName := filepath.Base(t.skillPath)
		if cleanPath != "." && cleanPath != baseName {
			return "", fmt.Errorf("file_path must be '%s' for file skill target", baseName)
		}
		fullPath = t.skillPath
	} else {
		fullPath = filepath.Join(t.skillPath, cleanPath)
	}

	// Verify the file is within the skill directory
	absSkillPath, err := filepath.Abs(t.skillPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve skill path: %w", err)
	}
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}
	if rootInfo.IsDir() && !strings.HasPrefix(absFullPath, absSkillPath) {
		return "", fmt.Errorf("file path is outside skill directory")
	}

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", args.FilePath)
		}
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("cannot read directory, use list_skill_files instead")
	}

	// Limit file size to prevent memory issues
	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large (max 5MB): %d bytes", info.Size())
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// GetSkillTools returns all tools for skill analysis
func GetSkillTools(skillPath string) []tool.BaseTool {
	return []tool.BaseTool{
		NewListSkillFilesTool(skillPath),
		NewReadSkillFileTool(skillPath),
	}
}
