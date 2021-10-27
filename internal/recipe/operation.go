package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/superfly/flyctl/api"
)

type RecipeOperation struct {
	Recipe       *Recipe
	Machine      *api.Machine
	Command      string
	ResultStatus string
	Message      string
	Error        string
}

type MachineResponse struct {
	Status string              `json:"status"`
	Data   MachineDataResponse `json:"data"`
}
type MachineDataResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func NewRecipeOperation(recipe *Recipe, machine *api.Machine, command string) *RecipeOperation {
	return &RecipeOperation{Machine: machine, Command: command, Recipe: recipe}
}

func (o *RecipeOperation) RunHTTPCommand(ctx context.Context, method, endpoint string) error {
	targetEndpoint := fmt.Sprintf("http://[%s]:4280%s", o.MachineIP(), endpoint)
	req, err := http.NewRequest(method, targetEndpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(o.Recipe.App.Name, o.Recipe.AuthToken)

	fmt.Printf("Running %s %s... ", method, endpoint)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var machineResp MachineResponse
	if err = json.Unmarshal(b, &machineResp); err != nil {
		return err
	}

	o.ResultStatus = machineResp.Status
	o.Message = machineResp.Data.Message
	o.Error = machineResp.Data.Error

	o.printResponse()

	return nil
}

func (o *RecipeOperation) RunSSHAttachCommand(ctx context.Context) error {
	fmt.Printf("Running %q against %s... ", o.Command, o.Machine.ID)

	formattedAddr := fmt.Sprintf("[%s]", o.Addr())
	err := sshConnect(&SSHParams{
		Ctx:       ctx,
		Org:       &o.Recipe.App.Organization,
		Dialer:    *o.Recipe.Dialer,
		ApiClient: o.Recipe.Client.API(),
		App:       o.Recipe.App.Name,
		Cmd:       o.Command,
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}, formattedAddr)
	if err != nil {
		return err
	}

	return nil
}

func (o *RecipeOperation) RunSSHCommand(ctx context.Context) error {
	var inBuf bytes.Buffer
	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	stdoutWriter := ioutils.NewWriteCloserWrapper(&outBuf, func() error { return nil })
	stderrWriter := ioutils.NewWriteCloserWrapper(&errBuf, func() error { return nil })
	inReader := ioutils.NewReadCloserWrapper(&inBuf, func() error { return nil })

	fmt.Printf("Running %q against %s... ", o.Command, o.Machine.ID)

	formattedAddr := fmt.Sprintf("[%s]", o.Addr())
	err := sshConnect(&SSHParams{
		Ctx:       ctx,
		Org:       &o.Recipe.App.Organization,
		Dialer:    *o.Recipe.Dialer,
		ApiClient: o.Recipe.Client.API(),
		App:       o.Recipe.App.Name,
		Cmd:       o.Command,
		Stdin:     inReader,
		Stdout:    stdoutWriter,
		Stderr:    stderrWriter,
	}, formattedAddr)
	if err != nil {
		return err
	}

	// TODO - I'm not 100% on how I feel about this yet.  However, I do like the idea of keeping the response format
	// consistent across operations.
	var machineResp MachineResponse
	if err = json.Unmarshal(outBuf.Bytes(), &machineResp); err != nil {
		return err
	}

	o.ResultStatus = machineResp.Status
	o.Message = machineResp.Data.Message
	o.Error = machineResp.Data.Error

	o.printResponse()

	return nil
}

func (o *RecipeOperation) Addr() string {
	return o.Machine.IPs.Nodes[0].IP
}

func (o *RecipeOperation) MachineIP() string {
	peerIP := net.ParseIP(o.Addr())
	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	return net.IP(natsIPBytes[:]).String()
}

func (o *RecipeOperation) printResponse() {
	if o.ResultStatus == "success" {
		if o.Message == "" {
			fmt.Printf("%s\n", o.ResultStatus)
		} else {
			fmt.Printf("%s - %q\n", o.ResultStatus, o.Message)
		}
	} else {
		fmt.Printf("%s - %q\n", o.ResultStatus, o.Error)
	}
}
