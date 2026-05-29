package flag

import (
	"context"
	"fmt"
	"github.com/karrick/tparse/v2"
	"github.com/spf13/pflag"
	"strings"
	"time"
)

type timeValue struct{ t *time.Time }

func newTimeValue(val time.Time, p *time.Time) *timeValue {
	*p = val
	return &timeValue{t: p}
}

func (tv *timeValue) String() string {
	if tv.t == nil {
		return ""
	}
	if tv.t.IsZero() {
		return "0"
	}
	return tv.t.Format(time.RFC3339)
}

func (tv *timeValue) Set(s string) error {
	s = strings.TrimSpace(strings.ToLower(s))
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.DateTime,
		time.DateOnly,
	}
	for _, layout := range layouts {
		if t, err := tparse.ParseNow(layout, s); err == nil {
			*tv.t = t
			return nil
		}
	}
	return fmt.Errorf("failed to parse time '%s': unsupported format or invalid value", s)
}

func (tv *timeValue) Type() string {
	return "time"
}

var _ pflag.Value = (*timeValue)(nil)

func TimeVar(p *pflag.FlagSet, pt *time.Time, name string, shorthand string, value time.Time, usage string) {
	p.VarP(newTimeValue(value, pt), name, shorthand, usage)
}

func GetTime(ctx context.Context, name string) time.Time {
	if f := FromContext(ctx).Lookup(name); f == nil {
		return time.Time{}
	} else {
		return *f.Value.(*timeValue).t
	}
}
