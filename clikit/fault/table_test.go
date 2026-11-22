package fault

import (
	"maps"
	"slices"
	"testing"
)

// TestCanonicalTable locks the table to the CLI design's canonical codes, so
// an added, dropped, or remapped code fails here first.
func TestCanonicalTable(t *testing.T) {
	want := map[string]struct {
		exit      int
		transient bool
	}{
		"INVALID_ARGUMENT":    {2, false},
		"INPUT_REQUIRED":      {2, false},
		"ENV_REQUIRED":        {2, false},
		"OUTPUT_REQUIRED":     {2, false},
		"INVALID_FIELDS":      {2, false},
		"INVALID_COLOR":       {2, false},
		"BAD_REQUEST":         {2, false},
		"AGENT_UNKNOWN":       {2, false},
		"NO_SKILLS_CONCEPT":   {2, false},
		"NO_SKILLS_ROOT":      {2, false},
		"NO_AGENTS":           {2, false},
		"NOT_FOUND":           {3, false},
		"UNAUTHENTICATED":     {4, false},
		"AUTH_EXPIRED":        {4, false},
		"PERMISSION_DENIED":   {4, false},
		"CONFLICT":            {5, false},
		"CURSOR_INVALID":      {5, false},
		"RATE_LIMITED":        {75, true},
		"UPSTREAM_ERROR":      {75, true},
		"WAIT_TIMEOUT":        {75, true},
		"CATALOG_UNAVAILABLE": {75, true},
		"UNKNOWN":             {1, false},
		"UPSTREAM_REJECTED":   {1, false},
		"CATALOG_INVALID":     {1, false},
		"CONFIG_INVALID":      {78, false},
	}
	if len(canonicalExit) != len(want) {
		t.Errorf("table has %d codes, want %d: %v", len(canonicalExit), len(want), slices.Sorted(maps.Keys(canonicalExit)))
	}
	for code, w := range want {
		p := ErrorPayload{Code: code, ExitCode: 99}
		if p.Exit() != w.exit || p.Transient() != w.transient {
			t.Errorf("%s: exit %d transient %v, want %d %v", code, p.Exit(), p.Transient(), w.exit, w.transient)
		}
	}
}
