package loki

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/incu6us/loki-mcp-server/internal/config"
)

type Client interface {
	QueryRange(ctx context.Context, params QueryRangeParams) (*QueryResponse, error)
	Query(ctx context.Context, params QueryParams) (*QueryResponse, error)
	Labels(ctx context.Context, params LabelsParams) ([]string, error)
	LabelValues(ctx context.Context, label string, params LabelsParams) ([]string, error)
	Series(ctx context.Context, params SeriesParams) ([]map[string]string, error)
}

type httpClient struct {
	baseURL     string
	httpClient  *http.Client
	username    string
	password    string
	bearerToken string
	tenantID    string
}

func NewClient(cfg *config.Config) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify,
		},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &httpClient{
		baseURL: cfg.LokiURL,
		httpClient: &http.Client{
			Timeout:   cfg.HTTPTimeout,
			Transport: transport,
		},
		username:    cfg.Username,
		password:    cfg.Password,
		bearerToken: cfg.BearerToken,
		tenantID:    cfg.TenantID,
	}
}

func (c *httpClient) setAuthHeaders(req *http.Request) {
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	} else if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	if c.tenantID != "" {
		req.Header.Set("X-Scope-OrgID", c.tenantID)
	}
}

type statusCarrier interface {
	GetStatus() string
}

func (c *httpClient) do(ctx context.Context, endpoint string, queryParams url.Values, result any) error {
	reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, endpoint, queryParams.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return c.parseLokiError(resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return c.validateStatus(result)
}

func (c *httpClient) parseLokiError(statusCode int, body []byte) error {
	var lokiErr lokiErrorResponse
	if err := json.Unmarshal(body, &lokiErr); err == nil && lokiErr.Error != "" {
		return fmt.Errorf("loki %s error (HTTP %d): %s", lokiErr.ErrorType, statusCode, lokiErr.Error)
	}
	return fmt.Errorf("loki request failed (HTTP %d): %s", statusCode, string(body))
}

func (c *httpClient) validateStatus(result any) error {
	if carrier, ok := result.(statusCarrier); ok {
		if carrier.GetStatus() != "success" {
			return fmt.Errorf("loki returned non-success status: %s", carrier.GetStatus())
		}
	}
	return nil
}

func (c *httpClient) QueryRange(ctx context.Context, params QueryRangeParams) (*QueryResponse, error) {
	v := url.Values{}
	v.Set("query", params.Query)
	if params.Start != "" {
		v.Set("start", params.Start)
	}
	if params.End != "" {
		v.Set("end", params.End)
	}
	if params.Limit > 0 {
		v.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Direction != "" {
		v.Set("direction", params.Direction)
	}

	var resp QueryResponse
	if err := c.do(ctx, "/loki/api/v1/query_range", v, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *httpClient) Query(ctx context.Context, params QueryParams) (*QueryResponse, error) {
	v := url.Values{}
	v.Set("query", params.Query)
	if params.Limit > 0 {
		v.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Time != "" {
		v.Set("time", params.Time)
	}
	if params.Direction != "" {
		v.Set("direction", params.Direction)
	}

	var resp QueryResponse
	if err := c.do(ctx, "/loki/api/v1/query", v, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *httpClient) Labels(ctx context.Context, params LabelsParams) ([]string, error) {
	v := url.Values{}
	if params.Start != "" {
		v.Set("start", params.Start)
	}
	if params.End != "" {
		v.Set("end", params.End)
	}

	var resp LabelsResponse
	if err := c.do(ctx, "/loki/api/v1/labels", v, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *httpClient) LabelValues(ctx context.Context, label string, params LabelsParams) ([]string, error) {
	v := url.Values{}
	if params.Start != "" {
		v.Set("start", params.Start)
	}
	if params.End != "" {
		v.Set("end", params.End)
	}

	var resp LabelsResponse
	if err := c.do(ctx, "/loki/api/v1/label/"+label+"/values", v, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *httpClient) Series(ctx context.Context, params SeriesParams) ([]map[string]string, error) {
	v := url.Values{}
	for _, m := range params.Match {
		v.Add("match[]", m)
	}
	if params.Start != "" {
		v.Set("start", params.Start)
	}
	if params.End != "" {
		v.Set("end", params.End)
	}

	var resp SeriesResponse
	if err := c.do(ctx, "/loki/api/v1/series", v, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}
