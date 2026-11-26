package catalog_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/p3bot/x/skill/catalog"
)

// The catalog is frozen against agentdex's agents.cue; a change to this set
// is a catalog edit, not a refactor.
var wantIDs = []string{
	"agy", "aider", "amp", "augment", "claude-code", "cline", "codewhale",
	"codex", "copilot", "crush", "forge", "goose", "grok", "hermes", "kilo",
	"kimi-code", "kiro", "open-interpreter", "openclaw", "opencode",
	"openhands", "pi", "plandex", "qwen-code",
}

func TestAgentsIDs(t *testing.T) {
	var got []string
	for _, a := range catalog.Agents() {
		got = append(got, a.ID)
	}
	if !slices.Equal(got, wantIDs) {
		t.Errorf("Agents() IDs = %q, want %q", got, wantIDs)
	}
}

func TestLookup(t *testing.T) {
	got, ok := catalog.Lookup("claude-code")
	if !ok {
		t.Fatal(`Lookup("claude-code") found nothing`)
	}
	want := catalog.Agent{
		ID:          "claude-code",
		Name:        "Claude Code",
		Bin:         "claude",
		Description: "Anthropic's agentic coding tool that runs in the terminal.",
		Homepage:    "https://github.com/anthropics/claude-code",
		Config:      catalog.Config{Global: "~/.claude", Local: ".claude"},
		Skills: catalog.Skills{
			Global: catalog.Scope{Native: "~/.claude/skills"},
			Local:  catalog.Scope{Native: ".claude/skills"},
		},
		Provider: []string{"anthropic"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lookup(\"claude-code\") = %+v, want %+v", got, want)
	}

	agnostic, ok := catalog.Lookup("aider")
	if !ok || !agnostic.Agnostic || agnostic.Provider != nil {
		t.Errorf("Lookup(\"aider\") = %+v, %v; want an agnostic agent with no provider", agnostic, ok)
	}

	if a, ok := catalog.Lookup("no-such-agent"); ok {
		t.Errorf("Lookup(\"no-such-agent\") = %+v, true; want false", a)
	}
}

func TestAgentsReturnsCopies(t *testing.T) {
	first := catalog.Agents()
	agy := first[slices.IndexFunc(first, func(a catalog.Agent) bool { return a.ID == "agy" })]
	agy.Provider[0] = "mutated"
	agy.Skills.Local.Alternatives[0] = "mutated"

	again, _ := catalog.Lookup("agy")
	if again.Provider[0] == "mutated" || again.Skills.Local.Alternatives[0] == "mutated" {
		t.Errorf("mutating an Agents() result changed the catalog: %+v", again)
	}
}

func TestScopePrimary(t *testing.T) {
	tests := []struct {
		name  string
		scope catalog.Scope
		want  string
	}{
		{"zero", catalog.Scope{}, ""},
		{"agents wins", catalog.Scope{Agents: "a", Native: "n", Alternatives: []string{"x"}}, "a"},
		{"native without agents", catalog.Scope{Native: "n", Alternatives: []string{"x"}}, "n"},
		{"alternatives only", catalog.Scope{Alternatives: []string{"x", "y"}}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scope.Primary(); got != tt.want {
				t.Errorf("Primary() = %q, want %q", got, tt.want)
			}
		})
	}
}
