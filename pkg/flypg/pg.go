package flypg

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) ListUsers(ctx context.Context) ([]PostgresUser, error) {
	var endpoint = "/commands/users/list"

	out := new(UserListResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *Client) CreateUser(ctx context.Context, name, password string, superuser bool) error {
	var endpoint = "/commands/users/create"

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
	var endpoint = "/commands/users/delete"

	in := &DeleteUserRequest{
		Username: name,
	}

	if err := c.Do(ctx, http.MethodDelete, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListDatabases(ctx context.Context) ([]PostgresDatabase, error) {
	var endpoint = "/commands/databases/list"

	out := new(DatabaseListResponse)

	if err := c.Do(ctx, http.MethodGet, endpoint, nil, out); err != nil {
		return nil, err
	}
	return out.Result, nil

}

func (c *Client) CreateDatabase(ctx context.Context, name string) error {
	var endpoint = "/commands/databases/create"

	in := &CreateDatabaseRequest{
		Name: name,
	}

	if err := c.Do(ctx, http.MethodPost, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteDatabase(ctx context.Context, name string) error {
	var endpoint = "/commands/databases/delete"

	in := &DeleteDatabaseRequest{
		Name: name,
	}

	if err := c.Do(ctx, http.MethodDelete, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) DatabaseExists(ctx context.Context, name string) (bool, error) {
	var endpoint = "/commands/databases"

	endpoint = fmt.Sprintf("%s/%s", endpoint, name)

	out := new(FindDatabaseResponse)

	if err := c.Do(ctx, http.MethodPost, endpoint, nil, out); err != nil {
		return false, err
	}

	if out.Result.Name == name {
		return true, nil
	}
	return false, nil
}

func (c *Client) UserExists(ctx context.Context, name string) (bool, error) {
	var endpoint = "/commands/users"

	endpoint = fmt.Sprintf("%s/%s", endpoint, name)

	out := new(FindUserResponse)

	if err := c.Do(ctx, http.MethodPost, endpoint, nil, out); err != nil {
		return false, err
	}

	if out.Result.Username == name {
		return true, nil
	}
	return false, nil
}

func (c *Client) GrantAccess(ctx context.Context, dbName, userName string) error {
	var endpoint = "/commands/databases/grant"

	in := &GrantAccessRequest{
		Database: dbName,
		Username: userName,
	}

	if err := c.Do(ctx, http.MethodPost, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) RevokeAccess(ctx context.Context, dbName, userName string) error {
	var endpoint = "/commands/databases/revoke"

	in := &RevokeAccessRequest{
		Database: dbName,
		Username: userName,
	}

	if err := c.Do(ctx, http.MethodPost, endpoint, in, nil); err != nil {
		return err
	}
	return nil
}
