package agent

import "time"

// ToolProgress tracks a tool call's execution state.
type ToolProgress struct {
	Name    string
	Args    map[string]any
	Status  string // "pending"|"running"|"done"|"error"
	Elapsed time.Duration
}

// AgentCallbacks is an optional interface for observing agent execution progress.
// It is invoked at key points in the ReAct loop: thinking, tool calls, streaming, heartbeat.
// A nil implementation means no callbacks are invoked.
type AgentCallbacks interface {
	OnThinkingStart()
	OnThinkingEnd()
	OnToolStart(tool *ToolProgress)
	OnToolEnd(tool *ToolProgress, result string)
	OnStreamDelta(delta string)
	OnHeartbeat(elapsed time.Duration)
	OnMemorySuggestion(target, content, reason string)
}
