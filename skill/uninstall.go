package skill

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/p3bot/x/clikit/atomicfile"
	"github.com/p3bot/x/skill/catalog"
)

// Outcome is what uninstall did at one skills root.
type Outcome string

// Uninstall outcomes, as the skill uninstall report prints them.
const (
	Removed Outcome = "removed"
	Absent  Outcome = "absent"
	Kept    Outcome = "kept"
)

// Removal is uninstall's result at one skills root: the skill directory
// <root>/<id>, the outcome, and for Kept the reason.
type Removal struct {
	Outcome Outcome
	Dir     string
	Reason  string
}

// Uninstall removes the skill's owned copies from every candidate root of the
// target agents at loc, and returns one Removal per root, sorted by
// directory. Targets are chosen as for Install. A directory is removed only
// when it holds SKILL.md alone, as a regular file whose frontmatter name is
// the id. With named agents, a root that an installed agent outside the
// targets also claims is never touched: it is Kept, naming the claiming
// agents, or Absent when it holds no copy. A claim covers the directory, so it
// holds when the two reach it by different paths, as through a symbolic link.
//
// An I/O error returns the removals already made with the error. A copy whose
// directory cannot be removed keeps its SKILL.md, so uninstall can be retried.
func (s Skill) Uninstall(env catalog.Env, loc Location, agents []string) ([]Removal, error) {
	if err := s.check(loc); err != nil {
		return nil, err
	}
	targets, err := s.selectTargets(env, opUninstall, agents)
	if err != nil {
		return nil, err
	}
	// The installed agents outside the targets, whose claims block removal.
	// The default set targets every installed agent, so it has none.
	var outside []catalog.Detection
	if len(agents) > 0 {
		outside = slices.DeleteFunc(installedSet(env), func(d catalog.Detection) bool {
			return slices.Contains(agents, d.Agent.ID)
		})
	}
	roots := make(map[string]bool)
	for _, d := range targets {
		for _, root := range candidateRoots(scopeAt(d, loc)) {
			roots[root] = true
		}
	}

	var out []Removal
	for _, root := range slices.Sorted(maps.Keys(roots)) {
		dir := filepath.Join(root, s.id)
		if claimants := claimantsOf(outside, loc, root); len(claimants) > 0 {
			r := Removal{Outcome: Absent, Dir: dir}
			if s.present(root) {
				r = Removal{Outcome: Kept, Dir: dir, Reason: "claimed by " + strings.Join(claimants, ",")}
			}
			out = append(out, r)
			continue
		}
		r, err := s.removeOwned(dir)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// claimantsOf returns the ids, in the order of agents, of those with a
// candidate root at loc that is the same directory as root.
func claimantsOf(agents []catalog.Detection, loc Location, root string) []string {
	var out []string
	for _, d := range agents {
		if slices.ContainsFunc(candidateRoots(scopeAt(d, loc)), func(r string) bool {
			return sameDir(r, root)
		}) {
			out = append(out, d.Agent.ID)
		}
	}
	return out
}

// sameDir reports whether the paths a and b are equal or both lead to one
// existing directory. A path that cannot be read matches only an equal path.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}

// removeOwned deletes dir when it holds SKILL.md alone and that file is
// owned, and reports why it kept dir otherwise.
func (s Skill) removeOwned(dir string) (Removal, error) {
	kept := func(reason string) (Removal, error) {
		return Removal{Outcome: Kept, Dir: dir, Reason: reason}, nil
	}
	exists, reason, err := skillDirState(dir)
	switch {
	case err != nil:
		return Removal{}, err
	case !exists:
		return Removal{Outcome: Absent, Dir: dir}, nil
	case reason != "":
		return kept(reason)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Removal{}, err
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		return kept("directory does not hold " + FileName + " alone")
	}
	if !entries[0].Type().IsRegular() {
		return kept(FileName + " is not a regular file")
	}
	file := filepath.Join(dir, FileName)
	data, reason, err := s.foreignReason(file)
	if err != nil {
		return Removal{}, err
	}
	if reason != "" {
		return kept(reason)
	}
	// Two removes, not RemoveAll: a file created in dir since the check makes
	// the second fail rather than deleting a file the skill does not own.
	if err := os.Remove(file); err != nil {
		return Removal{}, err
	}
	if err := os.Remove(dir); err != nil {
		// Without SKILL.md every later run would keep dir as not owned.
		if restoreErr := atomicfile.Write(file, data, 0o644); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore after failed removal: %w", restoreErr))
		}
		return Removal{}, err
	}
	return Removal{Outcome: Removed, Dir: dir}, nil
}
