// Package catalog is the catalog of known AI coding agents: their binaries,
// config directories, skills roots, and providers, and detection of which of
// them are installed.
//
// The catalog is agents.json, embedded in the package and edited by hand. It
// is decoded and validated once per process; the package tests run the same
// validation, so a bad edit fails go test rather than a user's run.
package catalog

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"sync"
)

//go:embed agents.json
var agentsJSON []byte

// Agent is one catalog entry.
type Agent struct {
	ID          string // the catalog key, matching ^[a-z0-9]+(-[a-z0-9]+)*$
	Name        string
	Bin         string // the binary name looked up on the search path
	Description string
	Homepage    string
	Config      Config
	Skills      Skills
	// Agnostic is true for an agent that works with any provider; Provider
	// is then empty. Otherwise Provider lists the agent's home providers by
	// models.dev provider id and is never empty.
	Agnostic bool
	Provider []string
}

// Config is an agent's config directories as written in the catalog. Global
// starts with ~ for the home directory; Local, empty when the agent has no
// project config, is relative to the project directory.
type Config struct {
	Global string
	Local  string
}

// Skills is an agent's skills roots by location. The zero Skills means the
// agent has no skills concept; a zero Scope means it has none at that
// location.
type Skills struct {
	Global Scope
	Local  Scope
}

// Scope is the skills roots of one location as written in the catalog. Agents
// is the shared cross-agent root, Native the agent's own root, and
// Alternatives other roots the agent also reads, in priority order. An empty
// string or nil slice means the agent has no root of that kind.
type Scope struct {
	Agents       string
	Native       string
	Alternatives []string
}

// Primary returns the scope's primary root: Agents, else Native, else the
// first alternative. It is empty for a zero Scope.
func (s Scope) Primary() string {
	switch {
	case s.Agents != "":
		return s.Agents
	case s.Native != "":
		return s.Native
	case len(s.Alternatives) > 0:
		return s.Alternatives[0]
	}
	return ""
}

var embedded = sync.OnceValue(func() []Agent {
	agents, err := parse(agentsJSON)
	if err != nil {
		// The package tests validate the same bytes, so only a build that
		// skipped them can reach this.
		panic(fmt.Sprintf("catalog: embedded agents.json is invalid: %v", err))
	}
	return agents
})

// Agents returns every catalog agent, sorted by ID. The result is the
// caller's to modify.
func Agents() []Agent {
	src := embedded()
	out := make([]Agent, len(src))
	for i, a := range src {
		out[i] = a.clone()
	}
	return out
}

// Lookup returns the agent with the given ID, and false when the catalog has
// no such agent.
func Lookup(id string) (Agent, bool) {
	src := embedded()
	i, ok := slices.BinarySearchFunc(src, id, func(a Agent, id string) int {
		return strings.Compare(a.ID, id)
	})
	if !ok {
		return Agent{}, false
	}
	return src[i].clone(), true
}

func (a Agent) clone() Agent {
	a.Provider = slices.Clone(a.Provider)
	a.Skills.Global.Alternatives = slices.Clone(a.Skills.Global.Alternatives)
	a.Skills.Local.Alternatives = slices.Clone(a.Skills.Local.Alternatives)
	return a
}
