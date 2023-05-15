package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type TestEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Time    time.Time
}

func (e *TestEvent) String() string {
	if e.Test != "" {
		return fmt.Sprintf("--- %s: %s (%.3f) [pkg: %s]", strings.ToUpper(e.Action), e.Test, e.Elapsed, e.Package)
	} else {
		return fmt.Sprintf("--- %s: [pkg: %s] (%.3f)", strings.ToUpper(e.Action), e.Package, e.Elapsed)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: %s <njson filename>", os.Args[0])
	}
	njsonFilename := os.Args[1]
	buf, err := os.ReadFile(njsonFilename)
	if err != nil {
		return err
	}
	lines := bytes.Split(buf, []byte("\n"))
	var passing []TestEvent
	var failing []TestEvent
	var skipping []TestEvent
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event TestEvent
		err := json.Unmarshal(line, &event)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: event %d could not be parsed as json: %s: %v\n", i+1, string(line), err)
			continue
		}
		switch event.Action {
		case "fail":
			failing = append(failing, event)
		case "pass":
			passing = append(passing, event)
		case "skip":
			skipping = append(skipping, event)
		}
	}
	totalTestCount := len(passing) + len(failing) + len(skipping)
	if totalTestCount == 0 {
		fmt.Printf("No events found. Assuming success.\n")
		return nil
	}
	if len(passing) > 0 {
		fmt.Printf("### Passing tests:\n")
		for _, e := range passing {
			fmt.Println(e.String())
		}
		fmt.Println()
	}
	if len(skipping) > 0 {
		fmt.Fprintf(os.Stdout, "### Skipped tests:\n")
		for _, e := range skipping {
			fmt.Println(e.String())
		}
		fmt.Println()
	}
	if len(failing) > 0 {
		fmt.Fprintf(os.Stdout, "### Failing tests:\n")
		var testStrings []string
		for _, e := range failing {
			fmt.Println(e.String())
			if e.Test != "" {
				testStrings = append(testStrings, e.Test)
			}
		}
		fmt.Println()
		return fmt.Errorf("some tests failed, run them on your machine to see the output with:\n  make preflight-test T='%s'", strings.Join(testStrings, "|"))
	}
	return nil
}
