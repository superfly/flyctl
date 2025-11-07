package uiex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

func setupTestClient(server *httptest.Server) (*Client, context.Context, error) {
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	ios, _, _, _ := iostreams.Test()
	ctx = iostreams.NewContext(ctx, ios)

	// Create a mock token
	token := &tokens.Tokens{}
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: token,
	})

	client := &Client{
		baseUrl:    baseURL,
		httpClient: http.DefaultClient,
	}

	return client, ctx, nil
}

func TestListDatabases_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/postgres/test-cluster-id/databases" {
			t.Errorf("expected path /api/v1/postgres/test-cluster-id/databases, got %s", r.URL.Path)
		}

		response := ListDatabasesResponse{
			Data: []Database{
				{Name: "db1"},
				{Name: "db2"},
				{Name: "db3"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	response, err := client.ListDatabases(ctx, "test-cluster-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(response.Data) != 3 {
		t.Errorf("expected 3 databases, got %d", len(response.Data))
	}

	expectedNames := []string{"db1", "db2", "db3"}
	for i, db := range response.Data {
		if db.Name != expectedNames[i] {
			t.Errorf("expected database name %s, got %s", expectedNames[i], db.Name)
		}
	}
}

func TestListDatabases_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	_, err = client.ListDatabases(ctx, "non-existent-cluster")
	if err == nil {
		t.Fatal("expected error for non-existent cluster")
	}

	if err.Error() != "cluster non-existent-cluster not found" {
		t.Errorf("expected 'cluster non-existent-cluster not found', got %s", err.Error())
	}
}

func TestListDatabases_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	_, err = client.ListDatabases(ctx, "test-cluster-id")
	if err == nil {
		t.Fatal("expected error for forbidden access")
	}

	expectedError := "access denied: you don't have permission to list databases for cluster test-cluster-id"
	if err.Error() != expectedError {
		t.Errorf("expected '%s', got %s", expectedError, err.Error())
	}
}

func TestCreateDatabase_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/postgres/test-cluster-id/databases" {
			t.Errorf("expected path /api/v1/postgres/test-cluster-id/databases, got %s", r.URL.Path)
		}

		var input CreateDatabaseInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if input.Name != "newdb" {
			t.Errorf("expected database name 'newdb', got %s", input.Name)
		}

		response := CreateDatabaseResponse{
			Data: Database{Name: "newdb"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	input := CreateDatabaseInput{Name: "newdb"}
	response, err := client.CreateDatabase(ctx, "test-cluster-id", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.Data.Name != "newdb" {
		t.Errorf("expected database name 'newdb', got %s", response.Data.Name)
	}
}

func TestCreateDatabase_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	input := CreateDatabaseInput{Name: "newdb"}
	_, err = client.CreateDatabase(ctx, "non-existent-cluster", input)
	if err == nil {
		t.Fatal("expected error for non-existent cluster")
	}

	if err.Error() != "cluster non-existent-cluster not found" {
		t.Errorf("expected 'cluster non-existent-cluster not found', got %s", err.Error())
	}
}

func TestCreateDatabase_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	input := CreateDatabaseInput{Name: "newdb"}
	_, err = client.CreateDatabase(ctx, "test-cluster-id", input)
	if err == nil {
		t.Fatal("expected error for forbidden access")
	}

	expectedError := "access denied: you don't have permission to create databases for cluster test-cluster-id"
	if err.Error() != expectedError {
		t.Errorf("expected '%s', got %s", expectedError, err.Error())
	}
}

func TestCreateDatabase_EmptyName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input CreateDatabaseInput
		json.NewDecoder(r.Body).Decode(&input)

		if input.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		response := CreateDatabaseResponse{
			Data: Database{Name: input.Name},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, ctx, err := setupTestClient(server)
	if err != nil {
		t.Fatalf("failed to setup test client: %v", err)
	}

	input := CreateDatabaseInput{Name: ""}
	_, err = client.CreateDatabase(ctx, "test-cluster-id", input)
	// The server returns 400, which will be handled as a default error
	if err == nil {
		t.Fatal("expected error for empty database name")
	}
}
