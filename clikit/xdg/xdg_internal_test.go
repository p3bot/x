package xdg

import (
	"os"
	"strings"
	"testing"
)

// The default search list is checked here because proving it through
// SystemConfigFile would need a file under /etc.
func TestConfigDirs(t *testing.T) {
	explicit := strings.Join([]string{"/opt/xdg", "relative", "/etc/xdg"}, string(os.PathListSeparator))
	for _, tt := range []struct {
		goos      string
		byDefault string
	}{
		{"linux", "/etc/xdg"},
		{"windows", "/etc/xdg"},
		{"freebsd", "/etc/xdg"},
		{"", "/etc/xdg"},
		{"darwin", ""},
	} {
		unset := Env{GOOS: tt.goos}
		if got := unset.configDirs(); got != tt.byDefault {
			t.Errorf("GOOS %q unset: configDirs = %q, want %q", tt.goos, got, tt.byDefault)
		}
		for value, want := range map[string]string{"": tt.byDefault, explicit: explicit} {
			env := Env{
				Getenv: func(key string) string { return map[string]string{"XDG_CONFIG_DIRS": value}[key] },
				GOOS:   tt.goos,
			}
			if got := env.configDirs(); got != want {
				t.Errorf("GOOS %q value %q: configDirs = %q, want %q", tt.goos, value, got, want)
			}
		}
	}
}
