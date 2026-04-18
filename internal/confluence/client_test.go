package confluence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient(ClientConfig{
		BaseURL: "https://example.atlassian.net/wiki",
		User:    "user@example.com",
		Token:   "token123",
		IsCloud: true,
	})

	if client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestGetPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/12345" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("expand") != "body.storage,version" {
			t.Error("should request body.storage,version expansion")
		}

		page := Page{
			ID:    "12345",
			Title: "Test Page",
			Body: PageBody{
				Storage: StorageBody{
					Value:          "<p>Hello World</p>",
					Representation: "storage",
				},
			},
			Version: PageVersion{Number: 1},
		}
		json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		BaseURL: server.URL,
		User:    "user",
		Token:   "token",
		IsCloud: true,
	})

	page, err := client.GetPage(context.Background(), "12345")
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if page.Title != "Test Page" {
		t.Errorf("title = %s, want Test Page", page.Title)
	}
	if page.Body.Storage.Value != "<p>Hello World</p>" {
		t.Error("body content mismatch")
	}
}

func TestGetPageNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Page not found"}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	_, err := client.GetPage(context.Background(), "99999")
	if err == nil {
		t.Error("should return error for 404")
	}
}

func TestCreatePage(t *testing.T) {
	var receivedReq struct {
		Type  string `json:"type"`
		Title string `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
		Body struct {
			Storage struct {
				Value string `json:"value"`
			} `json:"storage"`
		} `json:"body"`
		Ancestors []struct {
			ID string `json:"id"`
		} `json:"ancestors"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&receivedReq)

		resp := Page{ID: "67890", Title: receivedReq.Title}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	page, err := client.CreatePage(context.Background(), CreatePageRequest{
		SpaceKey: "TEST",
		Title:    "New Report",
		Body:     "<h1>Report</h1>",
		ParentID: "12345",
	})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}
	if page.ID != "67890" {
		t.Errorf("id = %s, want 67890", page.ID)
	}
	if receivedReq.Space.Key != "TEST" {
		t.Error("space key not sent")
	}
	if len(receivedReq.Ancestors) == 0 || receivedReq.Ancestors[0].ID != "12345" {
		t.Error("parent ID not sent")
	}
}

func TestUpdatePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/rest/api/content/12345" {
			t.Errorf("path = %s", r.URL.Path)
		}

		var req UpdatePageRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Version.Number != 2 {
			t.Errorf("version = %d, want 2", req.Version.Number)
		}

		json.NewEncoder(w).Encode(Page{ID: "12345", Title: req.Title})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	page, err := client.UpdatePage(context.Background(), "12345", UpdatePageRequest{
		Title:   "Updated Title",
		Body:    "<p>Updated</p>",
		Version: PageVersion{Number: 2},
	})
	if err != nil {
		t.Fatalf("UpdatePage failed: %v", err)
	}
	if page.Title != "Updated Title" {
		t.Errorf("title = %s", page.Title)
	}
}

func TestSearchPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spaceKey := r.URL.Query().Get("spaceKey")
		title := r.URL.Query().Get("title")

		if spaceKey != "TEST" || title != "My Page" {
			t.Errorf("query params: space=%s title=%s", spaceKey, title)
		}

		json.NewEncoder(w).Encode(PageSearchResult{
			Results: []Page{{ID: "111", Title: "My Page"}},
		})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, User: "u", Token: "t", IsCloud: true})

	pages, err := client.SearchPages(context.Background(), "TEST", "My Page")
	if err != nil {
		t.Fatalf("SearchPages failed: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}
}

func TestAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(Page{ID: "1"})
	}))
	defer server.Close()

	// Cloud uses Basic auth
	client := NewClient(ClientConfig{BaseURL: server.URL, User: "user", Token: "token", IsCloud: true})
	client.GetPage(context.Background(), "1")

	if authHeader == "" {
		t.Error("auth header should be set")
	}
}

func TestEmptyBaseURL(t *testing.T) {
	client := NewClient(ClientConfig{BaseURL: ""})
	_, err := client.GetPage(context.Background(), "123")
	if err == nil {
		t.Error("empty base URL should fail")
	}
}
