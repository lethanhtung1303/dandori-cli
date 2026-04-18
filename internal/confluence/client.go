package confluence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	user       string
	token      string
	isCloud    bool
	httpClient *http.Client
}

type ClientConfig struct {
	BaseURL string
	User    string
	Token   string
	IsCloud bool
	Timeout time.Duration
}

type ConfluenceClient interface {
	GetPage(ctx context.Context, pageID string) (*Page, error)
	CreatePage(ctx context.Context, req CreatePageRequest) (*Page, error)
	UpdatePage(ctx context.Context, pageID string, req UpdatePageRequest) (*Page, error)
	SearchPages(ctx context.Context, spaceKey, title string) ([]Page, error)
}

func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: cfg.BaseURL,
		user:    cfg.User,
		token:   cfg.Token,
		isCloud: cfg.IsCloud,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) GetPage(ctx context.Context, pageID string) (*Page, error) {
	if c.baseURL == "" {
		return nil, ErrEmptyBaseURL
	}

	url := fmt.Sprintf("%s/rest/api/content/%s?expand=body.storage,version", c.baseURL, pageID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setAuth(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPageNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, body)
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &page, nil
}

func (c *Client) CreatePage(ctx context.Context, createReq CreatePageRequest) (*Page, error) {
	if c.baseURL == "" {
		return nil, ErrEmptyBaseURL
	}

	apiReq := createPageAPIRequest{
		Type:  "page",
		Title: createReq.Title,
	}
	apiReq.Space.Key = createReq.SpaceKey
	apiReq.Body.Storage.Value = createReq.Body
	apiReq.Body.Storage.Representation = "storage"

	if createReq.ParentID != "" {
		apiReq.Ancestors = []struct {
			ID string `json:"id"`
		}{{ID: createReq.ParentID}}
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/content", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, respBody)
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &page, nil
}

func (c *Client) UpdatePage(ctx context.Context, pageID string, updateReq UpdatePageRequest) (*Page, error) {
	if c.baseURL == "" {
		return nil, ErrEmptyBaseURL
	}

	apiReq := updatePageAPIRequest{
		Type:    "page",
		Title:   updateReq.Title,
		Version: updateReq.Version,
	}
	apiReq.Body.Storage.Value = updateReq.Body
	apiReq.Body.Storage.Representation = "storage"

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	reqURL := fmt.Sprintf("%s/rest/api/content/%s", c.baseURL, pageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, respBody)
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &page, nil
}

func (c *Client) SearchPages(ctx context.Context, spaceKey, title string) ([]Page, error) {
	if c.baseURL == "" {
		return nil, ErrEmptyBaseURL
	}

	params := url.Values{}
	params.Set("spaceKey", spaceKey)
	params.Set("title", title)

	reqURL := fmt.Sprintf("%s/rest/api/content?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setAuth(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, body)
	}

	var result PageSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return result.Results, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.isCloud {
		// Cloud uses Basic auth with email:api-token
		auth := base64.StdEncoding.EncodeToString([]byte(c.user + ":" + c.token))
		req.Header.Set("Authorization", "Basic "+auth)
	} else {
		// Data Center uses Bearer token
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
