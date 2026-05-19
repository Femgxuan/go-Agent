package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// AgentState represents the current state of the agent.
type AgentState int

const (
	StateReady AgentState = iota
	StateThinking
	StateExecuting
)

// String returns the display string for an AgentState.
func (s AgentState) String() string {
	switch s {
	case StateReady:
		return "Ready"
	case StateThinking:
		return "Thinking"
	case StateExecuting:
		return "Executing"
	default:
		return "Unknown"
	}
}

// stateIcon returns the icon prefix for an AgentState.
func stateIcon(s AgentState) string {
	switch s {
	case StateReady:
		return "● "
	case StateThinking:
		return "◐ "
	case StateExecuting:
		return "⚡ "
	default:
		return "● "
	}
}

// stateStyle returns the lipgloss style for an AgentState.
func stateStyle(s AgentState) lipgloss.Style {
	switch s {
	case StateReady:
		return statusReadyStyle
	case StateThinking:
		return statusThinkingStyle
	case StateExecuting:
		return statusExecutingStyle
	default:
		return statusReadyStyle
	}
}

// renderStatusBar renders the full-width status bar string.
func renderStatusBar(width int, provider, model string, state AgentState, elapsed time.Duration) string {
	// Left section: app name + model info
	appName := appNameStyle.Render("Go-Agent v0.1")
	modelInfo := dimStyle.Render(fmt.Sprintf("│  %s/%s", provider, model))
	left := appName + "  " + modelInfo

	// Right section: state indicator + elapsed time
	stateText := stateStyle(state).Render(stateIcon(state) + state.String())
	elapsedText := dimStyle.Render(fmt.Sprintf("⏱ %.1fs", elapsed.Seconds()))
	right := stateText + "  " + elapsedText

	// Calculate visible lengths (strip ANSI for width calculation)
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)

	// Fill gap with spaces
	gap := width - leftLen - rightLen - 2 // 2 for padding(0,1) each side
	if gap < 1 {
		gap = 1
	}
	middle := strings.Repeat(" ", gap)

	content := left + middle + right
	return statusBarStyle.Width(width).Render(content)
}
