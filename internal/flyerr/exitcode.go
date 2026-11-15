package flyerr

import "fmt"

// ExitCodeError asks the CLI to exit with a specific status without printing
// an error. It is for commands that relay another process's exit status, such
// as a command run on a remote machine, where the remote command has already
// written whatever it had to say.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}
