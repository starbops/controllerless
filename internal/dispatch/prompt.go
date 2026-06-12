package dispatch

import (
	"fmt"
	"strings"

	"github.com/starbops/controllerless/internal/skill"
)

const harnessSystem = `You are an autonomous Kubernetes reconciler.
You will be given (a) a single triggering event and (b) one skill describing what to do.
Use only the provided tools to gather state and make changes.
Before acting, check current state — if the desired outcome is already true, call done().
When all work is finished, call done(). If you need to be retried later, call requeueAfter().
Tool errors are real — if a tool returns an error, do not retry the same tool with the same arguments.`

// buildPrompt assembles the full conversation seed: harness system + skill body + event YAML.
// Returns a single string that callers embed as the first user message.
func buildPrompt(s skill.Skill, eventYAML string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", harnessSystem)
	fmt.Fprintf(&b, "## Skill: %s\n\n%s\n\n", s.Frontmatter.Name, s.Body)
	fmt.Fprintf(&b, "## Triggering Event\n\n```yaml\n%s```\n", eventYAML)
	return b.String()
}
