// Package sync defines sync-run domain constants shared by handlers and repo.
package sync

const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusAborted   = "aborted"
)
