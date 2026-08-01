package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultAPIBase = "https://api.gopro.com"

var apiBaseURL = defaultAPIBase
var ErrAuth = errors.New("GoPro authentication failed")

type MediaItem struct {
	ID         string         `json:"id"`
	Filename   string         `json:"filename,omitempty"`
	FileSize   int64          `json:"file_size,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	CapturedAt string         `json:"captured_at,omitempty"`
	MediaType  string         `json:"type,omitempty"`
	Raw        map[string]any `json:"-"`
}

type GoProClient struct {
	Token, UserID, BaseURL string
	HTTP                   *http.Client
}

func NewGoProClient(token, userID string) *GoProClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns, transport.MaxIdleConnsPerHost = 32, 32
	transport.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 2 * time.Minute
	return &GoProClient{Token: token, UserID: userID, BaseURL: apiBaseURL, HTTP: &http.Client{Transport: transport}}
}

func (c *GoProClient) request(ctx context.Context, method, endpoint string, query url.Values) (*http.Response, error) {
	address := strings.TrimRight(c.BaseURL, "/") + endpoint
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.gopro.jk.media+json; version=2.0.0")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	request.AddCookie(&http.Cookie{Name: "gp_access_token", Value: c.Token})
	request.AddCookie(&http.Cookie{Name: "gp_user_id", Value: c.UserID})
	return c.HTTP.Do(request)
}

func (c *GoProClient) get(ctx context.Context, endpoint string, query url.Values) (*http.Response, error) {
	for attempt := 0; attempt < 5; attempt++ {
		response, err := c.request(ctx, http.MethodGet, endpoint, query)
		if err != nil {
			return nil, err
		}
		if response.StatusCode == http.StatusUnauthorized {
			response.Body.Close()
			return nil, fmt.Errorf("%w: HTTP 401 on %s", ErrAuth, endpoint)
		}
		if response.StatusCode != http.StatusTooManyRequests {
			return response, nil
		}
		response.Body.Close()
		delay := time.Duration(1<<attempt) * time.Second
		if value := response.Header.Get("Retry-After"); value != "" {
			if seconds, err := strconv.ParseFloat(value, 64); err == nil {
				delay = time.Duration(seconds * float64(time.Second))
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, errors.New("GoPro rate limit persisted after retries")
}

func decodeJSONResponse(response *http.Response, destination any) error {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d from GoPro API", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return fmt.Errorf("decode GoPro response: %w", err)
	}
	return nil
}

func (c *GoProClient) Validate(ctx context.Context) (map[string]any, error) {
	response, err := c.get(ctx, "/media/user", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := decodeJSONResponse(response, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		result, _ := typed.Int64()
		return result
	case string:
		result, _ := strconv.ParseInt(typed, 10, 64)
		return result
	}
	return 0
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if result, ok := value.(string); ok {
		return result
	}
	return fmt.Sprint(value)
}

func mediaFromRaw(raw map[string]any) (MediaItem, error) {
	id := stringValue(raw["id"])
	if id == "" {
		return MediaItem{}, errors.New("GoPro media record has no id")
	}
	captured := stringValue(raw["captured_at"])
	if captured == "" {
		captured = stringValue(raw["created_at"])
	}
	return MediaItem{ID: id, Filename: stringValue(raw["filename"]), FileSize: int64Value(raw["file_size"]), CreatedAt: stringValue(raw["created_at"]), CapturedAt: captured, MediaType: stringValue(raw["type"]), Raw: raw}, nil
}

func (c *GoProClient) ListAll(ctx context.Context, perPage int) ([]MediaItem, error) {
	items := []MediaItem{}
	seen := map[string]bool{}
	for page := 1; ; page++ {
		query := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}, "fields": {"camera_model,captured_at,content_title,content_type,created_at,filename,file_extension,file_size,height,id,item_count,orientation,ready_to_view,resolution,source_duration,type,width"}}
		response, err := c.get(ctx, "/media/search", query)
		if err != nil {
			return nil, err
		}
		var body struct {
			Embedded struct {
				Media []map[string]any `json:"media"`
			} `json:"_embedded"`
			Pages struct {
				Total int `json:"total_pages"`
			} `json:"_pages"`
		}
		if err := decodeJSONResponse(response, &body); err != nil {
			return nil, err
		}
		for _, raw := range body.Embedded.Media {
			item, err := mediaFromRaw(raw)
			if err != nil {
				return nil, err
			}
			if !seen[item.ID] {
				seen[item.ID] = true
				items = append(items, item)
			}
		}
		if body.Pages.Total == 0 || page >= body.Pages.Total {
			return items, nil
		}
	}
}

func (c *GoProClient) GetMedia(ctx context.Context, id string) (map[string]any, error) {
	response, err := c.get(ctx, "/media/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := decodeJSONResponse(response, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *GoProClient) FullRecords(ctx context.Context, items []MediaItem, concurrency int) (map[string]map[string]any, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	result := map[string]map[string]any{}
	var mu sync.Mutex
	jobs := make(chan MediaItem)
	var workers sync.WaitGroup
	var authErr error
	var authMu sync.Mutex
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				full, err := c.GetMedia(ctx, item.ID)
				if errors.Is(err, ErrAuth) {
					authMu.Lock()
					authErr = err
					authMu.Unlock()
					continue
				}
				if err != nil {
					full = make(map[string]any, len(item.Raw)+1)
					for key, value := range item.Raw {
						full[key] = value
					}
					full["_full_record_error"] = err.Error()
				}
				mu.Lock()
				result[item.ID] = full
				mu.Unlock()
			}
		}()
	}
	for _, item := range items {
		jobs <- item
	}
	close(jobs)
	workers.Wait()
	return result, authErr
}

func (c *GoProClient) StreamSourceZIP(ctx context.Context, id string, destination io.Writer) (int64, error) {
	query := url.Values{"ids": {id}, "access_token": {c.Token}}
	response, err := c.request(ctx, http.MethodGet, "/media/x/zip/source", query)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return 0, fmt.Errorf("%w: HTTP 401 on source ZIP for %s", ErrAuth, id)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("HTTP %d from source ZIP endpoint for %s", response.StatusCode, id)
	}
	return io.CopyBuffer(destination, response.Body, make([]byte, 4*1024*1024))
}
