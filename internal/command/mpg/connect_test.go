package mpg

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/mpgutil"
)

func TestConnectArguments(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		cluster string
		want    mpgutil.PsqlOptions
		invalid bool
	}{
		{args: []string{"mpg", "connect"}},
		{args: []string{"mpg", "connect", "id"}, cluster: "id"},
		{args: []string{"mpg", "connect", "id", "--", "-c", "select 1"}, cluster: "id", want: mpgutil.PsqlOptions{ExtraArgs: []string{"-c", "select 1"}}},
		{args: []string{"mpg", "connect", "id", "-c", ""}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{""}}},
		{args: []string{"mpg", "connect", "-c", "select 1"}, invalid: true},
		{args: []string{"mpg", "connect", "-c", "select 1", "--", "--csv"}, invalid: true},
		{args: []string{"-t", "tok", "mpg", "connect", "id", "-c", "select 1", "--", "--csv"}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{"select 1"}, ExtraArgs: []string{"--csv"}}},
		{args: []string{"--verbose", "mpg", "connect", "id", "-c", "select 1", "--", "--csv", "-t"}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{"select 1"}, ExtraArgs: []string{"--csv", "-t"}}},
		{args: []string{"mpg", "--verbose", "connect", "id", "-c", "select 1", "--", "--csv"}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{"select 1"}, ExtraArgs: []string{"--csv"}}},
		{args: []string{"mpg", "connect", "id", "-c", "--"}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{"--"}}},
		{args: []string{"mpg", "connect", "id", "--"}, cluster: "id", want: mpgutil.PsqlOptions{ExtraArgs: []string{}}},
		{args: []string{"mpg", "connect", "--", "id"}, want: mpgutil.PsqlOptions{ExtraArgs: []string{"id"}}},
		{args: []string{"mpg", "connect", "-c", "select 1", "--", "psql-arg"}, invalid: true},
		{args: []string{"mpg", "connect", "", "-c", "select 1"}, invalid: true},
		{args: []string{"mpg", "connect", "-f", "-"}, invalid: true},
		{args: []string{"mpg", "connect", "id", "-c", "select 1", "-f", "a.sql"}, invalid: true},
		{args: []string{"mpg", "connect", "id", "-f", "a.sql", "-f", "b.sql"}, cluster: "id", want: mpgutil.PsqlOptions{Files: []string{"a.sql", "b.sql"}}},
		{args: []string{"mpg", "connect", "id", "-c", "", "-c", "select 2"}, cluster: "id", want: mpgutil.PsqlOptions{Commands: []string{"", "select 2"}}},
	} {
		t.Run(fmt.Sprint(tt.args), func(t *testing.T) {
			root := &cobra.Command{Use: "fly"}
			var out bytes.Buffer
			root.SetOut(&out)
			root.PersistentFlags().StringP("access-token", "t", "", "")
			root.PersistentFlags().Bool("verbose", false, "")
			group := &cobra.Command{Use: "mpg"}
			cmd := newConnect()
			group.AddCommand(cmd)
			root.AddCommand(group)
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				ctx := flag.NewContext(context.Background(), cmd.Flags())
				if tt.invalid {
					err := runConnect(ctx)
					require.Error(t, err)
					if cmd.Flags().Changed("command") && cmd.Flags().Changed("file") {
						require.Contains(t, err.Error(), "mutually exclusive")
						require.Contains(t, err.Error(), "psql arguments after --")
					} else {
						require.Contains(t, err.Error(), "cluster ID must be specified")
					}

					return nil
				}
				require.Equal(t, tt.cluster, clusterIDFromArgs(ctx))
				got := mpgutil.PsqlOptionsFromContext(ctx)
				require.Equal(t, tt.want, got)

				return nil
			}
			root.SetArgs(tt.args)
			require.NoError(t, root.Execute())
			require.Empty(t, out.String())
		})
	}
}

func TestConnectShorthandsDoNotConflictWithAncestors(t *testing.T) {
	// Mirror the global flags from root.New without importing root (which imports mpg).
	root := &cobra.Command{Use: "fly"}
	root.PersistentFlags().StringP("access-token", "t", "", "")
	root.PersistentFlags().Bool("verbose", false, "")
	root.PersistentFlags().Bool("debug", false, "")
	mpg := New()
	root.AddCommand(mpg)
	cmd, _, err := root.Find([]string{"mpg", "connect"})
	require.NoError(t, err)
	for _, shorthand := range []string{"c", "f"} {
		require.Nil(t, cmd.InheritedFlags().ShorthandLookup(shorthand))
		require.NotNil(t, cmd.LocalFlags().ShorthandLookup(shorthand))
	}
}

func TestConnectCobraErrorsSilenceUsage(t *testing.T) {
	for _, args := range [][]string{{"a", "b"}, {"id", "--bogus"}, {"id", "-c"}, {"id", "-c", "select 1", "extra"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			root := &cobra.Command{Use: "fly"}
			var out, stderr bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&stderr)
			root.AddCommand(newConnect())
			root.SetArgs(append([]string{"connect"}, args...))
			require.Error(t, root.Execute())
			require.Empty(t, out.String())
			require.NotEmpty(t, stderr.String())
		})
	}
}
