package machine

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"
)

func ConfigCompare(ctx context.Context, original api.MachineConfig, new api.MachineConfig) string {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	origBytes, _ := json.MarshalIndent(original, "", "\t")
	newBytes, _ := json.MarshalIndent(new, "", "\t")

	transformJSON := cmp.FilterValues(func(x, y []byte) bool {
		// fmt.Printf("X: %s, Y: %s\n", x, y)
		return json.Valid(x) && json.Valid(y)
	}, cmp.Transformer("parseJSON", func(in []byte) (out string) {
		out = string(in)
		// fmt.Printf("Out: %s\n", out)

		return out
	}))

	diff := cmp.Diff(origBytes, newBytes, transformJSON)

	diffSlice := strings.Split(diff, "\n")

	var str string

	changes := 0
	for _, val := range diffSlice {
		vB := []byte(val)
		addition, _ := regexp.Match(`^\+.*`, vB)
		deletion, _ := regexp.Match(`^\-.*`, vB)

		// fmt.Printf("Val: %+v. Add Match: %v, Del Match: %v\n", val, addition, deletion)

		if addition {
			// fmt.Printf("Addition: %+v\n", val)
			changes++
			str += colorize.Green(val) + "\n"
		} else if deletion {
			// fmt.Printf("Deletion: %+v\n", val)

			changes++
			str += colorize.Red(val) + "\n"
		} else {
			str += val + "\n"
		}
	}

	//
	left := "\"\"\""
	right := "\"\"\""
	rx := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(left) + `(.*?)` + regexp.QuoteMeta(right))

	// re := regexp.MustCompile(`\"\"\"(.*?)`)

	match := rx.FindStringSubmatch(str)

	// fmt.Printf("%q\n", strings.Trim(match[1], "\n\u00a0\u00a0\t\t"))
	if len(match) > 0 {
		return strings.Trim(match[1], "\n\u00a0\u00a0\t")
	}
	// fmt.Printf("Matches: %s\n", matches)

	return ""
}

func CloneConfig(orig api.MachineConfig) (*api.MachineConfig, error) {
	config := &api.MachineConfig{}

	data, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, err
}
