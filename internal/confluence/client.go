package confluence

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UserAgent is sent on every request; Atlassian recommends programmatic
// clients identify themselves so they can be diagnosed server-side.
const UserAgent = "md2confluence"

// Retry policy for transient failures (network, 429, 5xx).
const (
	maxAttempts    = 4
	baseBackoff    = 500 * time.Millisecond
	maxBackoff     = 8 * time.Second
	maxRetryAfter  = 60 * time.Second // ignore Retry-After hints longer than this
	requestTimeout = 30 * time.Second
)

// Client handles communication with the Confluence Cloud REST API v2.
type Client struct {
	BaseURL    string
	Email      string
	Token      string
	authHeader string // pre-computed "Basic base64(email:token)"
	http       *http.Client
}

// NewClient creates a new Confluence API client.
func NewClient(baseURL, email, token string) *Client {
	auth := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Email:      email,
		Token:      token,
		authHeader: "Basic " + auth,
		http:       &http.Client{Timeout: requestTimeout},
	}
}

// doRequest performs an HTTP request with basic auth and returns a non-nil
// error for non-2xx status codes. Transient failures (connection errors, 429,
// 502, 503, 504) are retried with exponential backoff, honoring Retry-After
// when present.
//
// body, if non-nil, must implement io.Seeker (e.g. *bytes.Reader) so it can
// be rewound between retries. All callers pass bytes.NewReader, which is fine.
func (c *Client) doRequest(method, reqURL string, body io.Reader) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if body != nil {
			if s, ok := body.(io.Seeker); ok {
				if _, err := s.Seek(0, io.SeekStart); err != nil {
					return nil, fmt.Errorf("rewind request body: %w", err)
				}
			} else if attempt > 1 {
				return nil, fmt.Errorf("retry needed but request body is not seekable")
			}
		}

		req, err := http.NewRequest(method, reqURL, body)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", UserAgent)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("execute request: %w", err)
			if attempt < maxAttempts {
				time.Sleep(backoffDelay(attempt, 0))
				continue
			}
			return nil, lastErr
		}

		if shouldRetryStatus(resp.StatusCode) && attempt < maxAttempts {
			wait := backoffDelay(attempt, retryAfter(resp))
			resp.Body.Close()
			time.Sleep(wait)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var errMsg strings.Builder
			errMsg.WriteString(fmt.Sprintf("%s %s: status %d", method, reqURL, resp.StatusCode))
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if len(bodyBytes) > 0 {
				errMsg.WriteString(": " + string(bodyBytes))
			}
			return nil, fmt.Errorf("%s", errMsg.String())
		}

		return resp, nil
	}
	return nil, lastErr
}

// shouldRetryStatus reports whether status is a transient HTTP error we should
// retry: 429 (rate-limited) and the standard recoverable 5xx codes.
func shouldRetryStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryAfter parses the Retry-After header (delta-seconds form) returned by
// rate-limited responses. Zero means "no hint, use exponential backoff".
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

// backoffDelay returns the wait time before retry attempt n (1-indexed).
// If hint > 0 (e.g. from Retry-After), the larger of hint and the exponential
// schedule is used.
func backoffDelay(attempt int, hint time.Duration) time.Duration {
	d := baseBackoff << (attempt - 1) // 500ms, 1s, 2s, 4s...
	if d > maxBackoff {
		d = maxBackoff
	}
	if hint > d {
		return hint
	}
	return d
}

// LookupSpaceID resolves a human-readable space key (e.g. "DOCS") to its numeric-string space ID.
// v2 endpoints identify spaces by ID, not key.
func (c *Client) LookupSpaceID(key string) (string, error) {
	u := fmt.Sprintf("%s/api/v2/spaces?keys=%s&limit=1", c.BaseURL, urlEncode(key))

	resp, err := c.doRequest("GET", u, nil)
	if err != nil {
		return "", fmt.Errorf("lookup space %q: %w", key, err)
	}
	defer resp.Body.Close()

	var result SpaceList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode space response: %w", err)
	}

	if len(result.Results) == 0 {
		return "", fmt.Errorf("no Confluence space has key %q - the value is the short space key (e.g. DEV, DOCS, or ~user for personal spaces), not the display name. Find it in Confluence under Space settings → Space details → Key", key)
	}

	return result.Results[0].ID, nil
}

// SearchPage finds a page by space ID and title. Returns nil (no error) when not found.
// The storage-format body is requested inline so callers can diff it against a
// freshly rendered body to skip no-op updates.
func (c *Client) SearchPage(spaceID, title string) (*Page, error) {
	u := fmt.Sprintf("%s/api/v2/pages?space-id=%s&title=%s&body-format=storage&limit=1",
		c.BaseURL, spaceID, urlEncode(title))

	resp, err := c.doRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("search page: %w", err)
	}
	defer resp.Body.Close()

	var result PageList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, nil
	}

	return &result.Results[0], nil
}

// CreatePage creates a new Confluence page.
func (c *Client) CreatePage(req *CreateRequest) (*Page, error) {
	u := c.BaseURL + "/api/v2/pages"

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create request: %w", err)
	}

	resp, err := c.doRequest("POST", u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer resp.Body.Close()

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode create response: %w", err)
	}

	return &page, nil
}

// UpdatePage updates an existing Confluence page.
func (c *Client) UpdatePage(id string, req *UpdateRequest) (*Page, error) {
	u := fmt.Sprintf("%s/api/v2/pages/%s", c.BaseURL, id)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal update request: %w", err)
	}

	resp, err := c.doRequest("PUT", u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("update page: %w", err)
	}
	defer resp.Body.Close()

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode update response: %w", err)
	}

	return &page, nil
}

// urlEncode encodes a string for use in Confluence API query parameters.
func urlEncode(s string) string {
	return url.QueryEscape(s)
}

// FindAttachment returns the existing attachment on a page whose title matches
// filename, or nil (no error) when not found. Uses the Confluence v1 REST API
// because the v2 attachment endpoints do not cover uploads reliably.
func (c *Client) FindAttachment(pageID, filename string) (*Attachment, error) {
	u := fmt.Sprintf("%s/rest/api/content/%s/child/attachment?filename=%s&limit=1",
		c.BaseURL, pageID, urlEncode(filename))

	resp, err := c.doRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer resp.Body.Close()

	var result AttachmentList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode attachment list: %w", err)
	}
	if len(result.Results) == 0 {
		return nil, nil
	}
	return &result.Results[0], nil
}

// UploadAttachment creates or updates an attachment on a page. If an attachment
// with the same filename already exists its binary data is replaced, preserving
// history; otherwise a new attachment is created.
func (c *Client) UploadAttachment(pageID, filePath string) error {
	filename := filepath.Base(filePath)

	existing, err := c.FindAttachment(pageID, filename)
	if err != nil {
		return err
	}

	var endpoint string
	if existing != nil {
		endpoint = fmt.Sprintf("%s/rest/api/content/%s/child/attachment/%s/data",
			c.BaseURL, pageID, existing.ID)
	} else {
		endpoint = fmt.Sprintf("%s/rest/api/content/%s/child/attachment", c.BaseURL, pageID)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer file.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	// minorEdit=true suppresses notifications for attachment updates.
	_ = mw.WriteField("minorEdit", "true")
	if err := mw.Close(); err != nil {
		return fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute attachment upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload attachment %s: status %d: %s", filename, resp.StatusCode, string(respBody))
	}
	return nil
}
