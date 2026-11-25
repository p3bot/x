// Package version resolves the version a CLI reports, and the VCS details Go
// recorded in the binary for diagnostic commands.
//
// The CLI owns its ldflags variable, conventionally defaulting to Dev, and
// passes its value in; this package declares none, so each CLI's build
// scripts keep targeting their own package path.
package version

import "runtime/debug"

// Dev is reported when neither the ldflags value nor Go's build info carries
// a version: a build Go stamped "(devel)", such as one outside a VCS
// checkout or with -buildvcs=false.
const Dev = "dev"

// Info describes the running binary.
type Info struct {
	// Version is the reported version: the ldflags value unless it is empty
	// or Dev, else the main module version from build info unless it is
	// empty or "(devel)", else Dev.
	Version string
	// Revision is the VCS revision from build info, suffixed "+dirty" when the
	// working tree had uncommitted changes.
	Revision string
	// Time is the VCS commit time from build info, in RFC 3339 form.
	Time string
}

// Resolve returns the Info of the running binary, given the value of the
// CLI's ldflags version variable, or "" when the CLI has none.
//
// Revision and Time come only from build info, which carries VCS data only
// when the binary was built inside a checkout. go install of a module version
// and builds from a source tarball record none, so both are empty there and
// Version alone identifies the build.
func Resolve(ldflags string) Info {
	return resolve(ldflags, debug.ReadBuildInfo)
}

func resolve(ldflags string, read func() (*debug.BuildInfo, bool)) Info {
	info := Info{Version: Dev}
	bi, ok := read()
	if !ok || bi == nil {
		bi = &debug.BuildInfo{}
	}
	switch v := bi.Main.Version; {
	case ldflags != "" && ldflags != Dev:
		info.Version = ldflags
	case v != "" && v != "(devel)":
		info.Version = v
	}
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		case "vcs.time":
			info.Time = s.Value
		}
	}
	if dirty && info.Revision != "" {
		info.Revision += "+dirty"
	}
	return info
}
