// Package duration parses and formats the p3bot CLI duration grammar: one or
// more <n><unit> segments, n a positive integer and unit one of d, h, m, s,
// in that order and each at most once, or "0". Examples: 30s, 5m, 1h30m, 7d,
// 2d12h.
//
// time.ParseDuration accepts forms the grammar forbids, such as 1.5h, 300ms,
// and 30m1h, and rejects 7d, so every user-facing duration goes through this
// package instead.
package duration

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

type unit struct {
	suffix byte
	size   time.Duration
}

var units = []unit{
	{'d', 24 * time.Hour},
	{'h', time.Hour},
	{'m', time.Minute},
	{'s', time.Second},
}

// Parse returns the duration s denotes in the grammar. "0" parses as zero;
// what zero means, such as no limit, is the caller's to document.
func Parse(s string) (time.Duration, error) {
	if s == "0" {
		return 0, nil
	}
	bad := fmt.Errorf("invalid duration %q: use whole-number segments in d, h, m, s order, each once (30s, 1h30m, 7d) or 0", s)
	if s == "" {
		return 0, bad
	}
	var total time.Duration
	allowed := units
	for rest := s; rest != ""; {
		i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
		if i <= 0 {
			return 0, bad
		}
		n, err := strconv.ParseInt(rest[:i], 10, 64)
		if err != nil || n == 0 {
			return 0, bad
		}
		u := slices.IndexFunc(allowed, func(x unit) bool { return x.suffix == rest[i] })
		if u < 0 || time.Duration(n) > (math.MaxInt64-total)/allowed[u].size {
			return 0, bad
		}
		total += time.Duration(n) * allowed[u].size
		allowed = allowed[u+1:]
		rest = rest[i+1:]
	}
	return total, nil
}

// Format renders d in the grammar. A positive d rounds up to whole seconds,
// so a short wait never renders as "0"; zero and negative render as "0".
// At the top of the range, where the next second would not fit in a
// time.Duration, Format stops at the last whole second, so the result parses.
func Format(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	// Counting seconds rather than rounding d itself cannot overflow near the
	// maximum. The next second is taken only while it still fits.
	secs := int64(d / time.Second)
	if d%time.Second != 0 && secs < int64(math.MaxInt64/time.Second) {
		secs++
	}
	var b strings.Builder
	for _, u := range units {
		size := int64(u.size / time.Second)
		if n := secs / size; n > 0 {
			fmt.Fprintf(&b, "%d%c", n, u.suffix)
			secs -= n * size
		}
	}
	return b.String()
}

// Value is a flag value in the grammar. It implements the String, Set, and
// Type methods of pflag's Value interface, so a CLI registers it with
// Flags().Var without this package importing pflag. Convert with
// time.Duration(v) to use it.
type Value time.Duration

// Set parses s in the grammar into v, leaving v unchanged on error.
func (v *Value) Set(s string) error {
	d, err := Parse(s)
	if err != nil {
		return err
	}
	*v = Value(d)
	return nil
}

// String renders v in the grammar.
func (v *Value) String() string { return Format(time.Duration(*v)) }

// Type names the value in help output. It is deliberately not "duration":
// pflag's GetDuration accepts a flag of that type and parses String with
// time.ParseDuration, which fails on "0" and on any value of a day or more.
// Under another name GetDuration rejects every value, so read the bound Value
// instead.
func (*Value) Type() string { return "interval" }
