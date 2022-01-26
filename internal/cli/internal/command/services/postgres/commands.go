package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command/ssh"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"
)

type postgresDatabaseListResponse struct {
	Result []postgresDatabase
}

type postgresDatabase struct {
	Name  string
	Users []string
}

type postgresUserListResponse struct {
	Result []postgresUser
}

type postgresUser struct {
	Username  string
	Superuser bool
	Databases []string
}

type postgresRevokeAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type postgresGrantAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type postgresCreateUserRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Superuser bool   `json:"superuser"`
}

type postgresDeleteUserRequest struct {
	Username string `json:"username"`
}

type postgresCreateDatabaseRequest struct {
	Name string `json:"name"`
}

type postgresCommandResponse struct {
	Result bool   `json:"result"`
	Error  string `json:"error"`
}

type postgresCmd struct {
	ctx    *context.Context
	app    *api.App
	dialer agent.Dialer
	io     *iostreams.IOStreams
}

func newPostgresCmd(ctx context.Context, app *api.App, dialer agent.Dialer) *postgresCmd {
	return &postgresCmd{
		ctx:    &ctx,
		app:    app,
		dialer: dialer,
		io:     iostreams.FromContext(ctx),
	}
}

func (pc *postgresCmd) revokeAccess(dbName, username string) (*postgresCommandResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin revoke-access")
	req := &postgresRevokeAccessRequest{
		Database: dbName,
		Username: username,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin revoke-access %s", string(reqJSON))
	createUsrBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp postgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *postgresCmd) grantAccess(dbName, username string) (*postgresCommandResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin grant-access")
	req := &postgresGrantAccessRequest{
		Database: dbName,
		Username: username,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin grant-access %s", string(reqJSON))
	createUsrBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp postgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *postgresCmd) createUser(userName, pwd string) (*postgresCommandResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin user-create")
	req := &postgresCreateUserRequest{
		Username:  userName,
		Password:  pwd,
		Superuser: false,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin user-create %s", string(reqJSON))
	createUsrBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp postgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *postgresCmd) deleteUser(userName string) (*postgresCommandResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin user-delete")
	req := &postgresDeleteUserRequest{
		Username: userName,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin user-delete %s", string(reqJSON))
	createUsrBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp postgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *postgresCmd) createDatabase(dbName string) (*postgresCommandResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin database-create")
	req := &postgresCreateDatabaseRequest{Name: dbName}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin database-create %s", string(reqJSON))
	createDbBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp postgresCommandResponse
	if err := json.Unmarshal(createDbBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *postgresCmd) listDatabases() (*postgresDatabaseListResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin database-list")
	databaseListBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, "flyadmin database-list")
	if err != nil {
		return nil, err
	}

	var dbList postgresDatabaseListResponse
	if err := json.Unmarshal(databaseListBytes, &dbList); err != nil {
		return nil, err
	}

	return &dbList, nil
}

func (pc *postgresCmd) DbExists(dbName string) (bool, error) {
	dbList, err := pc.listDatabases()
	if err != nil {
		return false, err
	}

	for _, db := range dbList.Result {
		if db.Name == dbName {
			return true, nil
		}
	}

	return false, nil
}

func (pc *postgresCmd) listUsers() (*postgresUserListResponse, error) {
	fmt.Fprintln(pc.io.Out, "Running flyadmin user-list")
	userListBytes, err := ssh.RunSSHCommand(*pc.ctx, pc.app, pc.dialer, "flyadmin user-list")
	if err != nil {
		return nil, err
	}

	var userList postgresUserListResponse
	if err := json.Unmarshal(userListBytes, &userList); err != nil {
		return nil, err
	}

	return &userList, nil
}

func (pc *postgresCmd) userExists(userName string) (bool, error) {
	userList, err := pc.listUsers()
	if err != nil {
		return false, err
	}

	for _, user := range userList.Result {
		if user.Username == userName {
			return true, nil
		}
	}

	return false, nil
}
