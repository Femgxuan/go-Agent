package rules

import (
	"os"
	"path/filepath"
)

// RuleSet holds a loaded rule file from a single source layer.
type RuleSet struct {
	Source  string // "user" | "project"
	Path    string
	Content string
}

// Loader discovers and loads RULES.md files from user and project directories.
type Loader struct {
	userDir    string
	projectDir string
}

// NewLoader creates a Loader that will look for RULES.md in userDir and projectDir.
func NewLoader(userDir, projectDir string) *Loader {
	return &Loader{
		userDir:    userDir,
		projectDir: projectDir,
	}
}

// Load reads RULES.md from each configured directory.
// It returns found rule sets in order: user first, then project.
// Missing directories or missing files are silently skipped.
func (l *Loader) Load() []RuleSet {
	var results []RuleSet

	type layer struct {
		dir    string
		source string
	}

	layers := []layer{
		{l.userDir, "user"},
		{l.projectDir, "project"},
	}

	for _, lyr := range layers {
		if lyr.dir == "" {
			continue
		}

		path := filepath.Join(lyr.dir, "RULES.md")
		data, err := os.ReadFile(path)
		if err != nil {
			// File doesn't exist or unreadable — skip silently.
			continue
		}

		results = append(results, RuleSet{
			Source:  lyr.source,
			Path:    path,
			Content: string(data),
		})
	}

	return results
}
