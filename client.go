package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultBaseURL         = "https://p68-maildomainws.icloud.com"
	clientBuildNumber      = "2608Build39"
	clientMasteringNumber  = "2608Build39"
	userAgent              = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)"
)

// Client talks to the iCloud Hide My Email API.
type Client struct {
	BaseURL    string
	Cookies    string
	DSID       string
	ClientID   string
	HTTPClient *http.Client
}

// NewClient creates a Client from a cookie string.
func NewClient(cookies string) (*Client, error) {
	dsid, err := ExtractDSID(cookies)
	if err != nil {
		return nil, err
	}
	return &Client{
		BaseURL:  defaultBaseURL,
		Cookies:  cookies,
		DSID:     dsid,
		ClientID: generateClientID(),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// generateClientID produces a random UUID v4 client identifier per session.
func generateClientID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08X-%04X-%04X-%04X-%012X",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// queryParams returns the standard query parameters for all API calls.
func (c *Client) queryParams() url.Values {
	return url.Values{
		"clientBuildNumber":     {clientBuildNumber},
		"clientMasteringNumber": {clientMasteringNumber},
		"clientId":              {c.ClientID},
		"dsid":                  {c.DSID},
	}
}

// setHeaders applies the required headers to a request.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Cookie", c.Cookies)
	req.Header.Set("Origin", "https://www.icloud.com")
	req.Header.Set("Referer", "https://www.icloud.com/")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if req.Method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
}

// doRequest executes a request and decodes the JSON response.
func (c *Client) doRequest(req *http.Request, out interface{}) error {
	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach iCloud: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// continue to decode
	case http.StatusUnauthorized, 421:
		return fmt.Errorf("cookies expired. Run 'hme auth' to refresh")
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited. Wait a few minutes")
	default:
		return fmt.Errorf("unexpected status %d from iCloud", resp.StatusCode)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// Generate requests a new random HME email address.
func (c *Client) Generate() (string, error) {
	u := c.BaseURL + "/v1/hme/generate?" + c.queryParams().Encode()
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}

	var resp GenerateResponse
	if err := c.doRequest(req, &resp); err != nil {
		return "", err
	}
	if !resp.Success {
		return "", apiErrorToError(resp.Error)
	}
	if resp.Result.Hme == "" {
		return "", fmt.Errorf("API returned success but no email address")
	}
	return resp.Result.Hme, nil
}

// Reserve confirms (activates) a generated HME email with a label and optional note.
func (c *Client) Reserve(hme, label, note string) error {
	body, _ := json.Marshal(ReserveRequest{
		Hme:   hme,
		Label:  label,
		Note:   note,
	})
	u := c.BaseURL + "/v1/hme/reserve?" + c.queryParams().Encode()
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}

	var resp ReserveResponse
	if err := c.doRequest(req, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return apiErrorToError(resp.Error)
	}
	return nil
}

// List fetches all HME email aliases.
func (c *Client) List() ([]HmeEmail, error) {
	u := c.BaseURL + "/v2/hme/list?" + c.queryParams().Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	var resp ListResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, apiErrorToError(resp.Error)
	}
	return resp.Result.HmeEmails, nil
}

func apiErrorToError(e *APIError) error {
	if e == nil {
		return fmt.Errorf("unknown API error")
	}
	msg := e.ErrorMessage
	if msg == "" {
		msg = e.Reason
	}
	if msg == "" {
		msg = fmt.Sprintf("error code %d", e.ErrorCode)
	}
	return fmt.Errorf("%s", msg)
}
