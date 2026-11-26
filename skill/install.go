package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/p3bot/x/clikit/atomicfile"
	"github.com/p3bot/x/skill/catalog"
)

// Install writes the skill to <root>/<id>/SKILL.md under each target agent's
// install root at loc, and returns the written paths sorted. With no agents
// the targets are the installed agents with a skills concept, each at its
// primary root, and an agent with no root at loc is skipped; named agents
// resolve whether installed or not, each at its native root, else the shared
// agents root. A root shared by several targets is written once.
//
// Every root is checked before any is written, so a foreign copy at any root
// fails with ErrForeignCopy and writes nothing. A failure while writing
// returns the paths already written with the error.
func (s Skill) Install(env catalog.Env, loc Location, agents []string) ([]string, error) {
	if err := s.check(loc); err != nil {
		return nil, err
	}
	targets, err := s.selectTargets(env, opInstall, agents)
	if err != nil {
		return nil, err
	}
	named := len(agents) > 0
	var roots []string
	for _, d := range targets {
		root := installRoot(scopeAt(d, loc), named)
		if root == "" {
			if named {
				return nil, s.errNoSkillsRoot(fmt.Sprintf("agent %q has no %v skills root", d.Agent.ID, loc), nil)
			}
			continue
		}
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil, s.errNoSkillsRoot(fmt.Sprintf("no installed agent has a %v skills root", loc), nil)
	}
	slices.Sort(roots)

	for _, root := range roots {
		if err := s.checkReplaceable(root); err != nil {
			return nil, s.rootErr(root, err)
		}
	}
	written := make([]string, 0, len(roots))
	for _, root := range roots {
		path, err := s.write(root)
		if err != nil {
			return written, s.rootErr(root, err)
		}
		written = append(written, path)
	}
	return written, nil
}

// rootErr makes a refused write at root ErrNoSkillsRoot and returns any other
// error unchanged. Windows reports neither error, so there a refused write
// stays an ordinary I/O error.
func (s Skill) rootErr(root string, err error) error {
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return s.errNoSkillsRoot(fmt.Sprintf("skills root %s refuses writes: %v", root, err), err)
	}
	return err
}

// checkReplaceable returns ErrForeignCopy when root holds a copy install must
// not replace, and nil when <root>/<id>/SKILL.md is absent or owned.
func (s Skill) checkReplaceable(root string) error {
	dir := filepath.Join(root, s.id)
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !info.IsDir():
		return errForeignCopy(dir, "not a directory")
	}
	file := filepath.Join(dir, FileName)
	// Lstat, because atomicfile follows a link and would rewrite its target.
	info, err = os.Lstat(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return errForeignCopy(file, "not a regular file")
	}
	reason, err := s.foreignReason(file)
	if err != nil {
		return err
	}
	if reason != "" {
		return errForeignCopy(file, reason)
	}
	return nil
}

// foreignReason says why the SKILL.md at file is not this skill's, or returns
// "" when it is. An edited copy that still names the id is owned.
func (s Skill) foreignReason(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	name, err := frontmatterName(data)
	if err != nil {
		return "unreadable " + err.Error(), nil
	}
	if name != s.id {
		return fmt.Sprintf("frontmatter name is %q, not %q", name, s.id), nil
	}
	return "", nil
}

func (s Skill) write(root string) (string, error) {
	dir := filepath.Join(root, s.id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	file := filepath.Join(dir, FileName)
	if err := atomicfile.Write(file, []byte(s.text), 0o644); err != nil {
		return "", err
	}
	return file, nil
}
