package execution

// Input holds the HTTP contracts borrowed for one pipeline execution.
// Internal pipeline consumers access these fields directly. Public views use
// the execution owner's read lock, and Context.Close detaches the contracts.
type Input struct {
	Routing      RoutingContext
	HTTPRequest  HTTPRequestContext
	HTTPResponse HTTPResponseContext
}
