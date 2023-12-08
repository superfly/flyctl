package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/superfly/flyctl/api/tokens"
	"github.com/superfly/graphql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	baseURL          string
	errorLog         bool
	instrumenter     InstrumentationService
	defaultTransport http.RoundTripper = http.DefaultTransport
)

var contextKeyAction = contextKey("gql_action")

func ctxWithAction(ctx context.Context, action string) context.Context {
	return context.WithValue(ctx, contextKeyAction, action)
}

func actionFromCtx(ctx context.Context) string {
	action := ctx.Value(contextKeyAction)
	if action != nil {
		return action.(string)
	}
	return "unknown_actiom"
}

// SetBaseURL - Sets the base URL for the API
func SetBaseURL(url string) {
	baseURL = url
}

// SetErrorLog - Sets whether errors should be loddes
func SetErrorLog(log bool) {
	errorLog = log
}

func SetInstrumenter(i InstrumentationService) {
	instrumenter = i
}

func SetTransport(t http.RoundTripper) {
	defaultTransport = t
}

type InstrumentationService interface {
	ReportCallTiming(duration time.Duration)
}

// Client - API client encapsulating the http and GraphQL clients
type Client struct {
	httpClient *http.Client
	client     *graphql.Client
	GenqClient genq.Client
	tokens     *tokens.Tokens
	logger     Logger
}

func (c *Client) Authenticated() bool {
	return c.tokens.GraphQL() != ""
}

// NewClient - creates a new Client, takes an access token
func NewClient(accessToken, name, version string, logger Logger) *Client {
	return NewClientFromOptions(ClientOptions{
		AccessToken: accessToken,
		Name:        name,
		Version:     version,
		Logger:      logger,
		BaseURL:     baseURL,
	})
}

type ClientOptions struct {
	AccessToken      string
	Tokens           *tokens.Tokens
	Name             string
	Version          string
	BaseURL          string
	Logger           Logger
	EnableDebugTrace *bool
	Transport        *Transport
}

func (opts ClientOptions) tokens() *tokens.Tokens {
	if opts.Tokens == nil {
		opts.Tokens = tokens.Parse(opts.AccessToken)
	}

	return opts.Tokens
}

func (t *Transport) setDefaults(opts *ClientOptions) {
	if t.UnderlyingTransport == nil {
		t.UnderlyingTransport = defaultTransport
	}
	if t.Tokens == nil && t.Token == "" {
		t.Tokens = opts.tokens()
	}
	if t.UserAgent == "" {
		t.UserAgent = fmt.Sprintf("%s/%s", opts.Name, opts.Version)
	}
	if opts.EnableDebugTrace != nil {
		t.EnableDebugTrace = *opts.EnableDebugTrace
	} else {
		v := os.Getenv("FLY_FORCE_TRACE")
		t.EnableDebugTrace = !(v == "" || v == "0" || v == "false")
	}
}

func NewClientFromOptions(opts ClientOptions) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = baseURL
	}

	transport := opts.Transport
	if transport == nil {
		transport = &Transport{}
	}
	transport.setDefaults(&opts)

	httpClient, _ := NewHTTPClient(opts.Logger, transport)
	url := fmt.Sprintf("%s/graphql", opts.BaseURL)
	client := graphql.NewClient(url, graphql.WithHTTPClient(httpClient))
	genqClient := genq.NewClient(url, httpClient)

	return &Client{httpClient, client, genqClient, opts.tokens(), opts.Logger}
}

// NewRequest - creates a new GraphQL request
func (*Client) NewRequest(q string) *graphql.Request {
	q = compactQueryString(q)
	return graphql.NewRequest(q)
}

// Run - Runs a GraphQL request
func (c *Client) Run(req *graphql.Request) (Query, error) {
	return c.RunWithContext(context.Background(), req)
}

func (c *Client) Logger() Logger { return c.logger }

func (c *Client) getRequestType(r *graphql.Request) string {
	query := r.Query()

	if strings.Contains(query, "mutation") {
		return "mutation"
	}

	if strings.Contains(query, "query") {
		return "query"
	}
	return "unknown"
}
func (c *Client) getErrorFromErrors(errors Errors) string {
	errs := []string{}
	for _, err := range errors {
		errs = append(errs, err.Message)
	}

	return strings.Join(errs, ",")
}

// RunWithContext - Runs a GraphQL request within a Go context
func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request) (Query, error) {
	tracer := otel.GetTracerProvider().Tracer("github.com/superfly/flyctl/api")
	ctx, span := tracer.Start(ctx, fmt.Sprintf("web.%s", actionFromCtx(ctx)), trace.WithAttributes(
		attribute.String("request.action", actionFromCtx(ctx)),
		attribute.String("request.type", c.getRequestType(req)),
	))
	defer span.End()

	if instrumenter != nil {
		start := time.Now()
		defer func() {
			instrumenter.ReportCallTiming(time.Since(start))
		}()
	}

	var resp Query
	err := c.client.Run(ctx, req, &resp)

	if resp.Errors != nil {
		span.RecordError(fmt.Errorf(c.getErrorFromErrors(resp.Errors)))
		span.SetStatus(codes.Error, "failed to do grapqhl request")
	}

	if resp.Errors != nil && errorLog {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", resp.Errors)
	}

	return resp, err
}

var compactPattern = regexp.MustCompile(`\s+`)

func compactQueryString(q string) string {
	q = strings.TrimSpace(q)
	return compactPattern.ReplaceAllString(q, " ")
}

// GetAccessToken - uses email, password and possible otp to get token
func GetAccessToken(ctx context.Context, email, password, otp string) (token string, err error) {
	var postData bytes.Buffer
	if err = json.NewEncoder(&postData).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]string{
				"email":    email,
				"password": password,
				"otp":      otp,
			},
		},
	}); err != nil {
		return
	}

	url := fmt.Sprintf("%s/api/v1/sessions", baseURL)

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, &postData); err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	if res, err = http.DefaultClient.Do(req); err != nil {
		return
	}
	defer func() {
		closeErr := res.Body.Close()
		if err == nil {
			err = closeErr
		}
	}()

	switch {
	case res.StatusCode >= http.StatusInternalServerError:
		err = errors.New("An unknown server error occurred, please try again")
	case res.StatusCode >= http.StatusBadRequest:
		err = errors.New("Incorrect email and password combination")
	default:
		var result map[string]map[string]map[string]string

		if err = json.NewDecoder(res.Body).Decode(&result); err == nil {
			token = result["data"]["attributes"]["access_token"]
		}
	}

	return
}

type Transport struct {
	UnderlyingTransport http.RoundTripper
	UserAgent           string
	Token               string // deprecated
	Tokens              *tokens.Tokens
	EnableDebugTrace    bool
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.addAuthorization(req)

	req.Header.Set("User-Agent", t.UserAgent)
	if t.EnableDebugTrace {
		req.Header.Set("Fly-Force-Trace", "true")
	}
	return t.UnderlyingTransport.RoundTrip(req)
}

func (t *Transport) tokens() *tokens.Tokens {
	if t.Tokens == nil {
		t.Tokens = tokens.Parse(t.Token)
	}
	return t.Tokens
}

func (t *Transport) addAuthorization(req *http.Request) {
	hdr, ok := req.Context().Value(contextKeyAuthorization).(string)
	if !ok {
		hdr = t.tokens().GraphQLHeader()
	}
	req.Header.Set("Authorization", hdr)
}
