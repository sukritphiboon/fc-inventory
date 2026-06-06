package fcclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// versionNegotiationOrder is the priority list of API versions to try.
// Matches fc_client.py:51 exactly.
var versionNegotiationOrder = []string{"v8.0", "v6.5", "v6.3", "v6.1", "v1.0", "v9.0"}

// loginAttempt describes one entry in the auto-detect matrix.
type loginAttempt struct {
	label   string
	method  string
	headers map[string]string
	body    any
}

// versionRejected reports whether the response body indicates the
// supplied Accept-version was rejected. Matches the "10000022" check
// in fc_client.py:130. The code is numeric so it is locale-safe.
func versionRejected(body string) bool {
	return strings.Contains(body, "10000022")
}

// sha256Hex is the FC password-hashing scheme. Mirrors
// fc_client._sha256 (fc_client.py:36-38).
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// buildAuthMethods returns the three auth flavours from fc_client.py:54-83.
func (c *Client) buildAuthMethods() []loginAttempt {
	host := StripProtocol(c.cfg.FC.Host)
	_ = host
	u := c.cfg.FC.Username
	p := c.cfg.FC.Password
	return []loginAttempt{
		{
			label:  "POST + headers + plain",
			method: http.MethodPost,
			headers: map[string]string{
				"X-Auth-User":         u,
				"X-Auth-Key":          p,
				"X-Auth-UserType":     "0",
				"X-ENCRYPT-ALGORITHM": "1",
			},
		},
		{
			label:  "POST + headers + SHA256",
			method: http.MethodPost,
			headers: map[string]string{
				"X-Auth-User":         u,
				"X-Auth-Key":          sha256Hex(p),
				"X-Auth-UserType":     "0",
				"X-ENCRYPT-ALGORITHM": "0",
			},
		},
		{
			label:   "PUT + JSON body",
			method:  http.MethodPut,
			headers: map[string]string{},
			body:    map[string]any{"userName": u, "password": p},
		},
	}
}

