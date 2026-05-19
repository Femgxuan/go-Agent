package commands

type CommandAction int

const (
	ActionNone CommandAction = iota
	ActionQuit
	ActionClearScreen
	ActionRunAgent
)

type CommandContext struct {
	Args    []string
	Output  func(string)
	Runtime RuntimeAccessor
}

type CommandResult struct {
	Message string
	Action  CommandAction
	Input   string // used with ActionRunAgent: the text to send to the agent
}

type Command interface {
	Name() string
	Aliases() []string
	Description() string
	Usage() string
	Execute(ctx CommandContext) (CommandResult, error)
}

type RuntimeAccessor interface {
	ListSkills() []SkillInfo
	ActivateSkill(name string) error
	ReloadSkillsAndRules() error
	LastPromptTrace() string
	ListTools() []ToolInfo
	SwitchProvider(name string) error
	SwitchModel(name string) error
	CurrentProvider() string
	CurrentModel() string
	AvailableProviders() []string
	ClearHistory()
	CreateSkill(req CreateSkillRequest) (CreateSkillResult, error)
	DeleteSkill(name string) error
	UpdateSkill(name string, req CreateSkillRequest) (CreateSkillResult, error)
}

type SkillInfo struct {
	Name        string
	Description string
	Source      string
	Priority    int
	Tags        []string
}

type ToolInfo struct {
	Name        string
	Description string
}

type CreateSkillRequest struct {
	Name         string
	Description  string
	Tags         []string
	Keywords     []string
	Patterns     []string
	Priority     int
	Body         string
	ProjectLevel bool // if true, create in project dir instead of user dir
}

type CreateSkillResult struct {
	Name    string
	Path    string
	Message string
}
