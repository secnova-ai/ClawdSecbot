package modelfactory

import (
	"context"
	"encoding/json"
	"fmt"

	"go_lib/core/repository"

	"github.com/cloudwego/eino/schema"
)

// TestConnectionResponse represents the response from testing model connection
type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// TestModelConnectionInternal tests the model connection with a simple prompt
func TestModelConnectionInternal(configJSON string) string {
	var config repository.SecurityModelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid JSON: %v", err),
		})
	}

	if err := ValidateSecurityModelConfig(&config); err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
	}

	ctx := context.Background()

	chatModel, err := CreateChatModelFromConfig(ctx, &config)
	if err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create model: %v", err),
		})
	}

	testMessages := []*schema.Message{
		schema.UserMessage("Hi, respond with just 'OK' to confirm connection."),
	}

	_, err = chatModel.Generate(ctx, testMessages)
	if err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("Connection test failed: %v", err),
		})
	}

	return toJSONString(TestConnectionResponse{
		Success: true,
		Message: "Connection successful",
	})
}

// toJSONString marshals a value to JSON string
func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(b)
}