// Login attempts the 6 (versions) x 3 (auth methods) x N (ports) matrix
// until one combination returns HTTP 200. On success it stores the
// working baseURL, token, and version on the Client.
//
// Matches the loop structure of fc_client.py:88-148 with these ports:
//   [cfg.FC.Port, 7443, 8443] (deduped, preserving order).
//
// The original used a triple-nested Python loop with a `break` labelled
// to skip to the next port. In Go we encode the same control flow with
// helper functions that return a status enum and let the outer loop
// decide whether to advance.
func (c *Client) Login(ctx context.Context) error {
	host := StripProtocol(c.cfg.FC.Host)
	ports := dedupPorts(c.cfg.FC.Port, []int{7443, 8443})
	auths := c.buildAuthMethods()

	logger := slog.With("component", "fcclient.Login", "host", host, "user", c.cfg.FC.Username)
	logger.Info("starting auto-detect login")

	var lastErr error
	for _, port := range ports {
		for _, ver := range versionNegotiationOrder {
			for _, a := range auths {
				if err := ctx.Err(); err != nil {
					return err
				}
				logger.Debug("login attempt", "port", port, "version", ver, "method", a.label)

				tok, body, err := c.tryLogin(ctx, host, port, ver, a)
				if err == nil && tok != "" {
					c.baseURL = fmt.Sprintf("https://%s:%d/service", host, port)
					c.token = tok
					c.version = ver
					c.http.Timeout = c.cfg.RequestTimeout()
					logger.Info("login succeeded", "port", port, "version", ver, "method", a.label)
					return nil
				}

				// Failure: classify and decide the next step.
				if err != nil {
					lastErr = err
					// Connection-level error: skip remaining auths/versions
					// for this port and try the next one.
					if isConnError(err) {
						logger.Warn("connection error; advancing to next port", "port", port, "err", err.Error())
						break // breaks auth loop; port loop continues
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return err
					}
					logger.Debug("attempt failed; trying next", "err", err.Error(), "body", body)
				} else {
					// No error but no token: server rejected the request.
					// Check for version-rejection body to skip to next version.
					lastErr = fmt.Errorf("no token in response (body=%s)", body)
					logger.Debug("attempt failed; trying next", "body", body)
				}

				if versionRejected(body) {
					logger.Info("version rejected; advancing to next version", "version", ver)
					break // break auth loop; version loop continues
				}
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no login method succeeded")
	}
	return fmt.Errorf("all login methods failed: %w", lastErr)
}

// tryLogin performs one HTTP request for the given (port, version, auth)
// combination. Returns (token, responseBody, error).
func (c *Client) tryLogin(ctx context.Context, host string, port int, version string, a loginAttempt) (string, string, error) {
	sessionURL := formatURL(host, port)

	var req *http.Request
	var err error
	switch a.method {
	case http.MethodPost:
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, sessionURL, nil)
	case http.MethodPut:
		req, err = http.NewRequestWithContext(ctx, http.MethodPut, sessionURL, bytesOrNil(a.body))
	default:
		return "", "", fmt.Errorf("unsupported method %q", a.method)
	}
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Accept", "application/json;version="+version+";charset=UTF-8")
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	// Bound the per-attempt timeout to 10s, matching fc_client.py:104.
	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req = req.WithContext(attemptCtx)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14)) // cap to 16 KB
	bodyStr := string(bodyBytes)

	if resp.StatusCode != 200 {
		// Try to surface the body's errorCode/errorMessage for the
		// warning log. Mirror the body[:200] truncation in the Python
		// log line at fc_client.py:125.
		return "", truncForLog(bodyStr), nil
	}

	// Token may be in the X-Auth-Token header or in the JSON body
	// (matches fc_client._extract_token at fc_client.py:156-194).
	if h := resp.Header.Get("X-Auth-Token"); h != "" {
		return h, bodyStr, nil
	}
	var parsed struct {
		Token         string `json:"token"`
		AccessToken   string `json:"access_token"`
		XsrfToken     string `json:"xsrf-token"`
	}
	if jerr := json.Unmarshal(bodyBytes, &parsed); jerr == nil {
		if parsed.Token != "" {
			return parsed.Token, bodyStr, nil
		}
		if parsed.AccessToken != "" {
			return parsed.AccessToken, bodyStr, nil
		}
		if parsed.XsrfToken != "" {
			return parsed.XsrfToken, bodyStr, nil
		}
	}
	return "", bodyStr, nil
}

// Logout terminates the session. Mirrors fc_client.logout (line 187-194):
// DELETE /service/session with the X-Auth-Token header.
func (c *Client) Logout(ctx context.Context) error {
	if c.token == "" {
		return nil
	}
	endpoint := c.baseURL + "/session"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func bytesOrNil(body any) io.Reader {
	if body == nil {
		return nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return bytes.NewReader(b)
}

// dedupPorts returns [first, others...] with duplicates removed,
// preserving first occurrence. Mirrors dict.fromkeys() at
// fc_client.py:86.
func dedupPorts(first int, others []int) []int {
	seen := map[int]bool{first: true}
	out := []int{first}
	for _, p := range others {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// isConnError reports whether err is a connection-level error
// (DNS failure, refused, reset, timeout) where the port should be
// considered closed and the loop advanced to the next port. Matches
// the except branches in fc_client.py:134-141.
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	if strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "network is unreachable") ||
		strings.Contains(s, "i/o timeout") {
		return true
	}
	// Go's http client returns url.Error wrapping a net.Error whose
	// Timeout() reports true for read timeouts.
	var ue *url.Error
	if errors.As(err, &ue) {
		var ne interface{ Timeout() bool }
		if errors.As(ue.Err, &ne) && ne.Timeout() {
			return true
		}
	}
	return false
}

func truncForLog(s string) string {
	const max = 200
	if len(s) > max {
		return s[:max]
	}
	return s
}
