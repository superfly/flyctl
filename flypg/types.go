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

type RestartResponse struct {
	Result string
}

type NodeRoleResponse struct {
	Result string
}

type PGSettings struct {
	Settings []PGSetting `json:"settings,omitempty"`
}

type PGSetting struct {
	Name           string   `json:"name,omitempty"`
	Setting        string   `json:"setting,omitempty"`
	VarType        string   `json:"vartype,omitempty"`
	MinVal         string   `json:"min_val,omitempty"`
	MaxVal         string   `json:"max_val,omitempty"`
	EnumVals       []string `json:"enumvals,omitempty"`
	Context        string   `json:"context,omitempty"`
	Unit           string   `json:"unit,omitempty"`
	Desc           string   `json:"short_desc,omitempty"`
	PendingChange  string   `json:"pending_change,omitempty"`
	PendingRestart bool     `json:"pending_restart,omitempty"`
}

type SettingsViewResponse struct {
	Result PGSettings
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
	e := new(Error)

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
