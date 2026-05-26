package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/commands"
	"github.com/fengxuan/go-agent/config"
)

// RuntimeInterface abstracts the runtime subsystem for the TUI.
type RuntimeInterface interface {
	RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent
	ExecuteCommand(input string) (commands.CommandResult, error)
	CurrentProvider() string
	CurrentModel() string
	CmdRegistry() *commands.Registry
}

// AppConfig holds the configuration for the TUI app.
type AppConfig struct {
	Runtime RuntimeInterface
	Display config.DisplayConfig
}

// agentEventMsg wraps an AgentEvent for the bubbletea message loop.
type agentEventMsg struct {
	event agent.AgentEvent
}

// agentDoneMsg signals the agent has finished.
type agentDoneMsg struct{}

// tickMsg is sent periodically to update elapsed time.
type tickMsg time.Time

// Model is the main bubbletea model for the TUI.
type Model struct {
	config      AppConfig
	viewport    viewport.Model
	textarea    textarea.Model
	spinner     spinner.Model
	messageView *MessageView
	completion  CompletionState

	state       AgentState
	startTime   time.Time
	elapsed     time.Duration
	width       int
	height      int
	agentCancel context.CancelFunc
	agentCh     <-chan agent.AgentEvent
	answerBuf   string
	ready       bool

	display       config.DisplayConfig
	seenTools     map[string]bool // for "new" mode: track tool names already shown
	shownToolIDs  map[string]bool // track which tool results to show
}

// NewModel creates a new TUI Model with the given config.
func NewModel(config AppConfig) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	var cs CompletionState
	if config.Runtime != nil {
		cs = NewCompletionState(config.Runtime.CmdRegistry().List())
	}

	return Model{
		config:        config,
		spinner:       s,
		messageView:   NewMessageView(),
		state:         StateReady,
		completion:    cs,
		display:       config.Display,
		seenTools:     make(map[string]bool),
		shownToolIDs:  make(map[string]bool),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textarea.Blink,
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Initialize viewport: total - statusbar(1) - input(3) - 2 padding lines
			vpHeight := m.height - 1 - 5 - 2
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent("")

			m.textarea = newInputArea(m.width)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			vpHeight := m.height - 1 - 5 - 2
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport.Height = vpHeight
			m.textarea.SetWidth(m.width)
		}

	case tea.KeyMsg:
		// When completion is active, route key events through completion handler first
		if m.completion.Active {
			cmd := m.handleCompletionKey(msg)
			return m, cmd
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.state != StateReady {
				// Cancel running agent
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.messageView.AddPanel(Panel{
					Type:    PanelError,
					Content: "Interrupted by user",
				})
				m.state = StateReady
				m.elapsed = 0
				m.answerBuf = ""
				m.textarea.Focus()
				m.syncViewport()
				return m, textarea.Blink
			}
			return m, tea.Quit

		case tea.KeyEsc:
			if m.state != StateReady {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.state = StateReady
				m.elapsed = 0
				m.answerBuf = ""
				m.textarea.Focus()
				m.syncViewport()
				return m, textarea.Blink
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state != StateReady {
				break
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				break
			}
			m.textarea.Reset()
			m.completion.Deactivate()

			// Handle slash commands
			if strings.HasPrefix(input, "/") {
				cmd := m.handleSlashCommand(input)
				if cmd != nil {
					return m, cmd
				}
				break
			}

			// Start agent run
			m.messageView.AddPanel(Panel{
				Type:    PanelUser,
				Content: input,
			})
			m.state = StateThinking
			m.startTime = time.Now()
			m.elapsed = 0
			m.answerBuf = ""

			ctx, cancel := context.WithCancel(context.Background())
			m.agentCancel = cancel
			m.agentCh = m.config.Runtime.RunUserInput(ctx, input)

			m.syncViewport()
			return m, tea.Batch(waitForEvent(m.agentCh), tickCmd())
		}

		// Pass key events to textarea when ready
		if m.state == StateReady {
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			cmds = append(cmds, taCmd)
			m.checkCompletion()
		}

	case agentEventMsg:
		cmds = append(cmds, m.handleAgentEvent(msg.event))

	case agentDoneMsg:
		if m.agentCancel != nil {
			m.agentCancel()
			m.agentCancel = nil
		}
		m.state = StateReady
		m.answerBuf = ""
		m.textarea.Focus()
		m.syncViewport()
		cmds = append(cmds, textarea.Blink)

	case tickMsg:
		if m.state != StateReady {
			m.elapsed = time.Since(m.startTime)
			cmds = append(cmds, tickCmd())
		}

	case spinner.TickMsg:
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		cmds = append(cmds, spinCmd)
	}

	// Update viewport scroll
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// checkCompletion checks the current textarea value and activates/updates/deactivates completion.
func (m *Model) checkCompletion() {
	val := m.textarea.Value()
	if strings.HasPrefix(val, "/") {
		if !m.completion.Active {
			m.completion.Activate()
		}
		m.completion.UpdatePrefix(val)
	} else {
		if m.completion.Active {
			m.completion.Deactivate()
		}
	}
}

