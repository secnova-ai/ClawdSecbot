package proxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go"
)

type captureSecurityDetector struct {
	requests  []securityDetectionRequest
	responses []securityDetectionResponse
}

func (d *captureSecurityDetector) Name() string {
	return "capture"
}

func (d *captureSecurityDetector) Detect(_ context.Context, req securityDetectionRequest) (*securityDetectionResponse, error) {
	d.requests = append(d.requests, req)
	if len(d.responses) > 0 {
		resp := d.responses[0]
		d.responses = d.responses[1:]
		return &resp, nil
	}
	allowed := true
	return &securityDetectionResponse{Allowed: &allowed, Reason: "allowed by test detector"}, nil
}

func TestExtractRawResponseToolPayloadOpenAIMCP(t *testing.T) {
	raw := `{
	  "id":"resp_1",
	  "object":"response",
	  "output":[
	    {
	      "type":"mcp_call",
	      "id":"mcp_call_123",
	      "server_label":"github",
	      "name":"create_issue",
	      "arguments":"{\"repo\":\"org/repo\",\"title\":\"demo\"}",
	      "output":"created issue #1",
	      "status":"completed"
	    }
	  ]
	}`

	got := extractRawResponseToolPayload(raw)
	if len(got.Calls) != 1 {
		t.Fatalf("expected one MCP call, got %+v", got.Calls)
	}
	call := got.Calls[0]
	if call.ToolCallID != "mcp:github:mcp_call_123" {
		t.Fatalf("unexpected normalized tool_call_id: %q", call.ToolCallID)
	}
	if call.OriginalToolCallID != "mcp_call_123" || call.Protocol != "mcp" || call.ServerLabel != "github" {
		t.Fatalf("unexpected MCP metadata: %+v", call)
	}
	if call.Name != "create_issue" || call.Arguments["repo"] != "org/repo" {
		t.Fatalf("unexpected MCP call content: %+v", call)
	}
	if len(got.Results) != 1 || got.Results[0].ToolCallID != call.ToolCallID || got.Results[0].Content != "created issue #1" {
		t.Fatalf("unexpected MCP result: %+v", got.Results)
	}
}

func TestExtractRawResponseToolPayloadOpenAIMCPApprovalRequest(t *testing.T) {
	raw := `{
	  "output":[
	    {
	      "type":"mcp_approval_request",
	      "id":"approval_123",
	      "server_label":"filesystem",
	      "name":"write_file",
	      "arguments":"{\"path\":\"/tmp/a\"}"
	    }
	  ]
	}`

	got := extractRawResponseToolPayload(raw)
	if len(got.Calls) != 1 {
		t.Fatalf("expected one MCP approval call, got %+v", got.Calls)
	}
	if got.Calls[0].ToolCallID != "mcp:filesystem:approval_123" || got.Calls[0].Name != "write_file" {
		t.Fatalf("unexpected approval request call: %+v", got.Calls[0])
	}
	if len(got.Results) != 0 {
		t.Fatalf("did not expect approval request result, got %+v", got.Results)
	}
}

func TestOnResponseRunsPolicyForOpenAIMCPRawResponse(t *testing.T) {
	raw := []byte(`{
	  "id":"resp_1",
	  "object":"response",
	  "model":"gpt-test",
	  "output":[
	    {
	      "type":"mcp_call",
	      "id":"mcp_call_abc",
	      "server_label":"github",
	      "name":"create_issue",
	      "arguments":"{\"repo\":\"org/repo\"}",
	      "output":"created",
	      "status":"completed"
	    }
	  ]
	}`)
	var resp openai.ChatCompletion
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	detector := &captureSecurityDetector{}
	pp := &ProxyProtection{
		records:          NewRecordStore(),
		assetName:        "openclaw",
		assetID:          "openclaw:test",
		securityDetector: detector,
	}
	ctx := context.Background()
	pp.bindRequestContext(ctx, "req_mcp")

	if !pp.onResponse(ctx, &resp) {
		t.Fatalf("expected MCP response to pass")
	}
	if len(detector.requests) != 2 {
		t.Fatalf("expected tool_call and tool_call_result checks, got %+v", detector.requests)
	}
	if detector.requests[0].Stage != hookStageToolCall || len(detector.requests[0].ToolCalls) != 1 {
		t.Fatalf("expected tool_call stage with one MCP call, got %+v", detector.requests[0])
	}
	call := detector.requests[0].ToolCalls[0]
	if call.ToolCallID != "mcp:github:mcp_call_abc" || call.Protocol != "mcp" || call.ServerLabel != "github" {
		t.Fatalf("unexpected MCP call passed to detector: %+v", call)
	}
	if detector.requests[1].Stage != hookStageToolCallResult || len(detector.requests[1].ToolResults) != 1 {
		t.Fatalf("expected tool_call_result stage with one MCP result, got %+v", detector.requests[1])
	}
	if detector.requests[1].ToolResults[0].ToolCallID != "mcp:github:mcp_call_abc" {
		t.Fatalf("unexpected MCP result passed to detector: %+v", detector.requests[1].ToolResults[0])
	}
}
