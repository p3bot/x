package catalog

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	if _, err := parse(agentsJSON); err != nil {
		t.Fatalf("agents.json: %v", err)
	}
}

// TestParseRejects edits a copy of the embedded catalog one rule at a time,
// covering every rule the agentdex CUE schema enforced.
func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(agents map[string]any)
		want []string // substrings of the error, the agent id first
	}{
		{"unknown agent field", func(m map[string]any) { agent(m, "amp")["version"] = "1" },
			[]string{"amp", `"version"`}},
		{"unknown nested field", func(m map[string]any) { obj(agent(m, "amp"), "config")["extra"] = "x" },
			[]string{"amp", `"extra"`}},
		{"field name in the wrong case", func(m map[string]any) {
			a := agent(m, "amp")
			a["Bin"] = a["bin"]
			delete(a, "bin")
		}, []string{"amp", `"Bin"`}},
		{"wrong field type", func(m map[string]any) { agent(m, "amp")["agnostic"] = "no" },
			[]string{"amp", "agnostic"}},
		{"null name", func(m map[string]any) { agent(m, "amp")["name"] = nil },
			[]string{`null at "/agents/amp/name"`}},
		{"null description", func(m map[string]any) { agent(m, "amp")["description"] = nil },
			[]string{`null at "/agents/amp/description"`}},
		{"null homepage", func(m map[string]any) { agent(m, "amp")["homepage"] = nil },
			[]string{`null at "/agents/amp/homepage"`}},
		{"null config.local", func(m map[string]any) { obj(agent(m, "amp"), "config")["local"] = nil },
			[]string{`null at "/agents/amp/config/local"`}},
		{"null skills", func(m map[string]any) { agent(m, "amp")["skills"] = nil },
			[]string{`null at "/agents/amp/skills"`}},
		{"null skills scope", func(m map[string]any) { obj(agent(m, "amp"), "skills")["global"] = nil },
			[]string{`null at "/agents/amp/skills/global"`}},
		{"null skills root", func(m map[string]any) { obj(obj(agent(m, "amp"), "skills"), "global")["native"] = nil },
			[]string{`null at "/agents/amp/skills/global/native"`}},
		{"null alternative", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "local")["alternatives"] = []any{".x/skills", nil}
		}, []string{`null at "/agents/amp/skills/local/alternatives/1"`}},
		{"null agnostic", func(m map[string]any) { agent(m, "agy")["agnostic"] = nil },
			[]string{`null at "/agents/agy/agnostic"`}},
		{"null provider", func(m map[string]any) { agent(m, "amp")["provider"] = nil },
			[]string{`null at "/agents/amp/provider"`}},
		{"every null reported", func(m map[string]any) {
			agent(m, "amp")["skills"] = nil
			agent(m, "aider")["homepage"] = nil
		}, []string{`null at "/agents/aider/homepage"`, `null at "/agents/amp/skills"`}},
		{"invalid id", func(m map[string]any) { m["Bad_ID"] = agent(m, "amp") },
			[]string{`"Bad_ID"`, "id does not match"}},
		{"id with a trailing hyphen", func(m map[string]any) { m["amp-"] = agent(m, "amp") },
			[]string{`"amp-"`, "id does not match"}},
		{"empty name", func(m map[string]any) { agent(m, "amp")["name"] = "" },
			[]string{`"amp"`, "name is empty"}},
		{"missing name", func(m map[string]any) { delete(agent(m, "amp"), "name") },
			[]string{`"amp"`, "name is empty"}},
		{"empty bin", func(m map[string]any) { agent(m, "amp")["bin"] = "" },
			[]string{`"amp"`, "bin is empty"}},
		{"missing bin", func(m map[string]any) { delete(agent(m, "amp"), "bin") },
			[]string{`"amp"`, "bin is empty"}},
		{"missing config", func(m map[string]any) { delete(agent(m, "amp"), "config") },
			[]string{`"amp"`, "config is missing"}},
		{"empty config.global", func(m map[string]any) { obj(agent(m, "amp"), "config")["global"] = "" },
			[]string{`"amp"`, "config.global is empty"}},
		{"empty config.local", func(m map[string]any) { obj(agent(m, "amp"), "config")["local"] = "" },
			[]string{`"amp"`, "config.local is empty"}},
		{"relative config.global", func(m map[string]any) { obj(agent(m, "amp"), "config")["global"] = ".a" },
			[]string{`"amp"`, `config.global ".a" is not "~"`}},
		{"home-relative config.local", func(m map[string]any) { obj(agent(m, "amp"), "config")["local"] = "~/.a" },
			[]string{`"amp"`, `config.local "~/.a" is not relative`}},
		{"another user's home as a global root", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "global")["native"] = "~other/skills"
		}, []string{`"amp"`, `skills.global.native "~other/skills" is not "~"`}},
		{"variable in a global root", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "global")["agents"] = "$XDG_CONFIG_HOME/a/skills"
		}, []string{`"amp"`, `skills.global.agents "$XDG_CONFIG_HOME/a/skills" is not "~"`, `contains "$"`}},
		{"variable below the home directory", func(m map[string]any) {
			obj(agent(m, "amp"), "config")["global"] = "~/$APP"
		}, []string{`"amp"`, `config.global "~/$APP" contains "$"`}},
		{"absolute local root", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "local")["native"] = "/etc/x"
		}, []string{`"amp"`, `skills.local.native "/etc/x" is not relative`}},
		{"drive prefix in a local alternative", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "local")["alternatives"] = []any{".x/skills", "C:/skills"}
		}, []string{`"amp"`, `skills.local.alternatives[1] "C:/skills" is not relative`}},
		{"skills without a scope", func(m map[string]any) { agent(m, "amp")["skills"] = map[string]any{} },
			[]string{`"amp"`, "skills has neither global nor local"}},
		{"scope without a root", func(m map[string]any) { obj(agent(m, "amp"), "skills")["global"] = map[string]any{} },
			[]string{`"amp"`, "skills.global has no agents, native, or alternatives"}},
		{"empty agents root", func(m map[string]any) { obj(obj(agent(m, "amp"), "skills"), "global")["agents"] = "" },
			[]string{`"amp"`, "skills.global.agents is empty"}},
		{"empty native root", func(m map[string]any) { obj(obj(agent(m, "amp"), "skills"), "global")["native"] = "" },
			[]string{`"amp"`, "skills.global.native is empty"}},
		{"empty alternatives list", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "local")["alternatives"] = []any{}
		}, []string{`"amp"`, "skills.local.alternatives is an empty list"}},
		{"empty alternative", func(m map[string]any) {
			obj(obj(agent(m, "amp"), "skills"), "local")["alternatives"] = []any{".x/skills", ""}
		}, []string{`"amp"`, "skills.local.alternatives[1] is empty"}},
		{"non-agnostic without provider", func(m map[string]any) { delete(agent(m, "agy"), "provider") },
			[]string{`"agy"`, "provider is missing or empty"}},
		{"non-agnostic with an empty provider list", func(m map[string]any) { agent(m, "agy")["provider"] = []any{} },
			[]string{`"agy"`, "provider is missing or empty"}},
		{"empty provider", func(m map[string]any) { agent(m, "agy")["provider"] = []any{"x", ""} },
			[]string{`"agy"`, "provider[1] is empty"}},
		{"agnostic with provider", func(m map[string]any) { agent(m, "amp")["provider"] = []any{"openai"} },
			[]string{`"amp"`, "provider is set on an agnostic agent"}},
		{"agnostic with an empty provider list", func(m map[string]any) { agent(m, "aider")["provider"] = []any{} },
			[]string{`"aider"`, "provider is set on an agnostic agent"}},
		{"every problem reported", func(m map[string]any) {
			agent(m, "amp")["bin"] = ""
			agent(m, "aider")["name"] = ""
		}, []string{`agent "aider": name is empty`, `agent "amp": bin is empty`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agents := copyAgents(t)
			tt.edit(agents)
			data, err := json.Marshal(map[string]any{"agents": agents})
			if err != nil {
				t.Fatal(err)
			}
			assertRejected(t, data, tt.want)
		})
	}
}

