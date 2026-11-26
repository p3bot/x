package skill

import (
	"slices"

	"github.com/p3bot/x/skill/catalog"
)

// hasSkillsConcept reports whether the catalog gives a any skills root.
// Catalog validation guarantees a scope with any root has a primary.
func hasSkillsConcept(a catalog.Agent) bool {
	return a.Skills.Global.Primary() != "" || a.Skills.Local.Primary() != ""
}

// installedSet returns every installed agent with a skills concept, sorted by
// id: the default target set, and the agents whose claims a root keeps.
func installedSet(env catalog.Env) []catalog.Detection {
	var out []catalog.Detection
	for _, d := range catalog.DetectAll(env) {
		if d.Binary.Found && hasSkillsConcept(d.Agent) {
			out = append(out, d)
		}
	}
	return out
}

// selectTargets returns the agents op acts on: the named agents, installed or
// not, in the order given without repeats, or the installed set when none are
// named.
func (s Skill) selectTargets(env catalog.Env, op operation, ids []string) ([]catalog.Detection, error) {
	if len(ids) == 0 {
		set := installedSet(env)
		if len(set) == 0 {
			return nil, s.errNoAgents()
		}
		return set, nil
	}
	out := make([]catalog.Detection, 0, len(ids))
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			continue
		}
		a, ok := catalog.Lookup(id)
		if !ok {
			return nil, s.errUnknownAgent(op, id)
		}
		if !hasSkillsConcept(a) {
			return nil, s.errNoSkillsConcept(id)
		}
		out = append(out, catalog.Detect(env, a))
	}
	return out, nil
}

// scopeAt returns d's resolved skills roots at loc.
func scopeAt(d catalog.Detection, loc Location) catalog.ResolvedScope {
	if loc == Local {
		return d.Skills.Local
	}
	return d.Skills.Global
}

// installRoot is the root install writes for one agent: the primary root for
// the default set, and for a named agent its native root, else the shared
// agents root. Named agents skip alternatives because an alternative may be
// another product's root. It is "" when the agent has no such root.
func installRoot(sc catalog.ResolvedScope, named bool) string {
	if !named {
		return sc.Primary.Path
	}
	if sc.Native.Path != "" {
		return sc.Native.Path
	}
	return sc.Agents.Path
}

// candidateRoots returns every root an install for the agent may have
// written: primary, native, and agents, without repeats or empty paths.
func candidateRoots(sc catalog.ResolvedScope) []string {
	var out []string
	for _, p := range []string{sc.Primary.Path, sc.Native.Path, sc.Agents.Path} {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
