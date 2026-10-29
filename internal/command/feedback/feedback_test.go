package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		in      uiex.FeedbackRequest
		wantErr string
	}{
		{name: "ok with request id", in: uiex.FeedbackRequest{Category: "bug", RequestID: "r"}},
		{name: "ok with url", in: uiex.FeedbackRequest{Category: "feature_request", URL: "https://fly.io"}},
		{name: "missing category", in: uiex.FeedbackRequest{RequestID: "r"}, wantErr: "--category is required"},
		{name: "bad category", in: uiex.FeedbackRequest{Category: "praise", RequestID: "r"}, wantErr: "invalid --category"},
		{name: "no target", in: uiex.FeedbackRequest{Category: "bug"}, wantErr: "--request-id or --url"},
		{name: "request id too long", in: uiex.FeedbackRequest{Category: "bug", RequestID: strings.Repeat("a", maxRequestIDLength+1)}, wantErr: "--request-id is too long"},
		{name: "url too long", in: uiex.FeedbackRequest{Category: "bug", URL: strings.Repeat("a", maxURLLength+1)}, wantErr: "--url is too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTarget(tt.in)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateNote(t *testing.T) {
	require.NoError(t, validateNote("it broke"))
	require.NoError(t, validateNote(strings.Repeat("é", maxNoteLength)))
	require.ErrorContains(t, validateNote(""), "can't be empty")
	require.ErrorContains(t, validateNote(strings.Repeat("a", maxNoteLength+1)), "too long")
}

func TestReadNote(t *testing.T) {
	t.Run("argument wins over stdin", func(t *testing.T) {
		ios, in, _, _ := iostreams.Test()
		in.WriteString("from stdin")
		ctx := iostreams.NewContext(context.Background(), ios)

		note, err := readNote(ctx, []string{"  from arg \n"})
		require.NoError(t, err)
		assert.Equal(t, "from arg", note)
	})

	t.Run("piped stdin", func(t *testing.T) {
		ios, in, _, _ := iostreams.Test()
		in.WriteString("from stdin\n")
		ctx := iostreams.NewContext(context.Background(), ios)

		note, err := readNote(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, "from stdin", note)
	})

	t.Run("tty without prompting is an error", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetNeverPrompt(true)
		ctx := iostreams.NewContext(context.Background(), ios)

		_, err := readNote(ctx, nil)
		require.ErrorContains(t, err, "note is required")
	})
}

func newRunContext(t *testing.T, args []string, stdin string, jsonOutput bool, client *mock.UiexClient) (context.Context, *bytes.Buffer) {
	t.Helper()

	ios, in, out, _ := iostreams.Test()
	in.WriteString(stdin)

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("category", "", "")
	flags.String("request-id", "", "")
	flags.String("url", "", "")
	require.NoError(t, flags.Parse(args))

	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = flagctx.NewContext(ctx, flags)
	ctx = uiexutil.NewContextWithClient(ctx, client)

	return ctx, out
}

