package protocol

import (
	"errors"
	"fmt"
)

type Code string

const (
	AdapterError         Code = "ADAPTER_ERROR"
	ApprovalError        Code = "APPROVAL_ERROR"
	InternalError        Code = "INTERNAL_ERROR"
	InvalidAdapterOutput Code = "INVALID_ADAPTER_OUTPUT"
	MissingSecret        Code = "MISSING_SECRET"
	RunNotFound          Code = "RUN_NOT_FOUND"
	StorageError         Code = "STORAGE_ERROR"
	ToolCatalogError     Code = "TOOL_CATALOG_ERROR"
	ToolChanged          Code = "TOOL_CHANGED"
	ToolNotFound         Code = "TOOL_NOT_FOUND"
	UsageError           Code = "USAGE_ERROR"
	ValidationError      Code = "VALIDATION_ERROR"
)

type Error struct {
	Code    Code
	Message string
}

func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var perr *Error
	if errors.As(err, &perr) && perr.Code == UsageError {
		return 2
	}
	return 1
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var perr *Error
	if errors.As(err, &perr) {
		return perr
	}
	return NewError(InternalError, err.Error())
}
