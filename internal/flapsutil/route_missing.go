package flapsutil

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/superfly/fly-go/flaps"
)

// IsFlapsRouteMissing reports whether err is a Flaps 404 that means "this
// flaps build doesn't implement this route yet", as opposed to a
// resource-level 404 (a missing cluster, database, or user) that flaps
// proxied verbatim from ui-ex.
//
// The two are easy to conflate because fly-go's FlapsError.Is matches
// purely on HTTP status code (see flaps.ErrFlapsNotFound), so
// errors.Is(err, flaps.ErrFlapsNotFound) is true for both. They are
// distinguishable by body shape, though:
//
//   - An unmatched route hits flaps's default mux handler, which is plain
//     net/http.NotFound: a text/plain body reading "404 page not found\n".
//   - A resource-level 404 is JSON flaps proxied from ui-ex's response,
//     shaped like {"error": "database not found"} (see fly-go's
//     handleAPIError, which is what turns that JSON into the FlapsError's
//     message).
//
// So: a 404 whose body parses as a JSON object carrying a non-empty "error"
// or "message" field is a resource error and must propagate to the user
// instead of triggering the legacy MPGv2 fallback. Anything else — a body
// that doesn't parse as JSON, an empty body, or JSON without either field —
// is conservatively treated as a missing route. That keeps today's fallback
// behavior intact for flaps builds old enough to predate the MPG v2 routes
// entirely, where there's no JSON error body to inspect. It also covers
// ui-ex's default JSON 404 body (FlyWeb.ErrorJSON),
// {"errors":{"detail":"not found"}}, which ui-ex uses both for an unmatched
// route and, on older builds, for an authz 404; the two can't be told
// apart, so the fallback wins.
func IsFlapsRouteMissing(err error) bool {
	var ferr *flaps.FlapsError
	if !errors.As(err, &ferr) {
		return false
	}

	if ferr.ResponseStatusCode != http.StatusNotFound {
		return false
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if jsonErr := json.Unmarshal(ferr.ResponseBody, &body); jsonErr != nil {
		// Not a JSON object at all (e.g. flaps's plain-text 404 page, or an
		// empty body) — conservatively assume the route is missing.
		return true
	}

	return body.Error == "" && body.Message == ""
}
