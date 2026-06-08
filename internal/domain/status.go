package domain

type Status string

const (
	StatusCreated         Status = "created"
	StatusPendingApproval Status = "pending_approval"
	StatusRunning         Status = "running"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
	StatusRejected        Status = "rejected"
)

func (s Status) Terminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusRejected
}
