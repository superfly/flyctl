package launch

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestValidatePostgresFlags(t *testing.T) {
	tests := []struct {
		name        string
		dbFlag      string
		noDbFlag    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid mpg flag",
			dbFlag:      "mpg",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid upg flag",
			dbFlag:      "upg",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid legacy flag",
			dbFlag:      "legacy",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid true flag",
			dbFlag:      "true",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid empty flag",
			dbFlag:      "",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "invalid flag value",
			dbFlag:      "invalid",
			noDbFlag:    false,
			expectError: true,
			errorMsg:    "Invalid value 'invalid' for --db flag",
		},
		{
			name:        "conflicting db and no-db",
			dbFlag:      "mpg",
			noDbFlag:    true,
			expectError: true,
			errorMsg:    "Cannot specify both --db and --no-db",
		},
		{
			name:        "conflicting upg and no-db",
			dbFlag:      "upg",
			noDbFlag:    true,
			expectError: true,
			errorMsg:    "Cannot specify both --db and --no-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with iostreams
			ctx := context.Background()
			ctx = iostreams.NewContext(ctx, iostreams.System())

			// Create a test context with flags
			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flagSet.String("db", tt.dbFlag, "")
			flagSet.Bool("no-db", tt.noDbFlag, "")
			ctx = flagctx.NewContext(ctx, flagSet)

			err := validatePostgresFlags(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")

					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestNewDefaultPlanSource(t *testing.T) {
	source := "test source"
	planSource := newDefaultPlanSource(source)

	assert.NotNil(t, planSource)
	assert.Equal(t, source, planSource.appNameSource)
	assert.Equal(t, source, planSource.regionSource)
	assert.Equal(t, source, planSource.orgSource)
	assert.Equal(t, source, planSource.computeSource)
	assert.Equal(t, source, planSource.postgresSource)
	assert.Equal(t, source, planSource.redisSource)
	assert.Equal(t, source, planSource.tigrisSource)
	assert.Equal(t, source, planSource.sentrySource)
}

func TestParseMountOptions(t *testing.T) {
	tests := []struct {
		name          string
		options       string
		expectedMount appconfig.Mount
		expectError   bool
		errMsg        string
	}{
		{
			name:          "empty options",
			options:       "",
			expectedMount: appconfig.Mount{},
		},
		{
			name:    "scheduled_snapshots true",
			options: "scheduled_snapshots=true",
			expectedMount: appconfig.Mount{
				ScheduledSnapshots: new(true),
			},
		},
		{
			name:    "scheduled_snapshots false",
			options: "scheduled_snapshots=false",
			expectedMount: appconfig.Mount{
				ScheduledSnapshots: new(false),
			},
		},
		{
			name:        "scheduled_snapshots invalid value",
			options:     "scheduled_snapshots=invalid",
			expectError: true,
			errMsg:      "invalid value for scheduled_snapshots",
		},
		{
			name:    "snapshot_retention",
			options: "snapshot_retention=7",
			expectedMount: appconfig.Mount{
				SnapshotRetention: new(7),
			},
		},
		{
			name:        "snapshot_retention invalid",
			options:     "snapshot_retention=invalid",
			expectError: true,
			errMsg:      "invalid value for snapshot_retention",
		},
		{
			name:    "initial_size",
			options: "initial_size=10GB",
			expectedMount: appconfig.Mount{
				InitialSize: "10GB",
			},
		},
		{
			name:    "auto_extend_size_threshold",
			options: "auto_extend_size_threshold=80",
			expectedMount: appconfig.Mount{
				AutoExtendSizeThreshold: 80,
			},
		},
		{
			name:        "auto_extend_size_threshold invalid",
			options:     "auto_extend_size_threshold=invalid",
			expectError: true,
			errMsg:      "invalid value for auto_extend_size_threshold",
		},
		{
			name:    "auto_extend_size_increment",
			options: "auto_extend_size_increment=1GB",
			expectedMount: appconfig.Mount{
				AutoExtendSizeIncrement: "1GB",
			},
		},
		{
			name:    "auto_extend_size_limit",
			options: "auto_extend_size_limit=100GB",
			expectedMount: appconfig.Mount{
				AutoExtendSizeLimit: "100GB",
			},
		},
		{
			name:    "multiple options",
			options: "initial_size=10GB,scheduled_snapshots=true,snapshot_retention=14",
			expectedMount: appconfig.Mount{
				InitialSize:        "10GB",
				ScheduledSnapshots: new(true),
				SnapshotRetention:  new(14),
			},
		},
		{
			name:        "unknown option",
			options:     "unknown_option=value",
			expectError: true,
			errMsg:      "unknown mount option",
		},
		{
			name:        "invalid format",
			options:     "invalid_format",
			expectError: true,
			errMsg:      "invalid mount option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mount := &appconfig.Mount{}
			err := ParseMountOptions(mount, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")

					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)

					return
				}

				assert.Equal(t, tt.expectedMount, *mount)
			}
		})
	}
}

