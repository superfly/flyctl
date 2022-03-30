package flypg

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
