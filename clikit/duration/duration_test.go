package duration_test

import (
	"math"
	"testing"
	"time"

	"github.com/p3bot/x/clikit/duration"
)

const day = 24 * time.Hour

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"0", 0},
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"1h30m", 90 * time.Minute},
		{"7d", 7 * day},
		{"2d12h", 60 * time.Hour},
		{"1d2h3m4s", 26*time.Hour + 3*time.Minute + 4*time.Second},
		{"1d30s", day + 30*time.Second},
		{"106751d", 106751 * day},
	}
	for _, tt := range tests {
		got, err := duration.Parse(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		in  string
		why string
	}{
		{"", "empty"},
		{"1.5h", "fraction"},
		{"300ms", "sub-second unit"},
		{"30m1h", "units out of order"},
		{"1h2h", "repeated unit"},
		{"0s", "zero segment"},
		{"00", "zero spelled other than 0"},
		{"5", "no unit"},
		{"h", "no number"},
		{"-1h", "sign"},
		{"+1h", "sign"},
		{"1H", "upper-case unit"},
		{" 1h", "leading space"},
		{"1h ", "trailing space"},
		{"1w", "unknown unit"},
		{"1h30", "trailing number"},
		{"106752d", "past the duration range"},
		{"99999999999999999999s", "past int64"},
		{"1é", "non-ASCII unit"},
	}
	for _, tt := range tests {
		if got, err := duration.Parse(tt.in); err == nil {
			t.Errorf("Parse(%q) = %v, want an error (%s)", tt.in, got, tt.why)
		}
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0"},
		{-time.Second, "0"},
		{time.Nanosecond, "1s"},
		{time.Second, "1s"},
		{time.Second + time.Nanosecond, "2s"},
		{time.Hour, "1h"},
		{90 * time.Minute, "1h30m"},
		{60 * time.Hour, "2d12h"},
		{day + time.Second, "1d1s"},
		{math.MaxInt64, "106751d23h47m16s"},
	}
	for _, tt := range tests {
		if got := duration.Format(tt.in); got != tt.want {
			t.Errorf("Format(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatRoundTrips(t *testing.T) {
	for _, d := range []time.Duration{0, time.Second, 59 * time.Minute, 7 * day, 3*day + 4*time.Hour + 5*time.Second} {
		got, err := duration.Parse(duration.Format(d))
		if err != nil || got != d {
			t.Errorf("Parse(Format(%v)) = %v, %v", d, got, err)
		}
	}
}

func TestFormatSaturates(t *testing.T) {
	const want = "106751d23h47m16s"
	last := time.Duration(int64(math.MaxInt64/time.Second)) * time.Second
	for _, d := range []time.Duration{math.MaxInt64, last + 1} {
		got := duration.Format(d)
		if got != want {
			t.Errorf("Format(%d) = %q, want %q", int64(d), got, want)
		}
		back, err := duration.Parse(got)
		if err != nil || back != last {
			t.Errorf("Parse(Format(%d)) = %v, %v; want %v", int64(d), back, err, last)
		}
	}
}

func TestValue(t *testing.T) {
	var v duration.Value
	if err := v.Set("1h30m"); err != nil {
		t.Fatalf("Set(1h30m) = %v", err)
	}
	if time.Duration(v) != 90*time.Minute || v.String() != "1h30m" {
		t.Errorf("after Set(1h30m): %v, String %q", time.Duration(v), v.String())
	}
	if err := v.Set("1.5h"); err == nil {
		t.Error("Set(1.5h) succeeded, want an error")
	}
	if time.Duration(v) != 90*time.Minute {
		t.Errorf("failed Set changed the value to %v", time.Duration(v))
	}
	// "duration" would let pflag's GetDuration accept the flag and then fail
	// for some values only.
	if v.Type() != "interval" {
		t.Errorf("Type() = %q", v.Type())
	}
}

// pflagValue mirrors github.com/spf13/pflag.Value, which Value must satisfy
// without this module importing pflag.
type pflagValue interface {
	String() string
	Set(string) error
	Type() string
}

var _ pflagValue = (*duration.Value)(nil)

func FuzzParse(f *testing.F) {
	for _, s := range []string{"0", "30s", "1h30m", "7d", "1.5h", "30m1h", "106752d"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := duration.Parse(s)
		if err != nil {
			return
		}
		if d < 0 {
			t.Fatalf("Parse(%q) = %v, negative", s, d)
		}
		back, err := duration.Parse(duration.Format(d))
		if err != nil || back != d {
			t.Fatalf("Parse(Format(Parse(%q))) = %v, %v; want %v", s, back, err, d)
		}
	})
}
