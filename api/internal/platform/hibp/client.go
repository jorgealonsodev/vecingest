// Package hibp implements the real Have I Been Pwned k-anonymity range
// API client, adapting the domain-owned password.HIBPChecker port
// (design D-F, D-G; auth-credentials: HIBP k-Anonymity Breach Check).
package hibp

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // required by the HIBP range-API protocol itself, never used for password storage
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pwnedpasswords.com/range/"

// Client is a k-anonymity range-API client. BaseURL and HTTPClient are
// exported so tests can point Client at a fake server with no network
// access (per the project's go-testing conventions).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a Client configured against the real HIBP endpoint
// with a bounded timeout, so a slow or hanging upstream cannot stall a
// registration or password-reset request indefinitely.
func NewClient() *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// IsBreached queries the k-anonymity range API with the 5-character
// SHA-1 prefix of password, sends Add-Padding: true, and discards
// response entries whose count is 0. err != nil signals a transport-level
// failure (network error, non-200 status) -- the domain layer
// (password.PasswordPolicy) is what decides to fail open on that error;
// this client never does.
func (c *Client) IsBreached(ctx context.Context, password string) (bool, error) {
	// SHA-1 here is the HIBP k-anonymity range-query protocol itself, never
	// used for password storage or verification (see the package doc).
	sum := sha1.Sum([]byte(password)) //nolint:gosec // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	hexSum := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hexSum[:5], hexSum[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("hibp: build request: %w", err)
	}
	req.Header.Set("Add-Padding", "true")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("hibp: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(parts[0], suffix) {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		if count == 0 {
			// A padding entry: Add-Padding fills the response with
			// decoy suffixes at count 0 so response size does not leak
			// whether the real suffix matched. Discard, keep scanning
			// (a genuine match could still appear later in the stream).
			continue
		}
		return true, nil
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("hibp: read response: %w", err)
	}
	return false, nil
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}
