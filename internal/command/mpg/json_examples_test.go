package mpg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterJSONExamples(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("example execution uses a POSIX shell stub")
	}
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("example execution requires %s", tool)
		}
	}
	dir := t.TempDir()
	stub := "#!/bin/sh\nprintf '%s' \"$STUB_OUT\"\nexit \"$STUB_CODE\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fly"), []byte(stub), 0o755))
	for _, shellName := range []string{"bash", "sh"} {
		t.Run(shellName, func(t *testing.T) {
			if _, err := exec.LookPath(shellName); err != nil {
				t.Skipf("example execution requires %s", shellName)
			}
			for _, operation := range []string{"create", "fork", "restore", "users create", "databases create"} {
				cmd, _, _ := mutationCommand(operation)
				syntax := exec.CommandContext(t.Context(), shellName, "-n")
				syntax.Stdin = strings.NewReader(cmd.Example)
				out, err := syntax.CombinedOutput()
				require.NoError(t, err, "%s", out)
				field, variable := "id", "cluster_id"
				switch operation {
				case "users create":
					field, variable = "name", "username"
				case "databases create":
					field, variable = "name", "database"
				}
				for _, strict := range []bool{false, true} {
					for _, stage := range []string{"ready", "exists but not ready", "nothing created", "empty", "null", "missing", "no document"} {
						t.Run(fmt.Sprintf("%s/%s/strict=%t", operation, stage, strict), func(t *testing.T) {
							document, code := fmt.Sprintf(`{"%s":"new-id","status":"ready"}`, field), "0"
							switch stage {
							case "exists but not ready":
								document, code = fmt.Sprintf(`{"%s":"new-id","status":"creating"}`, field), "1"
							case "nothing created":
								document, code = "", "1"
							case "empty":
								document = fmt.Sprintf(`{"%s":""}`, field)
							case "missing":
								document = `{}`
							case "no document":
								document = ""
							case "null":
								document = fmt.Sprintf(`{"%s":null}`, field)
							}
							args := []string{"-c"}
							if strict {
								if shellName == "bash" {
									args = append([]string{"-eo", "pipefail"}, args...)
								} else {
									args = append([]string{"-e"}, args...)
								}
							}
							args = append(args, cmd.Example+"\nprintf 'continued: %s\\n' \"$"+variable+"\"")
							shell := exec.CommandContext(t.Context(), shellName, args...)
							shell.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "STUB_OUT="+document, "STUB_CODE="+code,
								"source_id=source-id", "backup_id=backup-id", "cluster_id=source-id")
							var stdout, stderr strings.Builder
							shell.Stdout, shell.Stderr = &stdout, &stderr
							err := shell.Run()
							if stage == "ready" {
								require.NoError(t, err)
								require.Equal(t, "continued: new-id\n", stdout.String())
								require.Empty(t, stderr.String())
							} else {
								var exit *exec.ExitError
								require.ErrorAs(t, err, &exit)
								require.Equal(t, 1, exit.ExitCode())
								require.Empty(t, stdout.String())
								if code == "1" {
									require.Equal(t, document+"\n", stderr.String())
								} else {
									require.Empty(t, stderr.String())
								}
							}
							t.Logf("stdout=%q stderr=%q", stdout.String(), stderr.String())
						})
					}
				}
			}
		})
	}
}