// parseLaunchArgs parses args through the real launch command and runs its
// Args validator, the way cobra does before calling RunE.
func parseLaunchArgs(t *testing.T, args ...string) (db string, err error) {
	t.Helper()
	cmd := New()
	require.NoError(t, cmd.ParseFlags(args))
	err = cmd.ValidateArgs(cmd.Flags().Args())
	db, gerr := cmd.Flags().GetString("db")
	require.NoError(t, gerr)

	return db, err
}

func TestLaunchDBFlagValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"space separated mpg", []string{"--db", "mpg"}, "mpg"},
		{"space separated managed", []string{"--db", "managed"}, "managed"},
		{"space separated legacy", []string{"--db", "legacy"}, "legacy"},
		{"equals mpg", []string{"--db=mpg"}, "mpg"},
		{"space separated upg", []string{"--db", "upg"}, "upg"},
		{"bare db", []string{"--db"}, "true"},
		{"space separated with other flags", []string{"--no-deploy", "--db", "managed", "--region", "ord"}, "managed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := parseLaunchArgs(t, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.want, db)
		})
	}
}

// Anything that is not a database type name falls through to cobra.NoArgs with
// master's error, including boolean-like words, which must never be taken as
// the --db value or swallowed from another flag. flags lists values that must
// be left exactly as pflag parsed them (as on master).
func TestLaunchDBPositionalRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		stray string
		flags map[string]string
	}{
		{"unknown word", []string{"--db", "foo"}, "foo", map[string]string{"db": "true"}},
		{"ha false", []string{"--db", "--ha", "false"}, "false", map[string]string{"db": "true", "ha": "true"}},
		{"depot false", []string{"--db", "--depot", "false"}, "false", map[string]string{"db": "true", "depot": "true"}},
		{"no-deploy false", []string{"--db", "--no-deploy", "false"}, "false", map[string]string{"db": "true", "no-deploy": "true"}},
		{"yes no", []string{"--db", "--yes", "no"}, "no", map[string]string{"db": "true", "yes": "true"}},
		{"boolean-like true", []string{"--db", "true"}, "true", map[string]string{"db": "true"}},
		{"boolean-like false", []string{"--db", "false"}, "false", map[string]string{"db": "true"}},
		{"boolean-like 0", []string{"--db", "0"}, "0", map[string]string{"db": "true"}},
		{"empty value", []string{"--db", ""}, "", map[string]string{"db": "true"}},
		{"explicit true then upg", []string{"--db=true", "upg"}, "upg", map[string]string{"db": "true"}},
		{"explicit 1 then upg", []string{"--db=1", "upg"}, "upg", map[string]string{"db": "1"}},
		{"bare upg then explicit mpg", []string{"--db", "upg", "--db=mpg"}, "upg", map[string]string{"db": "mpg"}},
		{"bare then explicit true then upg", []string{"--db", "--db=true", "upg"}, "upg", map[string]string{"db": "true"}},
		{"bare then explicit mpg then upg", []string{"--db", "--db=mpg", "upg"}, "upg", map[string]string{"db": "mpg"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := New()
			require.NoError(t, cmd.ParseFlags(tc.args))
			err := cmd.ValidateArgs(cmd.Flags().Args())
			require.Error(t, err)
			assert.Equal(t, cobra.NoArgs(cmd, []string{tc.stray}).Error(), err.Error())
			assert.Contains(t, err.Error(), "unknown command")
			for n, want := range tc.flags {
				assert.Equal(t, want, cmd.Flags().Lookup(n).Value.String(), n)
			}
		})
	}
}

