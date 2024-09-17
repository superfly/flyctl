package synthetics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"golang.org/x/time/rate"
)

type SyntheticsWs struct {
	atime     time.Time
	lock      sync.RWMutex
	reset     chan bool
	wsConn    *websocket.Conn
	limit     *rate.Limiter
	authToken string
}

func NewMetricsWs() (*SyntheticsWs, error) {
	return &SyntheticsWs{
		atime:     time.Now(),
		reset:     make(chan bool),
		limit:     rate.NewLimiter(rate.Every(5*time.Second), 2),
		authToken: "",
	}, nil
}

func getFlyntheticsWsUrl(ctx context.Context) string {
	cfg := config.FromContext(ctx)
	return fmt.Sprintf("%s/ws", cfg.SyntheticsBaseURL)
}

func (ws *SyntheticsWs) Connect(ctx context.Context) error {
	rurl := getFlyntheticsWsUrl(ctx)

	log.Printf("(re-)connecting synthetics agent to %s", rurl)

	if ws.authToken == "" {
		authToken, err := GetSyntheticsToken(ctx)
		if err != nil {
			return err
		}
		ws.authToken = authToken
	}

	headers := http.Header{}
	headers.Set("Authorization", ws.authToken)
	headers.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	opts := &websocket.DialOptions{
		HTTPHeader: headers,
	}

	wsConn, resp, err := websocket.Dial(ctx, rurl, opts)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			// Handle 401 Unauthorized
			log.Printf("Unauthorized, resetting token.")
			ws.authToken = ""
		} else if resp != nil {
			// Handle other HTTP errors
			log.Printf("HTTP error: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		} else {
			// Handle non-HTTP errors
			log.Printf("Dial failed: %v", err)
		}
		return fmt.Errorf("error connecting synthetics agent to fynthetics: %w", err)
	}

	if ws.wsConn != nil {
		_ = ws.wsConn.CloseNow()
	}
	ws.wsConn = wsConn
	log.Printf("synthetics agent connected to %s", rurl)

	return nil
}

func (ws *SyntheticsWs) resetConn(c *websocket.Conn, err error) {
	ws.lock.RLock()
	cur := ws.wsConn
	ws.lock.RUnlock()

	if cur != c {
		return
	}

	ws.limit.Wait(context.Background())

	log.Printf("resetting synthetics agent connection due to error: %s", err)
	ws.reset <- true
}

func (ws *SyntheticsWs) listen(ctx context.Context) error {
	logger := logger.FromContext(ctx)
	logger.Debug("start listening for probes")
	for ctx.Err() == nil {
		ws.lock.RLock()
		c := ws.wsConn
		ws.lock.RUnlock()

		_, probeMessageJSON, err := c.Read(ctx)
		if err != nil {
			logger.Error("read error: ", err)
			ws.resetConn(c, err)
			continue
		}

		logger.Debug("received from server", string(probeMessageJSON))

		err = processProbe(ctx, probeMessageJSON, ws)
		if err != nil {
			logger.Error("failed processing probe", err)
		}

	}
	logger.Debug("stop listening for probes")

	return ctx.Err()
}

func (ws *SyntheticsWs) write(ctx context.Context, data []byte) (err error) {
	logger := logger.FromContext(ctx)
	ws.lock.RLock()
	c := ws.wsConn
	ws.lock.RUnlock()

	err = c.Write(ctx, websocket.MessageText, data)
	if err != nil {
		logger.Error("write error: ", err)
		ws.resetConn(c, err)
		return err
	}

	return nil
}
