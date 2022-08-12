package flypg

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) ListUsers(ctx context.Context) ([]PostgresUser, error) {
	endpoint := "/commands/users/list"

	out := new(UserListResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *Client) CreateUser(ctx context.Context, name, password string, superuser bool) error {
	endpoint := "/commands/users/create"

	in := &CreateUserRequest{
		Username:  name,
		Password:  password,
		Superuser: superuser,
	}

	if err := c.Do(ctx, http.MethodPost, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c Client) DeleteUser(ctx context.Context, name string) error {
	endpoint := "/commands/users/delete"

	endpoint = fmt.Sprintf("%s/%s", endpoint, name)

	if err := c.Do(ctx, http.MethodDelete, endpoint, nil, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListDatabases(ctx context.Context) ([]PostgresDatabase, error) {
	endpoint := "/commands/databases/list"

	out := new(DatabaseListResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *Client) CreateDatabase(ctx context.Context, name string) error {
	endpoint := "/commands/databases/create"

	in := &CreateDatabaseRequest{
		Name: name,
	}

	if err := c.Do(ctx, http.MethodPost, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteDatabase(ctx context.Context, name string) error {
	endpoint := "/commands/databases/delete"

	in := &DeleteDatabaseRequest{
		Name: name,
	}

	if err := c.Do(ctx, http.MethodDelete, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) DatabaseExists(ctx context.Context, name string) (bool, error) {
	endpoint := "/commands/databases"

	endpoint = fmt.Sprintf("%s/%s", endpoint, name)

	out := new(FindDatabaseResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		if ErrorStatus(err) == 404 {
			return false, nil
		}
		return false, err
	}

	if out.Result.Name == name {
		return true, nil
	}
	return false, nil
}

func (c *Client) UserExists(ctx context.Context, name string) (bool, error) {
	endpoint := "/commands/users"

	endpoint = fmt.Sprintf("%s/%s", endpoint, name)

	out := new(FindUserResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		if ErrorStatus(err) == 404 {
			return false, nil
		}
		return false, err
	}

	if out.Result.Username == name {
		return true, nil
	}
	return false, nil
}

func (c *Client) NodeRole(ctx context.Context) (string, error) {
	endpoint := "/commands/admin/role"

	out := new(NodeRoleResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return "", err
	}
	return out.Result, nil
}

func (c *Client) RestartNodePG(ctx context.Context) error {
	endpoint := "/commands/admin/restart"

	out := new(RestartResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return err
	}
	return nil
}

func (c *Client) Failover(ctx context.Context) error {
	endpoint := "/commands/admin/failover/trigger"

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) SettingsView(ctx context.Context, settings []string) (*PGSettings, error) {
	endpoint := "/commands/admin/settings/view"

	out := new(SettingsViewResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, settings, out); err != nil {
		return nil, err
	}

	return &out.Result, nil
}
