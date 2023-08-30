package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

type ErrNoConfigChangesFound struct{}

func (e *ErrNoConfigChangesFound) Error() string {
	return "no config changes found"
}

func ConfirmConfigChanges(ctx context.Context, machine *api.Machine, targetConfig api.MachineConfig, customPrompt string) (bool, error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	diff := configCompare(ctx, *machine.Config, targetConfig)
	if diff == "" {
		return false, &ErrNoConfigChangesFound{}
	}

	if customPrompt != "" {
		fmt.Fprintf(io.Out, customPrompt)
	} else {
		fmt.Fprintf(io.Out, "Configuration changes to be applied to machine: %s (%s)\n", colorize.Bold(machine.ID), colorize.Bold(machine.Name))
	}

	fmt.Fprintf(io.Out, "\n%s\n", diff)

	const msg = "Apply changes?"
	switch confirmed, err := prompt.Confirmf(ctx, msg); {
	case err == nil:
		if !confirmed {
			return false, nil
		}
	case prompt.IsNonInteractive(err):
		return false, prompt.NonInteractiveError("yes flag must be specified when not running interactively")
	default:
		return false, err
	}

	return true, nil
}

// CloneConfig deep-copies a MachineConfig.
// If CloneConfig is called on a nil config, nil is returned.
func CloneConfig(orig *api.MachineConfig) *api.MachineConfig {
	if orig == nil {
		return nil
	}
	return helpers.Clone(orig)
}

var cmpOptions = cmp.Options{
	cmp.FilterValues(
		func(x, y []byte) bool { return json.Valid(x) && json.Valid(y) },
		cmp.Transformer("parseJSON",
			func(in []byte) (out string) {
				return string(in)
			})),
}

func configCompare(ctx context.Context, original api.MachineConfig, new api.MachineConfig) string {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	origBytes, _ := json.MarshalIndent(original, "", "  ")
	newBytes, _ := json.MarshalIndent(new, "", "  ")

	if cmp.Equal(origBytes, newBytes, cmpOptions) {
		return ""
	}

	diff := cmp.Diff(origBytes, newBytes, cmpOptions)
	diffSlice := strings.Split(diff, "\n")

	var str string
	additionReg := regexp.MustCompile(`^\+.*`)
	deletionReg := regexp.MustCompile(`^\-.*`)

	// Highlight additions/deletions
	for _, val := range diffSlice {
		vB := []byte(val)

		if additionReg.Match(vB) {
			str += colorize.Green(val) + "\n"
		} else if deletionReg.Match(vB) {
			str += colorize.Red(val) + "\n"
		} else {
			str += val + "\n"
		}
	}

	// Cleanup output
	delim := "\"\"\""
	rx := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(delim) + `(.*?)` + regexp.QuoteMeta(delim))
	match := rx.FindStringSubmatch(str)
	if len(match) > 0 {
		return strings.Trim(match[1], "\n")
	}
	// We know the objects are different, if we can't cleanup return the best we have got
	return str
}

// MergeFiles merges the files parsed from the command line or fly.toml into the machine configuration.
func MergeFiles(machineConf *api.MachineConfig, files []*api.File) {
	for _, f := range files {
		idx := slices.IndexFunc(machineConf.Files, func(i *api.File) bool {
			return i.GuestPath == f.GuestPath
		})

		switch {
		case idx == -1:
			machineConf.Files = append(machineConf.Files, f)
			continue
		case f.RawValue == nil && f.SecretName == nil:
			machineConf.Files = slices.Delete(machineConf.Files, idx, idx+1)
		default:
			machineConf.Files = slices.Replace(machineConf.Files, idx, idx+1, f)
		}
	}
}