func TestRun(t *testing.T) {
	t.Run("submits the report and prints the ID", func(t *testing.T) {
		var got uiex.FeedbackRequest
		ctx, out := newRunContext(t,
			[]string{"--category", "bug", "--request-id", "req-1", "--url", "https://fly.io"},
			"it broke\n", false,
			&mock.UiexClient{
				SubmitFeedbackFunc: func(_ context.Context, in uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
					got = in
					return &uiex.FeedbackResponse{FeedbackID: "fb-1"}, nil
				},
			})

		require.NoError(t, run(ctx))
		assert.Equal(t, uiex.FeedbackRequest{Category: "bug", Note: "it broke", RequestID: "req-1", URL: "https://fly.io"}, got)
		assert.Equal(t, "Thanks! Feedback submitted (ID: fb-1)\n", out.String())
	})

	t.Run("reports an already submitted duplicate", func(t *testing.T) {
		ctx, out := newRunContext(t,
			[]string{"--category", "other", "--url", "https://fly.io", "a note"},
			"", false,
			&mock.UiexClient{
				SubmitFeedbackFunc: func(_ context.Context, in uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
					assert.Equal(t, "a note", in.Note)
					return &uiex.FeedbackResponse{FeedbackID: "fb-1", AlreadySubmitted: true}, nil
				},
			})

		require.NoError(t, run(ctx))
		assert.Equal(t, "This feedback was already submitted (ID: fb-1)\n", out.String())
	})

	t.Run("json output", func(t *testing.T) {
		ctx, out := newRunContext(t,
			[]string{"--category", "bug", "--request-id", "req-1"},
			"it broke", true,
			&mock.UiexClient{
				SubmitFeedbackFunc: func(context.Context, uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
					return &uiex.FeedbackResponse{FeedbackID: "fb-1"}, nil
				},
			})

		require.NoError(t, run(ctx))

		var res uiex.FeedbackResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &res))
		assert.Equal(t, uiex.FeedbackResponse{FeedbackID: "fb-1"}, res)
	})

	t.Run("propagates API errors", func(t *testing.T) {
		ctx, out := newRunContext(t,
			[]string{"--category", "bug", "--request-id", "req-1"},
			"it broke", false,
			&mock.UiexClient{
				SubmitFeedbackFunc: func(context.Context, uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
					return nil, errors.New("boom")
				},
			})

		require.ErrorContains(t, run(ctx), "boom")
		assert.Empty(t, out.String())
	})

	t.Run("invalid flags never reach the API", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--category", "praise", "--request-id", "req-1", "note"},
			"", false,
			&mock.UiexClient{
				SubmitFeedbackFunc: func(context.Context, uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
					t.Fatal("SubmitFeedback should not be called")
					return nil, nil
				},
			})

		require.ErrorContains(t, run(ctx), "invalid --category")
	})

	t.Run("missing category never reaches the API", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--request-id", "req-1", "note"},
			"", false, failOnSubmit(t))

		require.ErrorContains(t, run(ctx), "--category is required")
	})

	t.Run("missing request id and url never reaches the API", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--category", "bug", "note"},
			"", false, failOnSubmit(t))

		require.ErrorContains(t, run(ctx), "--request-id or --url")
	})

	t.Run("empty note never reaches the API", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--category", "bug", "--request-id", "req-1"},
			" \n", false, failOnSubmit(t))

		require.ErrorContains(t, run(ctx), "can't be empty")
	})

	t.Run("too long note never reaches the API", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--category", "bug", "--request-id", "req-1", strings.Repeat("a", maxNoteLength+1)},
			"", false, failOnSubmit(t))

		require.ErrorContains(t, run(ctx), "too long")
	})

	t.Run("flags are validated before stdin is read", func(t *testing.T) {
		ctx, _ := newRunContext(t,
			[]string{"--category", "praise", "--request-id", "req-1"},
			"", false, failOnSubmit(t))

		ios, _, _, _ := iostreams.Test()
		stdin := &readRecorder{}
		ios.In = stdin
		ctx = iostreams.NewContext(ctx, ios)

		require.ErrorContains(t, run(ctx), "invalid --category")
		assert.False(t, stdin.read, "stdin should not be read when the flags are invalid")
	})
}

// failOnSubmit returns a client that fails the test if the API is called.
func failOnSubmit(t *testing.T) *mock.UiexClient {
	t.Helper()

	return &mock.UiexClient{
		SubmitFeedbackFunc: func(context.Context, uiex.FeedbackRequest) (*uiex.FeedbackResponse, error) {
			t.Error("SubmitFeedback should not be called")

			return nil, errors.New("unexpected call")
		},
	}
}

// readRecorder is a stdin that records whether it was read.
type readRecorder struct{ read bool }

func (r *readRecorder) Read([]byte) (int, error) {
	r.read = true

	return 0, io.EOF
}

func (r *readRecorder) Close() error { return nil }
