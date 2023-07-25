package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"

	"github.com/superfly/flyctl/terminal"
)

type InvalidReleaseError struct {
	status int
	msg    string
}

func (i InvalidReleaseError) Error() string {
	return i.msg
}
func (i InvalidReleaseError) StatusCode() int {
	return i.status
}

// memoized values for ValidateRelease
var _validatedReleases = map[string]error{}
var _validatedReleaseLock sync.Mutex

// ValidateRelease reports whether the given release is valid via an API call.
// If the version is invalid, the error will be an InvalidReleaseError.
// Note that other errors may be returned if the API call fails.
func ValidateRelease(ctx context.Context, version string) (err error) {

	_validatedReleaseLock.Lock()
	defer _validatedReleaseLock.Unlock()

	if version[0] == 'v' {
		version = version[1:]
	}

	if err, ok := _validatedReleases[version]; ok {
		return err
	}

	defer func() {
		_validatedReleases[version] = err
	}()

	updateUrl := fmt.Sprintf("https://api.fly.io/app/flyctl_validate/v%s", version)

	req, err := http.NewRequestWithContext(ctx, "GET", updateUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %s", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return &InvalidReleaseError{
			status: resp.StatusCode,
			msg:    string(body),
		}
	}

	return nil
}

func latestApiRelease(ctx context.Context, channel string) (*Release, error) {

	updateUrl := fmt.Sprintf("https://api.fly.io/app/flyctl_releases/%s/%s/%s", runtime.GOOS, runtime.GOARCH, channel)

	req, err := http.NewRequestWithContext(ctx, "GET", updateUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %s", err)
		}
	}()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return &release, err
	}

	return &release, nil
}
