package skill

import (
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/p3bot/x/skill/catalog"
)

// Copy is one installed copy of the skill: the path of its SKILL.md, and the
// ids, sorted, of the installed agents whose candidate roots include it.
type Copy struct {
	Path   string
	Agents []string
}

// List returns the installed copies at loc, sorted by path: every candidate
// root of an installed agent with a skills concept that holds the skill's
// SKILL.md as a regular file. It is empty, not an error, when nothing is
// installed or no agent is.
func (s Skill) List(env catalog.Env, loc Location) ([]Copy, error) {
	if err := s.check(loc); err != nil {
		return nil, err
	}
	claims := make(map[string][]string)
	for _, d := range installedSet(env) {
		for _, root := range candidateRoots(scopeAt(d, loc)) {
			if s.present(root) {
				claims[root] = append(claims[root], d.Agent.ID)
			}
		}
	}
	copies := make([]Copy, 0, len(claims))
	for _, root := range slices.Sorted(maps.Keys(claims)) {
		copies = append(copies, Copy{Path: filepath.Join(root, s.id, FileName), Agents: claims[root]})
	}
	return copies, nil
}

// present reports whether <root>/<id>/SKILL.md is a regular file.
func (s Skill) present(root string) bool {
	info, err := os.Stat(filepath.Join(root, s.id, FileName))
	return err == nil && info.Mode().IsRegular()
}