// handleCompletionKey handles key events when the completion panel is active.
func (m *Model) handleCompletionKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyUp:
		m.completion.MoveUp()
		return nil

	case tea.KeyDown:
		m.completion.MoveDown()
		return nil

	case tea.KeyTab:
		// Accept selected item: fill textarea with /name
		if item, ok := m.completion.SelectedItem(); ok {
			// Reset textarea and set to the completed value
			m.textarea.Reset()
			// Insert the command name with slash
			insertVal := "/" + item.Name + " "
			for _, ch := range insertVal {
				m.textarea.InsertRune(ch)
			}
			m.completion.UpdatePrefix("/" + item.Name)
		}
		return nil

	case tea.KeyEnter:
		// Accept and submit if an item is selected
		if item, ok := m.completion.SelectedItem(); ok {
			m.textarea.Reset()
			insertVal := "/" + item.Name
			for _, ch := range insertVal {
				m.textarea.InsertRune(ch)
			}
		}
		m.completion.Deactivate()

		// Now submit the command
		input := strings.TrimSpace(m.textarea.Value())
		if input == "" {
			return nil
		}
		m.textarea.Reset()
		if strings.HasPrefix(input, "/") {
			return m.handleSlashCommand(input)
		}
		return nil

	case tea.KeyEsc:
		m.completion.Deactivate()
		return nil

	default:
		// Pass through to textarea, then re-check completion
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		m.checkCompletion()
		return taCmd
	}
}

// shouldShowTool determines whether to display a tool call based on verbosity level.
func (m *Model) shouldShowTool(name string) bool {
	switch m.display.ToolProgress {
	case "off":
		return false
	case "new":
		if m.seenTools[name] {
			return false
		}
		m.seenTools[name] = true
		return true
	default: // "all", "verbose"
		return true
	}
}

// truncateToolOutput truncates tool params/result based on display config.
func (m *Model) truncateToolOutput(s string) string {
	if m.display.ToolProgress == "verbose" || m.display.ToolPreviewLength == 0 {
		return s
	}
	// Collapse to single line, truncate
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > m.display.ToolPreviewLength {
		s = s[:m.display.ToolPreviewLength-3] + "..."
	}
	return s
}

