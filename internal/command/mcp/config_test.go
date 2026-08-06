package mcp

import (
	"testing"
)

func TestUnmarshalJSONC(t *testing.T) {
	t.Run("plain JSON, no comments", func(t *testing.T) {
		input := []byte(`{"name": "Alice", "n": 42}`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "Alice" || got["n"].(float64) != 42 {
			t.Errorf("unexpected unmarshal result: %v", got)
		}
	})

	t.Run("with line comments", func(t *testing.T) {
		input := []byte(`{
            // top-level comment
            "name": "Alice", // trailing comment
            "n": 42
        }`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "Alice" || got["n"].(float64) != 42 {
			t.Errorf("unexpected unmarshal result: %v", got)
		}
	})

	t.Run("with block comments", func(t *testing.T) {
		input := []byte(`{
            /* top-level block comment */
            "name": "Alice",
            /* multi-line
               block comment */
            "n": 42
        }`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "Alice" || got["n"].(float64) != 42 {
			t.Errorf("unexpected unmarshal result: %v", got)
		}
	})

	t.Run("with trailing commas", func(t *testing.T) {
		input := []byte(`{
            "name": "Alice",
            "items": [1, 2, 3,],
            "n": 42,
        }`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "Alice" || got["n"].(float64) != 42 {
			t.Errorf("unexpected unmarshal result: %v", got)
		}
		if items, ok := got["items"].([]any); !ok || len(items) != 3 {
			t.Errorf("expected items=[1,2,3], got %v", got["items"])
		}
	})

	t.Run("realistic Zed-style settings", func(t *testing.T) {
		// Mirrors the actual #4430 reproducer: a settings.json with both
		// comment styles AND trailing commas in the same file.
		input := []byte(`// Settings for Zed
{
    /* MCP server registry */
    "context_servers": {
        "test-mcp": {
            "command": "/bin/echo",
            "args": ["hello"], // arg list
        }, // trailing comma in object too
    },
}`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		servers, ok := got["context_servers"].(map[string]any)
		if !ok {
			t.Fatalf("missing context_servers map")
		}
		if _, ok := servers["test-mcp"].(map[string]any); !ok {
			t.Errorf("missing test-mcp entry: %v", servers)
		}
	})

	t.Run("malformed input returns error", func(t *testing.T) {
		input := []byte(`{ this is not valid json or jsonc`)
		var got map[string]any
		if err := unmarshalJSONC(input, &got); err == nil {
			t.Fatal("expected error for malformed input, got nil")
		}
	})
}
