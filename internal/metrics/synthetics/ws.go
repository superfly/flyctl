package synthetics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	metrics "github.com/superfly/flyctl/internal/metrics"
	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

type SyntheticsWs struct {
	atime  time.Time
	lock   sync.RWMutex
	reset  chan bool
	wsConn *websocket.Conn
	limit  *rate.Limiter
}

func NewMetricsWs() (*SyntheticsWs, error) {
	return &SyntheticsWs{
		atime: time.Now(),
		reset: make(chan bool),
		limit: rate.NewLimiter(rate.Every(5*time.Second), 2),
	}, nil
}

func getFlyntheticsWsUrl(ctx context.Context) string {
	cfg := config.FromContext(ctx)
	return fmt.Sprintf("%s/ws", cfg.SyntheticsBaseURL)
}

func (ws *SyntheticsWs) Connect(ctx context.Context) error {
	rurl := getFlyntheticsWsUrl(ctx)

	log.Printf("(re-)connecting synthetics agent to %s", rurl)

	authToken, err := metrics.GetMetricsToken(ctx)
	if err != nil {
		return err
	}

	headers := http.Header{}
	headers.Set("Authorization", authToken)
	headers.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	opts := &websocket.DialOptions{
		HTTPHeader: headers,
	}

	wsConn, _, err := websocket.Dial(ctx, rurl, opts)
	if err != nil {
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