func TestParseRejectsFileShape(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []string
	}{
		{"duplicate agent id", `{"agents":{"a":{"name":"A","bin":"a","config":{"global":"~/.a"},"provider":["p"]},` +
			`"a":{"name":"B","bin":"b","config":{"global":"~/.b"},"provider":["p"]}}}`, []string{"duplicate", `"a"`}},
		{"duplicate field", `{"agents":{"a":{"name":"A","bin":"a","bin":"b","config":{"global":"~/.a"},"provider":["p"]}}}`,
			[]string{"duplicate", `"bin"`}},
		{"unknown top-level member", `{"agents":{},"version":1}`, []string{`"version"`}},
		{"no agents", `{}`, []string{"agents is missing or empty"}},
		{"empty agents", `{"agents":{}}`, []string{"agents is missing or empty"}},
		{"trailing data", `{"agents":{}} {}`, []string{"decode agents.json"}},
		{"not JSON", `agents: {}`, []string{"decode agents.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRejected(t, []byte(tt.data), tt.want)
		})
	}
}

func TestParseAcceptsMinimalAgent(t *testing.T) {
	data := `{"agents":{"a":{"name":"A","bin":"a","config":{"global":"~"},"provider":["p"]},` +
		`"b":{"name":"B","bin":"b","config":{"global":"~/.b"},"agnostic":true,` +
		`"skills":{"local":{"alternatives":[".x/skills",".y/skills"]}}}}}`
	agents, err := parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(agents) != 2 || agents[0].ID != "a" || agents[1].ID != "b" {
		t.Fatalf("parse = %+v, want agents a and b in order", agents)
	}
	if got := agents[1].Skills.Local.Primary(); got != ".x/skills" {
		t.Errorf("alternatives-only primary = %q, want %q", got, ".x/skills")
	}
}

func assertRejected(t *testing.T, data []byte, want []string) {
	t.Helper()
	agents, err := parse(data)
	if err == nil {
		t.Fatalf("parse accepted the catalog: %d agents", len(agents))
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not contain %q", err, w)
		}
	}
}

func copyAgents(t *testing.T) map[string]any {
	t.Helper()
	var f struct {
		Agents map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(agentsJSON, &f); err != nil {
		t.Fatal(err)
	}
	return f.Agents
}

func agent(agents map[string]any, id string) map[string]any { return obj(agents, id) }

func obj(m map[string]any, key string) map[string]any {
	v, ok := m[key].(map[string]any)
	if !ok {
		panic("test fixture: " + key + " is not an object")
	}
	return v
}