// Occurrence, not final string, decides: the last --db wins.
func TestLaunchDBRepeatedFlagLastWins(t *testing.T) {
	db, err := parseLaunchArgs(t, "--db=mpg", "--db", "upg")
	require.NoError(t, err)
	assert.Equal(t, "upg", db)

	// When the last --db carries a value, a positional is a stray argument
	// (see TestLaunchDBPositionalRejected).
	db, err = parseLaunchArgs(t, "--db", "--db=mpg")
	require.NoError(t, err)
	assert.Equal(t, "mpg", db)
}

func TestLaunchDBBareStaysTrue(t *testing.T) {
	db, err := parseLaunchArgs(t, "--db", "--no-deploy")
	require.NoError(t, err)
	assert.Equal(t, "true", db)
	db, err = parseLaunchArgs(t, "--db=true")
	require.NoError(t, err)
	assert.Equal(t, "true", db)
}

// The bare --db sentinel must not leak into help, for launch or its
// subcommands (which inherit launch's usage function).
func TestLaunchDBHelpShowsPlainTrue(t *testing.T) {
	root := &cobra.Command{Use: "fly"}
	launch := New()
	root.AddCommand(launch)

	var out strings.Builder
	launch.SetOut(&out)
	require.NoError(t, launch.Usage())
	assert.Contains(t, out.String(), `--db string[="true"]`)
	assert.NotContains(t, out.String(), bareDBSentinel)
	assert.Equal(t, bareDBSentinel, launch.Flags().Lookup("db").NoOptDefVal)

	plan, _, err := root.Find([]string{"launch", "plan"})
	require.NoError(t, err)
	out.Reset()
	plan.SetOut(&out)
	require.NoError(t, plan.Usage())
	assert.Contains(t, out.String(), "launch plan")
}

func TestLaunchDBWithNoDBStillConflicts(t *testing.T) {
	cmd := New()
	require.NoError(t, cmd.ParseFlags([]string{"--db", "mpg", "--no-db"}))
	require.NoError(t, cmd.ValidateArgs(cmd.Flags().Args()))
	db, _ := cmd.Flags().GetString("db")
	noDB, _ := cmd.Flags().GetBool("no-db")
	assert.Equal(t, "mpg", db)

	ctx := flagctx.NewContext(context.Background(), cmd.Flags())
	err := validatePostgresFlags(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot specify both --db and --no-db")
	assert.True(t, noDB)
}

func TestLaunchPositionalArgsStillRejected(t *testing.T) {
	// Without --db, behaviour is identical to cobra.NoArgs.
	cmd := New()
	require.NoError(t, cmd.ParseFlags([]string{"extra"}))
	err := cmd.ValidateArgs(cmd.Flags().Args())
	require.Error(t, err)
	assert.Equal(t, cobra.NoArgs(cmd, []string{"extra"}).Error(), err.Error())
	assert.Contains(t, err.Error(), `unknown command "extra" for "launch"`)

	// An explicit --db=mpg does not make a stray argument acceptable.
	_, err = parseLaunchArgs(t, "--db=mpg", "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown command "extra"`)

	// Bare --db with more than one argument is rejected.
	_, err = parseLaunchArgs(t, "--db", "mpg", "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

// internal/cli strips everything after "--" before cobra sees the arguments
// and hands it to the scanner through flag.WithExtraArgs, so launch's Args
// validator only ever sees the part before "--". Check both that view and the
// case where "--" does reach cobra: neither may change.
func TestLaunchDoubleDashUnchanged(t *testing.T) {
	// What cobra sees after internal/cli splits `launch --db mpg -- --foo`.
	db, err := parseLaunchArgs(t, "--db", "mpg")
	require.NoError(t, err)
	assert.Equal(t, "mpg", db)

	// "--" reaching cobra directly: still rejected like NoArgs.
	_, err = parseLaunchArgs(t, "--", "--foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown command "--foo"`)

	// The value after "--" is never taken as the --db value.
	_, err = parseLaunchArgs(t, "--db", "--", "mpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown command "mpg"`)

	// Extra args travel through the context, untouched by Args.
	ctx := flag.WithExtraArgs(context.Background(), []string{"--foo", "bar"})
	assert.Equal(t, []string{"--foo", "bar"}, flag.ExtraArgsFromContext(ctx))
}

// Documents a known looseness: launchArgs cannot see where the argument sat on
// the command line, so a value before a bare --db is accepted too.
func TestLaunchDBValueBeforeFlagAccepted(t *testing.T) {
	db, err := parseLaunchArgs(t, "mpg", "--db")
	require.NoError(t, err)
	assert.Equal(t, "mpg", db)
}
