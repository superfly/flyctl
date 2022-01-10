package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/superfly/flyctl/pkg/agent/client"

	"github.com/superfly/flyctl/internal/buildinfo"
)

func status(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, http.StatusOK, client.Status{
		PID:        os.Getpid(),
		Version:    buildinfo.Version(),
		Background: false, // TODO: fix this
	})
}

func renderCode(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
}

func renderJSON(w http.ResponseWriter, code int, v interface{}) {
	renderCode(w, code)

	_ = json.NewEncoder(w).Encode(v)
}
