package catalog

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	drivePattern = regexp.MustCompile(`^[A-Za-z]:`)
)

// The wire types mirror agents.json. Pointers and nil slices tell an absent
// member from an empty one, which validation must reject.
type (
	wireFile struct {
		Agents map[string]wireAgent `json:"agents"`
	}
	wireAgent struct {
		Name        string      `json:"name"`
		Bin         string      `json:"bin"`
		Description string      `json:"description"`
		Config      *wireConfig `json:"config"`
		Skills      *wireSkills `json:"skills"`
		Agnostic    bool        `json:"agnostic"`
		Provider    []string    `json:"provider"`
		Homepage    string      `json:"homepage"`
	}
	wireConfig struct {
		Global string  `json:"global"`
		Local  *string `json:"local"`
	}
	wireSkills struct {
		Global *wireScope `json:"global"`
		Local  *wireScope `json:"local"`
	}
	wireScope struct {
		Agents       *string  `json:"agents"`
		Native       *string  `json:"native"`
		Alternatives []string `json:"alternatives"`
	}
)

// parse decodes and validates a catalog file, returning its agents sorted by
// ID. Every null is rejected first, each named by its JSON pointer. A decode
// error then stops at the first problem; validation reports every problem, each
// naming the agent ID and field.
func parse(data []byte) ([]Agent, error) {
	if err := rejectNulls(data); err != nil {
		return nil, err
	}
	var f wireFile
	// json/v2 also rejects duplicate member names and matches names
	// case-sensitively, so a hand edit cannot shadow or misspell a field.
	if err := json.Unmarshal(data, &f, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode agents.json: %w", err)
	}
	if len(f.Agents) == 0 {
		return nil, errors.New("agents.json: agents is missing or empty")
	}
	var errs []error
	agents := make([]Agent, 0, len(f.Agents))
	for _, id := range slices.Sorted(maps.Keys(f.Agents)) {
		a, err := f.Agents[id].agent(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		agents = append(agents, a)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return agents, nil
}

// rejectNulls reports every null in data. No member has a meaning for null,
// and decoding turns one into the zero value, which validation cannot tell from
// an absent member. Malformed input is left for the decoder to report.
func rejectNulls(data []byte) error {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var errs []error
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return errors.Join(errs...)
		}
		if tok.Kind() == jsontext.KindNull {
			errs = append(errs, fmt.Errorf("agents.json: null at %q; give a value or omit the member", dec.StackPointer()))
		}
	}
}

func (w wireAgent) agent(id string) (Agent, error) {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("agent %q: "+format, append([]any{id}, args...)...))
	}

	if !idPattern.MatchString(id) {
		fail("id does not match %s", idPattern)
	}
	if w.Name == "" {
		fail("name is empty")
	}
	if w.Bin == "" {
		fail("bin is empty")
	}
	var cfg Config
	if w.Config == nil {
		fail("config is missing")
	} else {
		cfg.Global = w.Config.Global
		checkPath("config.global", cfg.Global, globalPath, fail)
		if w.Config.Local != nil {
			cfg.Local = *w.Config.Local
			checkPath("config.local", cfg.Local, localPath, fail)
		}
	}
	var skills Skills
	if w.Skills != nil {
		if w.Skills.Global == nil && w.Skills.Local == nil {
			fail("skills has neither global nor local")
		}
		skills.Global = w.Skills.Global.scope("skills.global", globalPath, fail)
		skills.Local = w.Skills.Local.scope("skills.local", localPath, fail)
	}
	switch {
	case w.Agnostic && w.Provider != nil:
		fail("provider is set on an agnostic agent")
	case !w.Agnostic && len(w.Provider) == 0:
		fail("provider is missing or empty on a non-agnostic agent")
	}
	for i, p := range w.Provider {
		if p == "" {
			fail("provider[%d] is empty", i)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return Agent{}, err
	}
	return Agent{
		ID:          id,
		Name:        w.Name,
		Bin:         w.Bin,
		Description: w.Description,
		Homepage:    w.Homepage,
		Config:      cfg,
		Skills:      skills,
		Agnostic:    w.Agnostic,
		Provider:    w.Provider,
	}, nil
}

// pathRule is the form a catalog path takes at one location. It reports what is
// wrong with a path, or "" when nothing is. The rules read the slash-separated
// string alone, so every host reaches the same verdict.
type pathRule func(path string) string

func globalPath(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return `is not "~" and does not start with "~/"`
	}
	return ""
}

func localPath(p string) string {
	if strings.HasPrefix(p, "~") || strings.HasPrefix(p, "/") || drivePattern.MatchString(p) {
		return "is not relative to the project directory"
	}
	return ""
}

// checkPath fails field when path is empty, breaks the rule, or holds a "$",
// which resolution would keep as a literal name rather than expand.
func checkPath(field, path string, rule pathRule, fail func(string, ...any)) {
	if path == "" {
		fail("%s is empty", field)
		return
	}
	if problem := rule(path); problem != "" {
		fail("%s %q %s", field, path, problem)
	}
	if strings.Contains(path, "$") {
		fail(`%s %q contains "$"; variables are not expanded`, field, path)
	}
}

func (w *wireScope) scope(field string, rule pathRule, fail func(string, ...any)) Scope {
	if w == nil {
		return Scope{}
	}
	if w.Agents == nil && w.Native == nil && w.Alternatives == nil {
		fail("%s has no agents, native, or alternatives", field)
		return Scope{}
	}
	var s Scope
	if w.Agents != nil {
		s.Agents = *w.Agents
		checkPath(field+".agents", s.Agents, rule, fail)
	}
	if w.Native != nil {
		s.Native = *w.Native
		checkPath(field+".native", s.Native, rule, fail)
	}
	if w.Alternatives != nil {
		if len(w.Alternatives) == 0 {
			fail("%s.alternatives is an empty list", field)
		}
		for i, alt := range w.Alternatives {
			checkPath(fmt.Sprintf("%s.alternatives[%d]", field, i), alt, rule, fail)
		}
		s.Alternatives = w.Alternatives
	}
	return s
}
