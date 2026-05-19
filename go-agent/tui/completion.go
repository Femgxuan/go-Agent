package tui

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

// CompletionItem represents a single completable command entry.
type CompletionItem struct {
	Name        string
	Aliases     string
	Description string
	HasArgs     bool
}

// CompletionState holds all state for the completion panel.
type CompletionState struct {
	Active   bool
	Items    []CompletionItem
	Filtered []CompletionItem
	Selected int
	Prefix   string
}

// NewCompletionState builds a CompletionState from a slice of CommandInfo.
func NewCompletionState(cmdInfos []commands.CommandInfo) CompletionState {
	items := make([]CompletionItem, 0, len(cmdInfos))
	for _, info := range cmdInfos {
		aliases := strings.Join(info.Aliases, ", ")
		items = append(items, CompletionItem{
			Name:        info.Name,
			Aliases:     aliases,
			Description: info.Description,
			HasArgs:     info.HasArgs,
		})
	}
	cs := CompletionState{Items: items}
	cs.filter()
	return cs
}

// Activate enables the completion panel and resets state.
func (cs *CompletionState) Activate() {
	cs.Active = true
	cs.Selected = 0
	cs.Prefix = ""
	cs.filter()
}

// Deactivate disables the completion panel and resets all state.
func (cs *CompletionState) Deactivate() {
	cs.Active = false
	cs.Selected = 0
	cs.Prefix = ""
	cs.Filtered = nil
}

// UpdatePrefix sets the current prefix, resets selected, and refilters.
func (cs *CompletionState) UpdatePrefix(prefix string) {
	cs.Prefix = prefix
	cs.Selected = 0
	cs.filter()
}

// MoveUp moves the selection up by one (clamped at 0).
func (cs *CompletionState) MoveUp() {
	if cs.Selected > 0 {
		cs.Selected--
	}
}

// MoveDown moves the selection down by one (clamped at len-1).
func (cs *CompletionState) MoveDown() {
	if cs.Selected < len(cs.Filtered)-1 {
		cs.Selected++
	}
}

// SelectedItem returns the currently selected CompletionItem, or false if none.
func (cs *CompletionState) SelectedItem() (CompletionItem, bool) {
	if !cs.Active || len(cs.Filtered) == 0 {
		return CompletionItem{}, false
	}
	if cs.Selected < 0 || cs.Selected >= len(cs.Filtered) {
		return CompletionItem{}, false
	}
	return cs.Filtered[cs.Selected], true
}

// filter rebuilds Filtered based on the current Prefix (case-insensitive prefix match on name and aliases).
func (cs *CompletionState) filter() {
	lower := strings.ToLower(cs.Prefix)
	// Strip leading slash for matching
	matchPrefix := strings.TrimPrefix(lower, "/")

	cs.Filtered = cs.Filtered[:0]
	for _, item := range cs.Items {
		name := strings.ToLower(item.Name)
		aliases := strings.ToLower(item.Aliases)
		if matchPrefix == "" || strings.HasPrefix(name, matchPrefix) || strings.Contains(aliases, matchPrefix) {
			cs.Filtered = append(cs.Filtered, item)
		}
	}
}

const maxVisibleItems = 8

// renderCompletion renders the completion panel as a string with lipgloss styling.
func renderCompletion(cs CompletionState, width int) string {
	if !cs.Active || len(cs.Filtered) == 0 {
		return ""
	}

	items := cs.Filtered
	if len(items) > maxVisibleItems {
		items = items[:maxVisibleItems]
	}

	// Determine the widest name for alignment
	maxNameLen := 0
	for _, item := range items {
		if len(item.Name) > maxNameLen {
			maxNameLen = len(item.Name)
		}
	}

	var rows []string
	for i, item := range items {
		nameStr := fmt.Sprintf("/%-*s", maxNameLen, item.Name)
		desc := item.Description

		if i == cs.Selected {
			nameRendered := completionSelectedStyle.Render(nameStr)
			descRendered := completionSelectedStyle.Render("  " + desc)
			rows = append(rows, nameRendered+descRendered)
		} else {
			nameRendered := completionItemStyle.Render(nameStr)
			descRendered := completionDescStyle.Render("  " + desc)
			rows = append(rows, nameRendered+descRendered)
		}
	}

	inner := strings.Join(rows, "\n")

	// Constrain panel width to terminal width minus borders/padding
	panelWidth := width - 4
	if panelWidth < 20 {
		panelWidth = 20
	}

	return completionBorderStyle.Width(panelWidth).Render(inner)
}
