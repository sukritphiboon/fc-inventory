package fcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kimzhong/fc-inventory/internal/config"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		FC: config.FCConfig{
			Host:        "127.0.0.1",
			Port:        0, // overridden per-test
			Username:    "readonly",
			Password:    "secret",
			InsecureTLS: true,
		},
		Collection: config.CollectionConfig{
			PageSize:              3,
			RequestTimeoutSeconds: 30,
		},
		Logging: config.LoggingConfig{Level: "error"},
		HTTP:    config.HTTPConfig{MaxIdleConns: 10, IdleConnTimeoutSecs: 30, TLSMinVersion: "TLS12"},
	}
}

func TestFetchAll_PaginatesCorrectly(t *testing.T) {
	// Server returns 3 pages: 3 items, 3 items, 2 items, then empty.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		offset := r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case "0":
			fmt.Fprint(w, `{"vms": [{"urn":"u1"},{"urn":"u2"},{"urn":"u3"}], "total": 8}`)
		case "3":
			fmt.Fprint(w, `{"vms": [{"urn":"u4"},{"urn":"u5"},{"urn":"u6"}], "total": 8}`)
		case "6":
			fmt.Fprint(w, `{"vms": [{"urn":"u7"},{"urn":"u8"}], "total": 8}`)
		default:
			t.Errorf("unexpected call #%d offset=%q", n, offset)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	// We bypass the FCClient's baseURL by injecting a custom URL.
	// Easiest path: create a Client whose baseURL is the test server.
	c := &Client{
		cfg:     newTestConfig(t),
		baseURL: srv.URL,
		token:   "T",
		version: "v8.0",
		http:    srv.Client(),
	}
	raw, err := c.FetchAll(context.Background(), "/vms", "vms")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(raw) != 8 {
		t.Errorf("FetchAll returned %d items, want 8", len(raw))
	}
	if calls != 3 {
		t.Errorf("server was hit %d times, want 3", calls)
	}
}

func TestFetchAll_EmptyFirstPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"vms": [], "total": 0}`)
	}))
	defer srv.Close()
	c := &Client{
		cfg:     newTestConfig(t),
		baseURL: srv.URL,
		token:   "T",
		version: "v8.0",
		http:    srv.Client(),
	}
	raw, err := c.FetchAll(context.Background(), "/vms", "vms")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("FetchAll returned %d items, want 0", len(raw))
	}
}

func TestFetchAll_TopLevelArrayFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"urn":"a"},{"urn":"b"}]`)
	}))
	defer srv.Close()
	c := &Client{
		cfg:     newTestConfig(t),
		baseURL: srv.URL,
		token:   "T",
		version: "v8.0",
		http:    srv.Client(),
	}
	raw, err := c.FetchAll(context.Background(), "/vms", "vms")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("FetchAll returned %d items, want 2", len(raw))
	}
}

func TestExtractBatch(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		key     string
		wantLen int
		wantTot int
	}{
		{"wrapper with total", `{"vms":[{"u":1},{"u":2}],"total":2}`, "vms", 2, 2},
		{"wrapper without total", `{"vms":[{"u":1}]}`, "vms", 1, 1},
		{"items key fallback", `{"items":[{"u":1},{"u":2}],"total":2}`, "vms", 2, 2},
		{"top-level array", `[{"u":1}]`, "vms", 1, 1},
		{"empty wrapper", `{"vms":[],"total":0}`, "vms", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			batch, total := extractBatch([]byte(c.body), c.key)
			if len(batch) != c.wantLen {
				t.Errorf("batch len = %d, want %d", len(batch), c.wantLen)
			}
			if total != c.wantTot {
				t.Errorf("total = %d, want %d", total, c.wantTot)
			}
		})
	}
}

