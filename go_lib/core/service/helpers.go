package service

// ========== Response formatting helpers ==========

// successResult returns a success response without payload.
func successResult() map[string]interface{} {
	return map[string]interface{}{
		"success": true,
	}
}

// successDataResult returns a success response with payload.
func successDataResult(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"data":    data,
	}
}

// errorResult returns an error response from an error value.
func errorResult(err error) map[string]interface{} {
	return map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	}
}

// errorMessageResult returns an error response from a raw message.
func errorMessageResult(msg string) map[string]interface{} {
	return map[string]interface{}{
		"success": false,
		"error":   msg,
	}
}
