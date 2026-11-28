package httptracing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFinishWritesValidHARWithNoRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.har")
	t.Setenv("FLYCTL_OUTPUT_HAR", path)

	previous := har
	t.Cleanup(func() { har = previous })
	har = nil

	Init()
	if har == nil {
		t.Fatal("Init did not enable HAR tracing with FLYCTL_OUTPUT_HAR set")
	}

	Finish()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading HAR file: %v", err)
	}

	var doc struct {
		Log struct {
			Entries json.RawMessage `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("HAR file is not valid JSON: %v", err)
	}

	if got := string(doc.Log.Entries); got != "[]" {
		t.Fatalf("log.entries = %s, want []; a null entries array is rejected by HAR importers", got)
	}
}
