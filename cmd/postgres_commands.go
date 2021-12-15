package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/pkg/agent"
)

type PostgresDatabaseListResponse struct {
	Result []PostgresDatabase
}

type PostgresDatabase struct {
	Name  string
	Users []string
}

type PostgresUserListResponse struct {
	Result []PostgresUser
}

type PostgresUser struct {
	Username  string
	Superuser bool
	Databases []string
}

type PostgresRevokeAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type PostgresGrantAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type PostgresCreateUserRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Superuser bool   `json:"superuser"`
}

type PostgresDeleteUserRequest struct {
	Username string `json:"username"`
}

type PostgresCreateDatabaseRequest struct {
	Name string `json:"name"`
}

type PostgresCommandResponse struct {
	Result bool   `json:"result"`
	Error  string `json:"error"`
}

type PostgresCmd struct {
	cmdCtx *cmdctx.CmdContext
	app    *api.App
	dialer agent.Dialer
}

func NewPostgresCmd(cmdCtx *cmdctx.CmdContext, app *api.App, dialer agent.Dialer) *PostgresCmd {
	return &PostgresCmd{
		cmdCtx: cmdCtx,
		app:    app,
		dialer: dialer,
	}
}

func (pc *PostgresCmd) RevokeAccess(dbName, username string) (*PostgresCommandResponse, error) {
	fmt.Println("Running flyadmin revoke-access")
	req := &PostgresRevokeAccessRequest{
		Database: dbName,
		Username: username,
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin revoke-access %s", string(reqJson))
	createUsrBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp PostgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *PostgresCmd) GrantAccess(dbName, username string) (*PostgresCommandResponse, error) {
	fmt.Println("Running flyadmin grant-access")
	req := &PostgresGrantAccessRequest{
		Database: dbName,
		Username: username,
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin grant-access %s", string(reqJson))
	createUsrBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp PostgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *PostgresCmd) CreateUser(userName, pwd string) (*PostgresCommandResponse, error) {
	fmt.Println("Running flyadmin user-create")
	req := &PostgresCreateUserRequest{
		Username:  userName,
		Password:  pwd,
		Superuser: false,
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin user-create %s", string(reqJson))
	createUsrBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp PostgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *PostgresCmd) DeleteUser(userName string) (*PostgresCommandResponse, error) {
	fmt.Println("Running flyadmin user-delete")
	req := &PostgresDeleteUserRequest{
		Username: userName,
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin user-delete %s", string(reqJson))
	createUsrBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp PostgresCommandResponse
	if err := json.Unmarshal(createUsrBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *PostgresCmd) CreateDatabase(dbName string) (*PostgresCommandResponse, error) {
	fmt.Println("Running flyadmin database-create")
	req := &PostgresCreateDatabaseRequest{Name: dbName}
	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("flyadmin database-create %s", string(reqJson))
	createDbBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, cmd)
	if err != nil {
		return nil, err
	}

	var resp PostgresCommandResponse
	if err := json.Unmarshal(createDbBytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (pc *PostgresCmd) DbExists(dbName string) (bool, error) {
	fmt.Println("Running flyadmin database-list")
	databaseListBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, "flyadmin database-list")
	if err != nil {
		return false, err
	}

	var dbList PostgresDatabaseListResponse
	if err := json.Unmarshal(databaseListBytes, &dbList); err != nil {
		return false, err
	}

	for _, db := range dbList.Result {
		if db.Name == dbName {
			return true, nil
		}
	}

	return false, nil
}

func (pc *PostgresCmd) UserExists(userName string) (bool, error) {
	fmt.Println("Running flyadmin user-list")
	userListBytes, err := runSSHCommand(pc.cmdCtx, pc.app, pc.dialer, "flyadmin user-list")
	if err != nil {
		return false, err
	}

	var userList PostgresUserListResponse
	if err := json.Unmarshal(userListBytes, &userList); err != nil {
		return false, err
	}

	for _, user := range userList.Result {
		if user.Username == userName {
			return true, nil
		}
	}

	return false, nil
}
