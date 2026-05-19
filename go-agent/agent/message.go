package agent

type EventType int

const (
	EventThought     EventType = iota
	EventToolCall
	EventToolResult
	EventAnswer
	EventError
	EventDelta
	EventPromptTrace
)

func (e EventType) String() string {
	switch e {
	case EventThought:
		return "Thought"
	case EventToolCall:
		return "Action"
	case EventToolResult:
		return "Observation"
	case EventAnswer:
		return "Answer"
	case EventError:
		return "Error"
	case EventDelta:
		return "Delta"
	case EventPromptTrace:
		return "PromptTrace"
	default:
		return "Unknown"
	}
}

type AgentEvent struct {
	Type     EventType
	Content  string
	ToolName string
	ToolID   string
	Params   map[string]any
}
