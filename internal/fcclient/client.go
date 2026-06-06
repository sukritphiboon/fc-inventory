package fcclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sukritphiboon/fc-inventory/internal/config"
)

// Client is the FusionCompute REST adapter. It is safe for concurrent
// use after Login() has succeeded; Login() itself is not goroutine-safe
// and should be called once.
type Client struct {
	cfg     *config.Config
	http    *http.Client
	baseURL string // e.g. https://10.0.0.10:7443/service, set on successful login
	token   string
	version string // working API version (e.g. "v8.0"), set on successful login
}

// New constructs a Client from a loaded *config.Config. The baseURL
// and token are empty until Login succeeds.
func New(cfg *config.Config) *Client {
	tr := &http.Transport{
		// FusionCompute ships with self-signed certs; InsecureSkipVerify
		// matches the original requests.Session.verify = False behaviour
		// (fc_client.py line 29). This is documented loudly in the
		// README and required by the config validation.
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: cfg.FC.InsecureTLS, MinVersion: parseTLSVersion(cfg.HTTP.TLSMinVersion)},
		DisableKeepAlives: cfg.HTTP.DisableKeepAlives,
		MaxIdleConns:      cfg.HTTP.MaxIdleConns,
		IdleConnTimeout:   time.Duration(cfg.HTTP.IdleConnTimeoutSecs) * time.Second,
	}
	hc := &http.Client{Transport: tr, Timeout: cfg.RequestTimeout()}
	return &Client{cfg: cfg, http: hc}
}

// BaseURL returns the working service base URL (e.g.
// "https://10.0.0.10:7443/service") or "" if Login has not run.
func (c *Client) BaseURL() string { return c.baseURL }

// Token returns the X-Auth-Token (empty if Login has not run).
func (c *Client) Token() string { return c.token }

// Version returns the negotiated API version (e.g. "v8.0") or "" if Login
// has not run.
func (c *Client) Version() string { return c.version }

// StripProtocol mirrors fc_client.py:22 — strip "https://" / "http://"
// prefix and trailing slashes from a host string. Used as a safety net
// in case the config validation in package config did not catch it.
func StripProtocol(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.Trim(host, "/")
	return host
}

// parseTLSVersion maps the YAML string to a tls.Version constant. The
// original code defaulted to the Go stdlib default (TLS 1.2 minimum),
// but we make the value explicit per the config schema.
func parseTLSVersion(s string) uint16 {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TLS10":
		return tls.VersionTLS10
	case "TLS11":
		return tls.VersionTLS11
	case "TLS12", "":
		return tls.VersionTLS12
	case "TLS13":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

// formatURL is a small helper used by login.go to render the per-port
// session URL without repeating the prefix logic.
func formatURL(host string, port int) string {
	return fmt.Sprintf("https://%s:%d/service/session", host, port)
}

// itoa is a local wrapper to avoid pulling in strconv at the call site
// for what is almost always a 4-5 digit number.
func itoa(i int) string { return strconv.Itoa(i) }
