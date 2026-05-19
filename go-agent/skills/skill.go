package skills

type TriggerConfig struct {
	Keywords []string `yaml:"keywords"`
	Patterns []string `yaml:"patterns"`
}

type ScriptRef struct {
	Name        string `yaml:"name"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type InputDef struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

type OutputDef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type Skill struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Version     string        `yaml:"version"`
	Tags        []string      `yaml:"tags"`
	Triggers    TriggerConfig `yaml:"triggers"`
	Scope       string        `yaml:"scope"`
	Priority    int           `yaml:"priority"`
	Tools       []string      `yaml:"tools"`
	Scripts     []ScriptRef   `yaml:"scripts"`
	Inputs      []InputDef    `yaml:"inputs"`
	Outputs     []OutputDef   `yaml:"outputs"`
	Body        string        `yaml:"-"`
	Source      string        `yaml:"-"`
	BasePath    string        `yaml:"-"`
}
