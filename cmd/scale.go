package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newAppScaleCommand() *Command {
	scaleStrings := docstrings.Get("scale")

	cmd := BuildCommand(nil, runAppScale, scaleStrings.Usage, scaleStrings.Short, scaleStrings.Long, true, os.Stdout, requireAppName)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runAppScale(ctx *CmdContext) error {
	regionInput := []api.ScaleRegionInput{}

	for _, pair := range ctx.Args {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Changes must be provided as REGION=COUNT pairs (%s is invalid)", pair)
		}

		val, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("Counts must be numbers (%s is invalid)", pair)
		}

		regionInput = append(regionInput, api.ScaleRegionInput{
			Region: parts[0],
			Count:  val,
		})
	}

	if len(regionInput) < 1 {
		return errors.New("Requires at least one REGION=COUNT pair")
	}

	changes, err := ctx.FlyClient.ScaleApp(ctx.AppName, regionInput)
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		fmt.Println("No changes made")
		return nil
	}

	for _, change := range changes {
		fmt.Printf("Scaled %s from %d to %d\n", change.Region, change.FromCount, change.ToCount)
	}

	return nil
}