// handleAgentEvent processes a single agent event and returns the next command.
func (m *Model) handleAgentEvent(ev agent.AgentEvent) tea.Cmd {
	switch ev.Type {
	case agent.EventDelta:
		if m.answerBuf == "" {
			m.messageView.AddPanel(Panel{
				Type:    PanelAnswer,
				Content: ev.Content,
			})
		} else {
			m.messageView.AppendToLast(ev.Content)
		}
		m.answerBuf += ev.Content

	case agent.EventThought:
		m.state = StateThinking
		if m.display.ShowThinking {
			m.messageView.AddPanel(Panel{
				Type:    PanelThought,
				Title:   "Thought",
				Content: ev.Content,
			})
		}

	case agent.EventToolCall:
		if !m.shouldShowTool(ev.ToolName) {
			m.state = StateExecuting
			m.syncViewport()
			return waitForEvent(m.agentCh)
		}
		m.state = StateExecuting
		m.shownToolIDs[ev.ToolID] = true
		params := m.truncateToolOutput(formatParams(ev.Params))
		m.messageView.AddPanel(Panel{
			Type:    PanelAction,
			Title:   ev.ToolName,
			Content: params,
		})

	case agent.EventToolResult:
		if !m.shownToolIDs[ev.ToolID] {
			m.syncViewport()
			return waitForEvent(m.agentCh)
		}
		m.state = StateThinking
		content := m.truncateToolOutput(ev.Content)
		m.messageView.AddPanel(Panel{
			Type:    PanelObservation,
			Content: content,
		})

	case agent.EventAnswer:
		if m.answerBuf == "" {
			m.messageView.AddPanel(Panel{
				Type:    PanelAnswer,
				Content: ev.Content,
			})
		}

	case agent.EventError:
		m.messageView.AddPanel(Panel{
			Type:    PanelError,
			Content: ev.Content,
		})

	case agent.EventPromptTrace:
		// Silently ignore — trace is viewable via /prompt command
	}

	m.syncViewport()
	return waitForEvent(m.agentCh)
}

// handleSlashCommand processes a slash command input by delegating to the Runtime.
func (m *Model) handleSlashCommand(input string) tea.Cmd {
	if m.config.Runtime == nil {
		return nil
	}

	result, err := m.config.Runtime.ExecuteCommand(input)
	if err != nil {
		m.messageView.AddPanel(Panel{
			Type:    PanelError,
			Content: fmt.Sprintf("Command error: %v", err),
		})
		m.syncViewport()
		return nil
	}

	if result.Message != "" {
		m.messageView.AddPanel(Panel{
			Type:    PanelAnswer,
			Content: result.Message,
		})
		m.syncViewport()
	}

	// Refresh completion in case commands were added/removed
	m.RefreshCompletion()

	switch result.Action {
	case commands.ActionQuit:
		return tea.Quit
	case commands.ActionClearScreen:
		m.messageView.Clear()
		m.syncViewport()
	case commands.ActionRunAgent:
		m.messageView.AddPanel(Panel{
			Type:    PanelUser,
			Content: result.Input,
		})
		m.state = StateThinking
		m.startTime = time.Now()
		m.elapsed = 0
		m.answerBuf = ""

		ctx, cancel := context.WithCancel(context.Background())
		m.agentCancel = cancel
		m.agentCh = m.config.Runtime.RunUserInput(ctx, result.Input)
		m.syncViewport()
		return tea.Batch(waitForEvent(m.agentCh), tickCmd())
	}

	return nil
}
	// RefreshCompletion rebuilds the completion state from the current command registry.
	// Call after skill CRUD operations to update autocomplete.
	func (m *Model) RefreshCompletion() {
		if m.config.Runtime != nil {
			m.completion = NewCompletionState(m.config.Runtime.CmdRegistry().List())
		}
	}

// syncViewport updates viewport content and scrolls to bottom.
func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	content := m.messageView.Render(m.width)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	provider := ""
	model := ""
	if m.config.Runtime != nil {
		provider = m.config.Runtime.CurrentProvider()
		model = m.config.Runtime.CurrentModel()
	}

	statusBar := renderStatusBar(m.width, provider, model, m.state, m.elapsed)

	vpView := m.viewport.View()

	// Bordered textarea
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Width(m.width - 2)
	inputView := borderStyle.Render(m.textarea.View())

	// Insert completion panel between viewport and input when active
	if m.completion.Active && len(m.completion.Filtered) > 0 {
		completionView := renderCompletion(m.completion, m.width)
		return statusBar + "\n" + vpView + "\n" + completionView + "\n" + inputView
	}

	return statusBar + "\n" + vpView + "\n" + inputView
}

// waitForEvent returns a tea.Cmd that reads the next event from the agent channel.
func waitForEvent(ch <-chan agent.AgentEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg{event: ev}
	}
}

// tickCmd returns a tea.Cmd that sends a tickMsg after 200ms.
func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// formatParams formats tool call params as indented JSON.
func formatParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	b, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", params)
	}
	return string(b)
}
