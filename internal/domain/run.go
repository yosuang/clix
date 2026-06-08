package domain

import "time"

type Run struct {
	ID              string
	ToolName        string
	Effect          Effect
	ToolFingerprint string
	ToolSourcePath  string
	InputJSON       []byte
	Status          Status
	RequestedAt     time.Time
	ApprovedAt      *time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	ExitCode        *int
	ErrorCode       *string
	ErrorMessage    *string
}

func (r Run) PublicMap() map[string]any {
	result := map[string]any{
		"id":           r.ID,
		"tool_name":    r.ToolName,
		"effect":       string(r.Effect),
		"status":       string(r.Status),
		"requested_at": r.RequestedAt.Format(time.RFC3339Nano),
	}
	if r.ErrorCode != nil {
		result["error_code"] = *r.ErrorCode
	}
	if r.ErrorMessage != nil {
		result["error_message"] = *r.ErrorMessage
	}
	return result
}