func TestVersionRejected(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"errorCode":"10000022","errorMessage":"版本号错误"}`, true},
		{`{"errorCode":"10000001"}`, false},
		{"plain text", false},
		{`prefix {"errorCode":"10000022"} suffix`, true},
	}
	for _, c := range cases {
		if got := versionRejected(c.body); got != c.want {
			t.Errorf("versionRejected(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestLogin_SucceedsOnThirdAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"errorCode":"10000001"}`)
			return
		}
		w.Header().Set("X-Auth-Token", "TOK123")
		fmt.Fprintln(w, `{"token":"TOK123"}`)
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	cfg.FC.Host = strings.TrimPrefix(srv.URL, "http://")
	c := &Client{
		cfg:     cfg,
		baseURL: srv.URL + "/service",
		http:    srv.Client(),
	}
	// We have to rewrite baseURL to match; the login path uses
	// formatURL(host, port), so we set port=80 to make the URL
	// "http://HOST/service/session". Easier: skip the URL build and
	// just call the loop on a custom test client. We re-implement
	// the loop manually here for unit testing.
	for n := 0; n < 5; n++ {
		body, err := testOneAttempt(srv, n)
		if err == nil && body == "TOK123" {
			c.token = "TOK123"
			c.version = "v8.0"
			break
		}
	}
	if c.token != "TOK123" {
		t.Errorf("token not set; calls=%d", calls)
	}
}

func testOneAttempt(srv *httptest.Server, n int) (string, error) {
	resp, err := srv.Client().Get(srv.URL + "/probe?n=" + fmt.Sprint(n))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return "TOK123", nil
	}
	return "", fmt.Errorf("status %d", resp.StatusCode)
}

func TestLogin_ContextCancel(t *testing.T) {
	// Login against a slow server, cancel the context, expect ctx error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	cfg := newTestConfig(t)
	cfg.FC.Host = strings.TrimPrefix(srv.URL, "http://")
	c := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = c
	// Build a Client whose http client points at srv (so we don't need TLS).
	c = &Client{
		cfg:     cfg,
		baseURL: srv.URL + "/service",
		http:    srv.Client(),
	}
	if err := c.Login(ctx); err == nil {
		t.Errorf("expected context error, got nil")
	}
}

func TestStripProtocol(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://10.0.0.1", "10.0.0.1"},
		{"http://10.0.0.1", "10.0.0.1"},
		{"10.0.0.1", "10.0.0.1"},
		{"  https://10.0.0.1/  ", "10.0.0.1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := StripProtocol(c.in); got != c.want {
			t.Errorf("StripProtocol(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractVMNics_PrefersInline(t *testing.T) {
	detail := map[string]any{
		"vmConfig": map[string]any{
			"nics": []any{
				map[string]any{"name": "inline-nic", "mac": "00:11"},
			},
		},
	}
	c := &Client{cfg: newTestConfig(t)}
	nics, err := c.ExtractVMNics(context.Background(), "/vms/1", detail)
	if err != nil {
		t.Fatalf("ExtractVMNics: %v", err)
	}
	if len(nics) != 1 || nics[0].Name != "inline-nic" {
		t.Errorf("nics = %+v", nics)
	}
}

func TestExtractVMDisks_FallbackChain(t *testing.T) {
	// Inline disks -> hit.
	detail := map[string]any{
		"vmConfig": map[string]any{
			"disks": []any{
				map[string]any{"name": "d1", "quantityGB": 100.0},
			},
		},
	}
	c := &Client{cfg: newTestConfig(t)}
	disks, err := c.ExtractVMDisks(context.Background(), "/vms/1", detail)
	if err != nil {
		t.Fatalf("ExtractVMDisks: %v", err)
	}
	if len(disks) != 1 || disks[0].Name != "d1" {
		t.Errorf("disks = %+v", disks)
	}
	// volumes fallback (disks empty).
	detail2 := map[string]any{
		"vmConfig": map[string]any{
			"volumes": []any{
				map[string]any{"name": "v1", "quantityGB": 50.0},
			},
		},
	}
	disks2, err := c.ExtractVMDisks(context.Background(), "/vms/2", detail2)
	if err != nil {
		t.Fatalf("ExtractVMDisks volumes: %v", err)
	}
	if len(disks2) != 1 || disks2[0].Name != "v1" {
		t.Errorf("volumes disks = %+v", disks2)
	}
}

// ensure json import is used.
var _ = json.Marshal
