package prompt

const (
	TypeSystem = "system"
	TypeRules  = "rules"
	TypeSkill  = "skill"
	TypeUser   = "user"
)

type Source struct {
	Name    string
	Type    string
	Content string
}
