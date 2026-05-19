package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PanelType represents the type of a message panel.
type PanelType int

const (
	PanelUser PanelType = iota
	PanelThought
	PanelAction
	PanelObservation
	PanelAnswer
	PanelError
)

// Panel represents a single message panel in the viewport.
type Panel struct {
	Type      PanelType
	Title     string
	Content   string
	Collapsed bool
}

// MessageView manages a list of panels.
type MessageView struct {
	panels []Panel
}

// NewMessageView creates a new empty MessageView.
func NewMessageView() *MessageView {
	return &MessageView{}
}

// AddPanel appends a new panel.
func (m *MessageView) AddPanel(p Panel) {
	m.panels = append(m.panels, p)
}

// LastPanel returns a pointer to the last panel, or nil if empty.
func (m *MessageView) LastPanel() *Panel {
	if len(m.panels) == 0 {
		return nil
	}
	return &m.panels[len(m.panels)-1]
}

// UpdateLastContent replaces the content of the last panel.
func (m *MessageView) UpdateLastContent(content string) {
	if len(m.panels) == 0 {
		return
	}
	m.panels[len(m.panels)-1].Content = content
}

// AppendToLast appends delta text to the last panel's content.
func (m *MessageView) AppendToLast(delta string) {
	if len(m.panels) == 0 {
		return
	}
	m.panels[len(m.panels)-1].Content += delta
}

// ToggleCollapse toggles the Collapsed state of a panel by index.
// No-op for User and Answer panels.
func (m *MessageView) ToggleCollapse(index int) {
	if index < 0 || index >= len(m.panels) {
		return
	}
	p := &m.panels[index]
	if p.Type == PanelUser || p.Type == PanelAnswer {
		return
	}
	p.Collapsed = !p.Collapsed
}

// PanelCount returns the number of panels.
func (m *MessageView) PanelCount() int {
	return len(m.panels)
}

// Clear removes all panels.
func (m *MessageView) Clear() {
	m.panels = nil
}

// Render renders all panels into a single string at the given width.
func (m *MessageView) Render(width int) string {
	if len(m.panels) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range m.panels {
		sb.WriteString(renderPanel(p, width))
	}
	return sb.String()
}

// renderPanel renders a single panel.
func renderPanel(p Panel, width int) string {
	switch p.Type {
	case PanelUser:
		return "\n  " + userStyle.Render("You:") + " " + p.Content + "\n"

	case PanelThought:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		header := thoughtStyle.Render(arrow+" Thought ") + dimStyle.Render(strings.Repeat("─", max(0, width-12)))
		if p.Collapsed {
			return "\n" + header + "\n"
		}
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelAction:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		title := p.Title
		if title == "" {
			title = "Action"
		}
		headerText := arrow + " Action: " + title + " "
		header := actionStyle.Render(headerText) + dimStyle.Render(strings.Repeat("─", max(0, width-lipgloss.Width(headerText)-2)))
		if p.Collapsed {
			return "\n" + header + "\n"
		}
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelObservation:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		header := observationStyle.Render(arrow+" Observation ") + dimStyle.Render(strings.Repeat("─", max(0, width-16)))
		if p.Collapsed {
			return "\n" + header + "\n"
		}
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelAnswer:
		return "\n  " + answerStyle.Render("Assistant:") + "\n    " + wrapText(p.Content, width-4, "    ") + "\n"

	case PanelError:
		return "\n  " + errorStyle.Render("Error: "+p.Content) + "\n"

	default:
		return ""
	}
}

// wrapText wraps text at the given width, indenting continuation lines.
func wrapText(text string, width int, indent string) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var result []string
	for i, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}
		// Wrap long lines
		for len(line) > width {
			result = append(result, line[:width])
			if i == 0 {
				line = indent + line[width:]
			} else {
				line = indent + line[width:]
			}
		}
		if len(line) > 0 {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// indent adds a prefix of n spaces to each line.
func indent(text string, n int) string {
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
