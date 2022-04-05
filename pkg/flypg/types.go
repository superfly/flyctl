package flypg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type DatabaseListResponse struct {
	Result []PostgresDatabase
}

type PostgresDatabase struct {
	Name  string
	Users []string
}

type UserListResponse struct {
	Result []PostgresUser
}

type PostgresUser struct {
	Username  string
	Superuser bool
	Databases []string
}

type RevokeAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type GrantAccessRequest struct {
	Database string `json:"database"`
	Username string `json:"username"`
}

type CreateUserRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Superuser bool   `json:"superuser"`
}

type DeleteUserRequest struct {
	Username string `json:"username"`
}

type CreateDatabaseRequest struct {
	Name string `json:"name"`
}

type DeleteDatabaseRequest struct {
	Name string `json:"name"`
}

type CommandResponse struct {
	Result bool   `json:"result"`
	Error  string `json:"error"`
}

type FindDatabaseResponse struct {
	Result PostgresDatabase
}

type FindUserResponse struct {
	Result PostgresUser
}

type Error struct {
	StatusCode int
	Err        string `json:"error"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.StatusCode, e.Err)
}

func ErrorStatus(err error) int {
	var e *Error

	if errors.As(err, &e) {
		return e.StatusCode
	}
	return http.StatusInternalServerError
}

func newError(status int, res *http.Response) error {
	var e = new(Error)

	e.StatusCode = status

	switch res.Header.Get("Content-Type") {
	case "application/json":

		if err := json.NewDecoder(res.Body).Decode(e); err != nil {
			return err
		}
	default:
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		e.Err = string(b)
	}

	return e
}
