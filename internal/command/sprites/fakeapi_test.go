package sprites

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	sprites "github.com/superfly/sprites-go"
)

// spritesAPI is an in-memory Sprites API: the REST endpoints, the filesystem,
// and the exec and proxy WebSockets, speaking the protocol the SDK expects.
type spritesAPI struct {
	t *testing.T
	*httptest.Server

	mu          sync.Mutex
	requests    []string
	sprites     map[string]*fakeSprite
	execQueries []url.Values

	// exec handles a new command. The default prints nothing and exits 0.
	exec func(c *execConn)

	// failExec makes that many exec connections fail with this status first.
	failExec       int
	failExecStatus int

	// tcp handles a proxied TCP connection to a port in the sprite. The
	// default echoes.
	tcp func(port int, conn io.ReadWriter)
}

type fakeSprite struct {
	info        map[string]any
	files       map[string]*fakeFile
	dirs        map[string]bool
	checkpoints []map[string]any
	sessions    []map[string]any
	killed      []string

	// unreadable paths list as files but fail to read, as symlinks to
	// directories and dangling symlinks do.
	unreadable map[string]bool
}

type fakeFile struct {
	data []byte
	mode string
}

func newSpritesAPI(t *testing.T) *spritesAPI {
	t.Helper()

	api := &spritesAPI{t: t, sprites: map[string]*fakeSprite{}}
	api.Server = httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(api.Close)

	return api
}

// addSprite registers a sprite and returns it for the test to fill in.
func (a *spritesAPI) addSprite(name string) *fakeSprite {
	a.mu.Lock()
	defer a.mu.Unlock()

	sp := &fakeSprite{
		info: map[string]any{
			"id":           "sprite-" + name,
			"name":         name,
			"organization": "acme",
			"status":       "running",
			"url":          "https://" + name + ".sprites.app",
			"created_at":   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		files:      map[string]*fakeFile{},
		dirs:       map[string]bool{"/": true, "/tmp": true, "/home": true, remoteHome: true},
		unreadable: map[string]bool{},
	}
	a.sprites[name] = sp

	return sp
}

func (a *spritesAPI) session() *session {
	return &session{
		Org:    "acme",
		APIURL: a.URL,
		Token:  "test-token",
		Client: sprites.New("test-token", sprites.WithBaseURL(a.URL), sprites.WithDisableControl()),
	}
}

// requested returns the requests seen so far, as "METHOD /path?query".
func (a *spritesAPI) requested() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.requests...)
}

func (a *spritesAPI) lastExecQuery() url.Values {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.execQueries) == 0 {
		return nil
	}

	return a.execQueries[len(a.execQueries)-1]
}

func (sp *fakeSprite) putFile(name, data, mode string) {
	sp.files[name] = &fakeFile{data: []byte(data), mode: mode}

	for dir := path.Dir(name); ; dir = path.Dir(dir) {
		sp.dirs[dir] = true

		if dir == "/" {
			return
		}
	}
}

func jsonReply(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func notFound(w http.ResponseWriter, what string) {
	jsonReply(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": what + " not found"})
}

func (a *spritesAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.requests = append(a.requests, r.Method+" "+r.URL.RequestURI())
	a.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer test-token" {
		jsonReply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "message": "authentication failed"})
		return
	}

	rest, ok := strings.CutPrefix(r.URL.Path, "/v1/sprites")
	if !ok {
		notFound(w, "route")
		return
	}

	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if parts[0] == "" {
		a.serveCollection(w, r)
		return
	}

	a.mu.Lock()
	sp := a.sprites[parts[0]]
	a.mu.Unlock()

	if sp == nil {
		notFound(w, "sprite")
		return
	}

	a.serveSprite(w, r, parts[0], sp, parts[1:])
}

