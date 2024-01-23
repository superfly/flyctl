package api

import (
	"encoding/json"
	"fmt"
	"time"
)

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	return d.ParseDuration(v)
}

func (d *Duration) UnmarshalTOML(v any) error {
	return d.ParseDuration(v)
}

func (d Duration) MarshalTOML() ([]byte, error) {
	v := fmt.Sprintf("\"%s\"", d.Duration.String())
	return []byte(v), nil
}

func (d *Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	return d.ParseDuration(text)
}

func (d *Duration) ParseDuration(v any) error {
	if v == nil {
		d.Duration = 0
		return nil
	}

	switch value := v.(type) {
	case int64:
		d.Duration = time.Duration(value)
	case float64:
		d.Duration = time.Duration(int64(value))
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unknown duration type: %T", value)
	}
	return nil
}

// Compile parses a duration and returns, if successful, a Duration object.
func ParseDuration(v any) (*Duration, error) {
	d := &Duration{}
	if err := d.ParseDuration(v); err != nil {
		return nil, err
	}
	return d, nil
}

// MustParseDuration is like ParseDuration but panics if the expression cannot be parsed.
// It simplifies safe initialization of global variables holding durations
// Same idea than regexp.MustCompile
func MustParseDuration(v any) *Duration {
	d, err := ParseDuration(v)
	if err != nil {
		panic(err)
	}
	return d
}
