package tui

import "github.com/charmbracelet/bubbles/textarea"

// newInputArea creates and configures a textarea for user input.
func newInputArea(width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Ask something... (Ctrl+C cancel | ESC quit)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096
	ta.SetHeight(1)
	ta.SetWidth(width - 4) // account for border (2) + prompt "> " (2)
	ta.SetPromptFunc(1, func(line int) string {
		return inputPromptStyle.Render("> ")
	})
	ta.Focus()
	return ta
}
