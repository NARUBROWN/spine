package hook

import "github.com/NARUBROWN/spine/internal/execution"

// PostExecutionHook runs after the controller and response preparation succeed.
type PostExecutionHook interface {
	AfterExecution(*execution.Context) error
}
