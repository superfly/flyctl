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
	return d.parseDuration(v)
}

func (d *Duration) UnmarshalTOML(v any) error {
	return d.parseDuration(v)
}

func (d Duration) MarshalTOML() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

func (d *Duration) parseDuration(v any) error {
	if v == nil {
		d.Duration = 0
		return nil
	}

	switch value := v.(type) {
	case int64, float64:
		d.Duration = time.Duration(value.(int64))
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unknown duration type: %T", value)
	}
}
