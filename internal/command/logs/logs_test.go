package logs

import (
	"fmt"
	"testing"
	"github.com/stretchr/testify/require"
)

func TestgetMachineID(t *testing.T){

	t.Run("TestThrowsErrorWhenInstanceAndMachineClash", func(tt *testing.T){
		machineId, err := getMachineID("testa","testb")
		fmt.Println(machineId)
		fmt.Println(err)
		require.Contains(t, err.Error(), `Tokens for app`)
	})

}