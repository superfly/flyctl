package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func ConfirmConfigChange(ctx context.Context, machine *api.Machine, targetConfig api.MachineConfig) (bool, error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	diff := configCompare(ctx, *machine.Config, targetConfig)
	// If there's no diff we can treat it as if it wasn't confirmed.
	// TODO - This may not be the right thing to do.  Consider throwing an exception and
	// allow the caller to decide what to do here.
	if diff == "" {
		return false, nil
	}
	fmt.Fprintf(io.Out, "Configuration changes to be applied to machine %s (%s)\n", colorize.Bold(machine.Name), colorize.Bold(machine.ID))
	fmt.Fprintf(io.Out, "%s\n", diff)

	const msg = "Apply changes?"

	switch confirmed, err := prompt.Confirmf(ctx, msg); {
	case err == nil:
		if !confirmed {
			return false, nil
		}
	case prompt.IsNonInteractive(err):
		return false, prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
	default:
		return false, err
	}

	return true, nil
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

func configCompare(ctx context.Context, original api.MachineConfig, new api.MachineConfig) string {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	origBytes, _ := json.MarshalIndent(original, "", "\t")
	newBytes, _ := json.MarshalIndent(new, "", "\t")

	transformJSON := cmp.FilterValues(func(x, y []byte) bool {
		return json.Valid(x) && json.Valid(y)
	}, cmp.Transformer("parseJSON", func(in []byte) (out string) {
		out = string(in)
		return out
	}))

	diff := cmp.Diff(origBytes, newBytes, transformJSON)
	diffSlice := strings.Split(diff, "\n")

	var str string
	for _, val := range diffSlice {
		vB := []byte(val)
		addition, _ := regexp.Match(`^\+.*`, vB)
		deletion, _ := regexp.Match(`^\-.*`, vB)

		if addition {
			str += colorize.Green(val) + "\n"
		} else if deletion {
			str += colorize.Red(val) + "\n"
		} else {
			str += val + "\n"
		}
	}

	delim := "\"\"\""

	rx := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(delim) + `(.*?)` + regexp.QuoteMeta(delim))
	match := rx.FindStringSubmatch(str)
	if len(match) > 0 {
		return strings.Trim(match[1], "\n")
	}

	return ""
}
