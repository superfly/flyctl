package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/pkg/agent/client"
)

func status(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, http.StatusOK, client.Status{
		PID:        os.Getpid(),
		Version:    buildinfo.Version(),
		Background: false, // TODO: fix this
	})
}

func renderJSON(w http.ResponseWriter, code int, v interface{}) {
	w.WriteHeader(code)

	_ = json.NewEncoder(w).Encode(v)
}
