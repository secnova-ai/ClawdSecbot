// Package api provides HTTP API server functionality for external integrations.
package api

import (
	"encoding/json"
	"net/http"
)

// Error codes
const (
	CodeSuccess        = 0    // 成功
	CodeAuthFailed     = 1001 // 认证失败
	CodeInvalidParam   = 1002 // 参数错误
	CodeNotFound       = 1003 // 资源未找到
	CodeInternalError  = 1004 // 内部错误
	CodeNotImplemented = 1005 // 功能未实现
)

// APIResponse represents a unified JSON response structure
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success sends a successful JSON response with data
func Success(w http.ResponseWriter, data interface{}) {
	resp := APIResponse{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	}
	writeJSON(w, http.StatusOK, resp)
}

// Error sends an error JSON response
func Error(w http.ResponseWriter, httpStatus int, code int, message string) {
	resp := APIResponse{
		Code:    code,
		Message: message,
	}
	writeJSON(w, httpStatus, resp)
}

// writeJSON marshals the response and writes it to the http.ResponseWriter
func writeJSON(w http.ResponseWriter, httpStatus int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Fallback: write a minimal error response if encoding fails
		http.Error(w, `{"code":1004,"message":"internal encoding error"}`, http.StatusInternalServerError)
	}
}
