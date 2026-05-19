package tui

import "github.com/charmbracelet/lipgloss"

var (
	appNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EEEEEE")).
			Background(lipgloss.Color("#333333")).
			Padding(0, 1)

	statusReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E")).
				Bold(true)

	statusThinkingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EAB308")).
				Bold(true)

	statusExecutingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#3B82F6")).
				Bold(true)

	thoughtStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	actionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24")).
			Bold(true)

	observationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#34D399")).
				Bold(true)

	answerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9FAFB"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C3AED")).
				Bold(true)

	completionBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7C3AED")).
				Padding(0, 1)

	completionItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5E7EB"))

	completionSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7C3AED")).
				Bold(true)

	completionDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF"))
)
