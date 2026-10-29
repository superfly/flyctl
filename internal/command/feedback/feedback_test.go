package feedback

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/uiex"
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