func (a *spritesAPI) serveCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name   string   `json:"name"`
			Labels []string `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		sp := a.addSprite(req.Name)
		sp.info["status"] = "cold"

		if len(req.Labels) > 0 {
			sp.info["labels"] = req.Labels
		}

		jsonReply(w, http.StatusCreated, map[string]string{"name": req.Name})
	case http.MethodGet:
		a.mu.Lock()
		defer a.mu.Unlock()

		prefix := r.URL.Query().Get("prefix")

		var names []string
		for name := range a.sprites {
			if strings.HasPrefix(name, prefix) {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		list := make([]map[string]any, 0, len(names))
		for _, name := range names {
			list = append(list, a.sprites[name].info)
		}

		jsonReply(w, http.StatusOK, map[string]any{
			"sprites": list,
			"org":     map[string]any{"name": "acme", "running": len(list), "running_limit": 10, "warm_limit": 10},
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *spritesAPI) serveSprite(w http.ResponseWriter, r *http.Request, name string, sp *fakeSprite, parts []string) {
	route := strings.Join(parts, "/")

	switch {
	case route == "" && r.Method == http.MethodGet:
		a.mu.Lock()
		defer a.mu.Unlock()

		jsonReply(w, http.StatusOK, sp.info)
	case route == "" && r.Method == http.MethodDelete:
		a.mu.Lock()
		delete(a.sprites, name)
		a.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	case route == "" && r.Method == http.MethodPut:
		var req struct {
			URLSettings map[string]any `json:"url_settings"`
			Labels      []string       `json:"labels"`
			ClearLabels bool           `json:"clear_labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		a.mu.Lock()
		defer a.mu.Unlock()

		if req.URLSettings != nil {
			sp.info["url_settings"] = req.URLSettings
		}

		if req.ClearLabels {
			delete(sp.info, "labels")
		}

		if len(req.Labels) > 0 {
			sp.info["labels"] = req.Labels
		}

		jsonReply(w, http.StatusOK, sp.info)
	case route == "checkpoint" && r.Method == http.MethodPost:
		var req struct {
			Comment string `json:"comment"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		a.mu.Lock()
		id := fmt.Sprintf("v%d", len(sp.checkpoints)+1)
		sp.checkpoints = append(sp.checkpoints, map[string]any{
			"id": id, "create_time": time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "comment": req.Comment,
		})
		a.mu.Unlock()

		ndjson(w,
			map[string]string{"type": "info", "data": "Creating checkpoint..."},
			map[string]string{"type": "info", "data": "  ID: " + id},
			map[string]string{"type": "complete", "data": "done"},
		)
	case route == "checkpoints" && r.Method == http.MethodGet:
		a.mu.Lock()
		defer a.mu.Unlock()

		list := sp.checkpoints
		if list == nil {
			list = []map[string]any{}
		}

		jsonReply(w, http.StatusOK, list)
	case len(parts) >= 2 && parts[0] == "checkpoints":
		a.serveCheckpoint(w, r, sp, parts[1:])
	case parts[0] == "exec":
		a.serveExec(w, r, sp, parts[1:])
	case route == "proxy":
		a.serveProxy(w, r)
	case len(parts) == 2 && parts[0] == "fs":
		a.serveFS(w, r, sp, parts[1])
	default:
		notFound(w, "route")
	}
}

func ndjson(w http.ResponseWriter, messages ...map[string]string) {
	w.Header().Set("Content-Type", "application/x-ndjson")

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			panic(err)
		}

		_, _ = w.Write(append(data, '\n'))
	}
}

func (a *spritesAPI) serveCheckpoint(w http.ResponseWriter, r *http.Request, sp *fakeSprite, parts []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	index := -1
	for i, cp := range sp.checkpoints {
		if cp["id"] == parts[0] {
			index = i
		}
	}

	if index < 0 {
		notFound(w, "checkpoint")
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		jsonReply(w, http.StatusOK, sp.checkpoints[index])
	case len(parts) == 1 && r.Method == http.MethodDelete:
		sp.checkpoints = append(sp.checkpoints[:index], sp.checkpoints[index+1:]...)
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 2 && parts[1] == "restore" && r.Method == http.MethodPost:
		ndjson(w,
			map[string]string{"type": "info", "data": "Restoring..."},
			map[string]string{"type": "warning", "data": "active sessions were ended"},
			map[string]string{"type": "complete", "data": "done"},
		)
	default:
		notFound(w, "route")
	}
}

func (a *spritesAPI) serveFS(w http.ResponseWriter, r *http.Request, sp *fakeSprite, op string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	query := r.URL.Query()

	abs := query.Get("path")
	if !path.IsAbs(abs) {
		abs = path.Join(query.Get("workingDir"), abs)
	}
	abs = path.Clean(abs)

	entry := func(p string) map[string]any {
		if sp.dirs[p] {
			return map[string]any{"name": path.Base(p), "path": p, "type": "directory", "isDir": true, "mode": "755"}
		}

		mode, size := "777", 0
		if file := sp.files[p]; file != nil {
			mode, size = file.mode, len(file.data)
		}

		return map[string]any{"name": path.Base(p), "path": p, "type": "file", "isDir": false, "mode": mode, "size": size}
	}

	switch op {
	case "list":
		switch {
		case sp.dirs[abs]:
			seen := map[string]bool{}

			for _, set := range []func(func(string)){
				func(add func(string)) {
					for p := range sp.files {
						add(p)
					}
				},
				func(add func(string)) {
					for p := range sp.dirs {
						add(p)
					}
				},
				func(add func(string)) {
					for p := range sp.unreadable {
						add(p)
					}
				},
			} {
				set(func(p string) {
					if p != abs && path.Dir(p) == abs {
						seen[p] = true
					}
				})
			}

			children := make([]string, 0, len(seen))
			for p := range seen {
				children = append(children, p)
			}
			sort.Strings(children)

			entries := make([]map[string]any, 0, len(children))
			for _, p := range children {
				entries = append(entries, entry(p))
			}

			jsonReply(w, http.StatusOK, map[string]any{"path": abs, "entries": entries, "count": len(entries)})
		case sp.files[abs] != nil || sp.unreadable[abs]:
			jsonReply(w, http.StatusOK, map[string]any{"path": abs, "entries": []map[string]any{entry(abs)}, "count": 1})
		default:
			jsonReply(w, http.StatusNotFound, map[string]string{"error": "file does not exist"})
		}
	case "read":
		file := sp.files[abs]

		switch {
		case sp.unreadable[abs]:
			jsonReply(w, http.StatusBadRequest, map[string]string{"error": "cannot read directory"})
		case file == nil:
			jsonReply(w, http.StatusNotFound, map[string]string{"error": "file does not exist"})
		default:
			_, _ = w.Write(file.data)
		}
	case "write":
		data, _ := io.ReadAll(r.Body)
		mode, _ := strconv.ParseUint(query.Get("mode"), 8, 32)
		sp.putFile(abs, string(data), fmt.Sprintf("%o", mode))

		jsonReply(w, http.StatusOK, map[string]any{"path": abs, "size": len(data)})
	default:
		jsonReply(w, http.StatusNotFound, map[string]string{"error": "unknown fs operation"})
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (a *spritesAPI) serveExec(w http.ResponseWriter, r *http.Request, sp *fakeSprite, parts []string) {
	// POST /exec/<id>/kill
	if len(parts) == 2 && parts[1] == "kill" && r.Method == http.MethodPost {
		a.mu.Lock()
		sp.killed = append(sp.killed, parts[0]+" "+r.URL.RawQuery)
		a.mu.Unlock()

		ndjson(w,
			map[string]string{"type": "signal", "message": "Signaling " + r.URL.Query().Get("signal")},
			map[string]string{"type": "exited", "message": "Process exited"},
		)

		return
	}

	if !websocket.IsWebSocketUpgrade(r) {
		a.mu.Lock()
		defer a.mu.Unlock()

		sessions := sp.sessions
		if sessions == nil {
			sessions = []map[string]any{}
		}

		jsonReply(w, http.StatusOK, map[string]any{"sessions": sessions})

		return
	}

	query := r.URL.Query()
	if len(parts) == 1 {
		query.Set("id", parts[0])
	}

	a.mu.Lock()
	a.execQueries = append(a.execQueries, query)

	fail := a.failExec > 0
	if fail {
		a.failExec--
	}

	status, handler := a.failExecStatus, a.exec
	a.mu.Unlock()

	if fail {
		w.WriteHeader(status)
		return
	}

	conn, err := upgrader.Upgrade(w, r, http.Header{"X-Sprite-Capabilities": {"signal"}})
	if err != nil {
		return
	}
	defer conn.Close() //nolint:errcheck

	c := &execConn{t: a.t, conn: conn, query: query, tty: query.Get("tty") == "true", messages: make(chan execMessage, 64)}
	go c.readLoop()

	if handler == nil {
		handler = func(c *execConn) { c.exit(0) }
	}

	handler(c)

	// Let the client read the exit before the connection goes away.
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))

	for range c.messages { //nolint:revive
	}
}

// execConn is the server side of one exec WebSocket.
type execConn struct {
	t     *testing.T
	conn  *websocket.Conn
	query url.Values
	tty   bool

	writeMu  sync.Mutex
	messages chan execMessage
}

// execMessage is something the client sent: stdin data, the end of stdin, or
// a control message such as a resize or a signal.
type execMessage struct {
	stdin    []byte
	stdinEOF bool
	control  map[string]any
}

func (c *execConn) readLoop() {
	defer close(c.messages)

	for {
		kind, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		switch {
		case kind == websocket.TextMessage:
			var control map[string]any
			if json.Unmarshal(data, &control) == nil {
				c.messages <- execMessage{control: control}
			}
		case c.tty:
			c.messages <- execMessage{stdin: data}
		case len(data) > 0 && data[0] == 0:
			c.messages <- execMessage{stdin: data[1:]}
		case len(data) > 0 && data[0] == 4:
			c.messages <- execMessage{stdinEOF: true}
		}
	}
}

func (c *execConn) write(kind int, data []byte) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = c.conn.WriteMessage(kind, data)
}

func (c *execConn) stdout(s string) {
	if c.tty {
		c.write(websocket.BinaryMessage, []byte(s))
		return
	}

	c.write(websocket.BinaryMessage, append([]byte{1}, s...))
}

func (c *execConn) stderr(s string) {
	c.write(websocket.BinaryMessage, append([]byte{2}, s...))
}

func (c *execConn) text(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	c.write(websocket.TextMessage, data)
}

func (c *execConn) exit(code int) {
	if c.tty {
		c.text(map[string]any{"type": "exit", "exit_code": code})
		return
	}

	c.write(websocket.BinaryMessage, []byte{3, byte(code)})
}

// readStdin collects stdin until the client says it has ended.
func (c *execConn) readStdin() string {
	var input []byte

	for msg := range c.messages {
		if msg.stdinEOF {
			break
		}

		input = append(input, msg.stdin...)
	}

	return string(input)
}

// waitControl waits for a control message of the given type.
func (c *execConn) waitControl(kind string) map[string]any {
	timeout := time.After(10 * time.Second)

	for {
		select {
		case msg, ok := <-c.messages:
			if !ok {
				return nil
			}

			if msg.control != nil && msg.control["type"] == kind {
				return msg.control
			}
		case <-timeout:
			assert.Fail(c.t, "timed out waiting for a "+kind+" control message")
			return nil
		}
	}
}

func (a *spritesAPI) serveProxy(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close() //nolint:errcheck

	var init struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	if err := conn.ReadJSON(&init); err != nil {
		return
	}

	_ = conn.WriteJSON(map[string]string{
		"status": "connected",
		"target": net.JoinHostPort(init.Host, strconv.Itoa(init.Port)),
	})

	a.mu.Lock()
	handler := a.tcp
	a.mu.Unlock()

	if handler == nil {
		handler = func(_ int, conn io.ReadWriter) { _, _ = io.Copy(conn, conn) }
	}

	handler(init.Port, &wsStream{conn: conn})

	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
}

// wsStream reads and writes a WebSocket's binary messages as a byte stream.
type wsStream struct {
	conn *websocket.Conn
	rest []byte
}

func (s *wsStream) Read(p []byte) (int, error) {
	for len(s.rest) == 0 {
		kind, data, err := s.conn.ReadMessage()
		if err != nil {
			return 0, io.EOF
		}

		if kind == websocket.BinaryMessage {
			s.rest = data
		}
	}

	n := copy(p, s.rest)
	s.rest = s.rest[n:]

	return n, nil
}

func (s *wsStream) Write(p []byte) (int, error) {
	if err := s.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// sessionWithToken is a session for api that presents a different token.
func sessionWithToken(api *spritesAPI, token string) *session {
	s := api.session()
	s.Token = token
	s.Client = sprites.New(token, sprites.WithBaseURL(api.URL), sprites.WithDisableControl())

	return s
}
