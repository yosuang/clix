package clix

import "fmt"

const (
	CodeAdapterError         = "ADAPTER_ERROR"
	CodeApprovalError        = "APPROVAL_ERROR"
	CodeInternalError        = "INTERNAL_ERROR"
	CodeInvalidAdapterOutput = "INVALID_ADAPTER_OUTPUT"
	CodeJQError              = "JQ_ERROR"
	CodeManifestChanged      = "MANIFEST_CHANGED"
	CodeManifestError        = "MANIFEST_ERROR"
	CodeMissingSecret        = "MISSING_SECRET"
	CodeRunNotFound          = "RUN_NOT_FOUND"
	CodeStorageError         = "STORAGE_ERROR"
	CodeToolNotFound         = "TOOL_NOT_FOUND"
	CodeUsageError           = "USAGE_ERROR"
	CodeValidationError      = "VALIDATION_ERROR"
)

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func newError(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func errorf(code, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}
