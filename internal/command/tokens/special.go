package tokens

import (
	"context"
	"fmt"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func newSpecial() *cobra.Command {
	const (
		short = "Create special attenuated Fly.io API tokens"
		long  = `Attenuate a Fly.io API token for special purposes. The
				token to be attenuated may either be passed in the -t argument
				or in FLY_API_TOKEN.`
		usage = "special"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newSpecialExec(),
	)

	return cmd
}

func newSpecialExec() *cobra.Command {
	const (
		short = "Create an API token that restricts command execution"
		long  = `Attenuate a Fly.io API token to restrict command execution. The
				token to be attenuated may either be passed in the -t argument
				or in FLY_API_TOKEN.`
		usage = "exec"
	)

	cmd := command.New(usage, short, long, runSpecialExec)

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "command",
			Shorthand:   "c",
			Description: "An allowed command with arguments. This command must match exactly",
		},
		flag.Bool{
			Name:        "all-commands",
			Description: "Allow all commands",
		},
		flag.String{
			Name:        "location",
			Shorthand:   "l",
			Description: "Location identifier of macaroons to attenuate",
			Default:     flyio.LocationPermission,
			Hidden:      true,
		},
	)

	return cmd
}

func runSpecialExec(ctx context.Context) error {
	macs, _, _, disToks, err := getPermissionAndDischargeTokens(ctx)
	if err != nil {
		return err
	}

	cav, err := getCommandCaveat(ctx)
	if err != nil {
		return err
	}

	for _, m := range macs {
		if err := m.Add(cav); err != nil {
			return fmt.Errorf("unable to attenuate macaroon: %w", err)
		}
	}

	return encodeAndPrintToken(macs, nil, nil, disToks)
}

func getCommandCaveat(ctx context.Context) (macaroon.Caveat, error) {
	commands := flyio.Commands{}
	if flag.GetBool(ctx, "all-commands") {
		cav := flyio.Command{
			Args:  []string{},
			Exact: false,
		}
		commands = append(commands, cav)
	}

	for _, cmd := range flag.GetStringSlice(ctx, "command") {
		args, err := shlex.Split(cmd)
		if err != nil {
			return nil, fmt.Errorf("cant parse `%s`: %w", cmd, err)
		}

		cav := flyio.Command{
			Args:  args,
			Exact: true,
		}
		commands = append(commands, cav)
	}

	for _, cmd := range flag.GetStringSlice(ctx, "command-prefix") {
		args, err := shlex.Split(cmd)
		if err != nil {
			return nil, fmt.Errorf("cant parse `%s`: %w", cmd, err)
		}

		cav := flyio.Command{
			Args:  args,
			Exact: false,
		}
		commands = append(commands, cav)
	}

	var cav macaroon.Caveat
	if true {
		// TODO: should we always return an if-present caveat?
		// or can we support a raw commands cavet?
		// Right now flyctl needs a token that can also perform graphql
		// read machines in order to execute a command.
		cav = &resset.IfPresent{
			Ifs:  macaroon.NewCaveatSet(&commands),
			Else: resset.ActionAll, // TODO: restrict to "r" here?  or allow All to only restrict on cmd?
		}
	} else {
		cav = &commands
	}

	return cav, nil
}
