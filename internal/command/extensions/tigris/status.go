package tigris

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func status() *cobra.Command {
	const (
		short = "Show details about a Tigris storage bucket"
		long  = short + "\n"

		usage = "status [name]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession, command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		extensions_core.SharedFlags,
	)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	extension, app, err := extensions_core.Discover(ctx, gql.AddOnTypeTigris)

	if err != nil {
		return err
	}

	obj := [][]string{
		{
			extension.Name,
			extension.Status,
		},
	}

	var cols []string = []string{"Name", "Status"}

	optionKeys := []string{"public", "shadow_bucket.write_through", "shadow_bucket.name", "shadow_bucket.endpoint"}

	options, _ := extension.Options.(map[string]interface{})

	for _, key := range optionKeys {
		value := "False"
		keys := strings.Split(key, ".")
		var opt interface{}
		var ok bool

		if len(keys) > 1 {
			nestedMap, exists := options[keys[0]].(map[string]interface{})
			if exists {
				opt, ok = nestedMap[keys[1]]
			} else {
				break
			}
		} else {
			opt, ok = options[key]
		}

		if ok {
			switch v := opt.(type) {
			case bool:
				if v {
					value = "True"
				}
			case string:
				value = v
			}
		}
		obj[0] = append(obj[0], value)
		colName := strings.Title(strings.Replace(strings.Join(keys, " "), "_", " ", -1))
		cols = append(cols, colName)
	}

	if app != nil {
		obj[0] = append(obj[0], app.Name)
		cols = append(cols, "App")
	}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}
	return
}
