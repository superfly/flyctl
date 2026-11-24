package machine

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineExecFlagValidation(t *testing.T) {
	cmd := newMachineExec()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"machine-123", "--container", "app", "--no-container", "uname -a"})

	err := cmd.Execute()
	require.EqualError(t, err, "if any flags in the group [container no-container] are set none of the others can be; [container no-container] were all set")
}
