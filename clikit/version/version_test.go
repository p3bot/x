package version

import (
	"runtime/debug"
	"testing"
)

func buildInfo(mainVersion string, settings ...string) func() (*debug.BuildInfo, bool) {
	bi := &debug.BuildInfo{Main: debug.Module{Path: "example.com/cli", Version: mainVersion}}
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return func() (*debug.BuildInfo, bool) { return bi, true }
}

func noBuildInfo() (*debug.BuildInfo, bool) { return nil, false }

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name    string
		ldflags string
		read    func() (*debug.BuildInfo, bool)
		want    string
	}{
		{"ldflags wins over build info", "v1.2.3", buildInfo("v9.9.9"), "v1.2.3"},
		{"ldflags without build info", "v1.2.3", noBuildInfo, "v1.2.3"},
		{"dev ldflags falls to module version", Dev, buildInfo("v1.4.0"), "v1.4.0"},
		{"empty ldflags falls to module version", "", buildInfo("v1.4.0"), "v1.4.0"},
		{"pseudo-version is a module version", "", buildInfo("v0.0.0-20261122000000-abcdef123456"), "v0.0.0-20261122000000-abcdef123456"},
		{"devel module version is absent", Dev, buildInfo("(devel)"), Dev},
		{"empty module version is absent", "", buildInfo(""), Dev},
		{"no build info", "", noBuildInfo, Dev},
		{"nil build info", Dev, func() (*debug.BuildInfo, bool) { return nil, true }, Dev},
	}
	for _, tt := range tests {
		if got := resolve(tt.ldflags, tt.read).Version; got != tt.want {
			t.Errorf("%s: Version = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestResolveVCS(t *testing.T) {
	const rev, at = "0123456789abcdef0123456789abcdef01234567", "2026-11-22T11:00:00Z"
	tests := []struct {
		name         string
		read         func() (*debug.BuildInfo, bool)
		wantRevision string
		wantTime     string
	}{
		{"clean checkout", buildInfo("(devel)", "vcs.revision", rev, "vcs.modified", "false", "vcs.time", at), rev, at},
		{"dirty checkout", buildInfo("(devel)", "vcs.revision", rev, "vcs.modified", "true", "vcs.time", at), rev + "+dirty", at},
		{"go install records no VCS data", buildInfo("v1.0.0"), "", ""},
		{"modified without a revision stays empty", buildInfo("", "vcs.modified", "true"), "", ""},
		{"no build info", noBuildInfo, "", ""},
	}
	for _, tt := range tests {
		got := resolve("v1.0.0", tt.read)
		if got.Revision != tt.wantRevision || got.Time != tt.wantTime {
			t.Errorf("%s: Revision %q Time %q, want %q and %q", tt.name, got.Revision, got.Time, tt.wantRevision, tt.wantTime)
		}
	}
}

// TestResolveRunningBinary pins that Resolve reads the running binary. The
// version Go stamps on a test binary depends on the toolchain, so only the
// ldflags path is asserted exactly.
func TestResolveRunningBinary(t *testing.T) {
	if got := Resolve("v2.0.0").Version; got != "v2.0.0" {
		t.Errorf("Resolve(v2.0.0).Version = %q", got)
	}
	if got := Resolve("").Version; got == "" {
		t.Error("Resolve(\"\").Version is empty, want a module version or Dev")
	}
}
