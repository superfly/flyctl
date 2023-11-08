package flaps

import "net/http"

type flapsAction int

func (a *flapsAction) String() string {
	switch *a {
	case launch:
		return "launch"
	case update:
		return "update"
	case start:
		return "start"
	case wait:
		return "wait"
	case stop:
		return "stop"
	case restart:
		return "restart"
	case get:
		return "get"
	case list:
		return "list"
	case destroy:
		return "destroy"
	case kill:
		return "kill"
	case findLease:
		return "findLease"
	case acquireLease:
		return "acquireLease"
	case refreshLease:
		return "refreshLease"
	case releaseLease:
		return "releaseLease"
	case exec:
		return "exec"
	case ps:
		return "ps"
	case cordon:
		return "cordon"
	case uncordon:
		return "uncordon"
	default:
		return "unknownAction"
	}
}

const (
	launch flapsAction = iota
	update
	start
	wait
	stop
	restart
	get
	list
	destroy
	kill
	findLease
	acquireLease
	refreshLease
	releaseLease
	exec
	ps
	cordon
	uncordon
)

// The mapping from a flaps action to the API endpoint being hit
var flapsActionToEndpoint = map[flapsAction]string{
	launch:       "",
	update:       "",
	start:        "start",
	wait:         "wait",
	stop:         "stop",
	restart:      "restart",
	get:          "",
	list:         "",
	destroy:      "",
	kill:         "signal",
	findLease:    "lease",
	acquireLease: "lease",
	refreshLease: "lease",
	releaseLease: "lease",
	exec:         "exec",
	ps:           "ps",
	cordon:       "cordon",
	uncordon:     "uncordon",
}

// The mapping from a flaps action to the HTTP method being used
var flapsActionToMethod = map[flapsAction]string{
	launch:       http.MethodPost,
	update:       http.MethodPost,
	start:        http.MethodPost,
	wait:         http.MethodGet,
	stop:         http.MethodPost,
	restart:      http.MethodPost,
	get:          http.MethodGet,
	list:         http.MethodGet,
	destroy:      http.MethodDelete,
	kill:         http.MethodPost,
	findLease:    http.MethodGet,
	acquireLease: http.MethodPost,
	refreshLease: http.MethodPost,
	releaseLease: http.MethodDelete,
	exec:         http.MethodPost,
	ps:           http.MethodGet,
	cordon:       http.MethodPost,
	uncordon:     http.MethodPost,
}
