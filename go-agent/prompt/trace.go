package prompt

import (
	"fmt"
	"strings"
	"time"
)

type Trace struct {
	Timestamp     time.Time
	RulesLoaded   []string
	SkillsMatched []string
	Sources       []string
	TotalChars    int
}

func (t *Trace) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", t.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("RulesLoaded: [%s]\n", strings.Join(t.RulesLoaded, ", ")))
	sb.WriteString(fmt.Sprintf("SkillsMatched: [%s]\n", strings.Join(t.SkillsMatched, ", ")))
	sb.WriteString(fmt.Sprintf("Sources: [%s]\n", strings.Join(t.Sources, " -> ")))
	sb.WriteString(fmt.Sprintf("TotalChars: %d", t.TotalChars))
	return sb.String()
}
