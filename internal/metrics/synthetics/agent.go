package synthetics

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	dto "github.com/prometheus/client_model/go"
	"github.com/superfly/flyctl/internal/logger"
)

func RunAgent(ctx context.Context) error {
	logger := logger.FromContext(ctx)
	logger.Info("running synthetics agent")
	ws, err := NewMetricsWs()
	if err != nil {
		return err
	}

	if err = ws.Connect(ctx); err != nil {
		logger.Errorf("error connecting to flynthetics: %v", err)
		return err
	}

	go func() {
		defer ws.wsConn.CloseNow()
		c := make(chan os.Signal, 1)
		signalChannel(c)

		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()

		reconnectAt := time.Time{}

		for {
			select {
			case <-tick.C:
				if !reconnectAt.IsZero() && reconnectAt.Before(time.Now()) {
					ws.lock.Lock()
					ws.Connect(ctx)
					ws.lock.Unlock()

					reconnectAt = time.Time{}
				}

			case <-ctx.Done():
				return

			case <-c:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}

			case <-ws.reset:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}
			}
		}
	}()

	go ws.listen(ctx)

	return nil
}

type ProbeMessage struct {
	Module     string `json:"module"`
	Target     string `json:"target"`
	IPProtocol string `json:"ip_protocol"`
}

func processProbe(ctx context.Context, probeMessageJSON []byte, ws *SyntheticsWs) error {
	logger := logger.FromContext(ctx)
	logger.Debug("proccessing probes")

	probeMessage := ProbeMessage{}
	err := json.Unmarshal(probeMessageJSON, &probeMessage)
	if err != nil {
		logger.Error("JSON parse error:", err)
		return err
	}

	var (
		buf    bytes.Buffer
		mfs    []*dto.MetricFamily
		logBuf bytes.Buffer
	)

	sl := log.NewLogfmtLogger(log.NewSyncWriter(&logBuf))

	if !isFlyInfraTarget(probeMessage.Target) {
		logger.Warnf("skipping probe message for non-fly infra endpoint %s", probeMessage.Target)
	} else {
		mfs, err = probeHTTP(ctx, probeMessage, sl)
		if err != nil {
			logger.Errorf("error processing probe for endpoint %s. error: %v", probeMessage.Target, err)
			return err
		}
	}

	encoder := json.NewEncoder(&buf)
	err = encoder.Encode(mfs)
	if err != nil {
		return err
	}

	data := buf.Bytes()
	err = ws.write(ctx, data)
	if err != nil {
		return err
	}

	logger.Debugf("probe result sent to server. log: %s", &logBuf)

	return nil
}

func isFlyInfraTarget(target string) bool {
	var flyInfraDomains = []string{
		"fly.io",
		"flyio.net",
		"machines.dev",
	}

	parsedURL, err := url.Parse(target)
	if err != nil {
		return false
	}

	host, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		host = parsedURL.Host
	}

	for _, flyInfraDomain := range flyInfraDomains {
		if strings.HasSuffix(host, "."+flyInfraDomain) || host == flyInfraDomain {
			return true
		}
	}

	return false
}
