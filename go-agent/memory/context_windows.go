package memory

// ModelContextWindows maps model IDs to their context window sizes (in tokens).
var ModelContextWindows = map[string]int{
	"deepseek-chat":     65536,
	"deepseek-coder":    65536,
	"deepseek-reasoner": 65536,
	"gpt-4o":            128000,
	"gpt-4-turbo":       128000,
	"gpt-4":             8192,
	"gpt-3.5-turbo":     16385,
	"claude-opus-4-7":   200000,
	"claude-sonnet-4-6": 200000,
	"claude-haiku-4-5":  200000,
}

// DefaultContextWindow is the fallback when model is unknown.
const DefaultContextWindow = 8192

// GetContextWindow returns the context window size for a model.
// Priority: configOverride > ModelContextWindows lookup > DefaultContextWindow.
func GetContextWindow(model string, configOverride int) int {
	if configOverride > 0 {
		return configOverride
	}
	if size, ok := ModelContextWindows[model]; ok {
		return size
	}
	return DefaultContextWindow
}
