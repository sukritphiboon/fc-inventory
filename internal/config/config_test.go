package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fc-inventory.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestLoad_HappyPath(t *testing.T) {
	path := writeTempConfig(t, `
fc:
  host: "10.0.0.1"
  port: 7443
  username: "readonly"
  password: "literal-secret"
  insecure_tls: true
collection:
  page_size: 50
output:
  directory: "./out"
logging:
  level: "debug"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FC.Host != "10.0.0.1" {
		t.Errorf("Host = %q", cfg.FC.Host)
	}
	if cfg.FC.Port != 7443 {
		t.Errorf("Port = %d", cfg.FC.Port)
	}
	if cfg.FC.Password != "literal-secret" {
		t.Errorf("Password = %q", cfg.FC.Password)
	}
	if cfg.Collection.PageSize != 50 {
		t.Errorf("PageSize = %d", cfg.Collection.PageSize)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Level = %q", cfg.Logging.Level)
	}
}

func TestLoad_EnvInterpolation(t *testing.T) {
	t.Setenv("FC_PASSWORD", "from-env-secret")
	path := writeTempConfig(t, `
fc:
  host: "10.0.0.1"
  username: "readonly"
  password: "${FC_PASSWORD}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FC.Password != "from-env-secret" {
		t.Errorf("Password = %q, want from-env-secret", cfg.FC.Password)
	}
}

func TestLoad_MissingEnvVarFails(t *testing.T) {
	os.Unsetenv("FC_PASSWORD")
	path := writeTempConfig(t, `
fc:
  host: "10.0.0.1"
  username: "readonly"
  password: "${FC_PASSWORD_MISSING}"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "FC_PASSWORD_MISSING") {
		t.Errorf("error = %v, want it to mention FC_PASSWORD_MISSING", err)
	}
}

func TestLoad_StripProtocol(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://10.0.0.1", "10.0.0.1"},
		{"http://10.0.0.1/", "10.0.0.1"},
		{"10.0.0.1", "10.0.0.1"},
		{"  https://10.0.0.1/  ", "10.0.0.1"},
	}
	for _, c := range cases {
		path := writeTempConfig(t, "fc:\n  host: \""+c.in+"\"\n  username: u\n  password: p\n")
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load(%q): %v", c.in, err)
		}
		if cfg.FC.Host != c.want {
			t.Errorf("Host(%q) = %q, want %q", c.in, cfg.FC.Host, c.want)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	path := writeTempConfig(t, `
fc:
  host: "10.0.0.1"
  username: "u"
  password: "p"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FC.Port != 7443 {
		t.Errorf("Port default = %d, want 7443", cfg.FC.Port)
	}
	if !cfg.FC.InsecureTLS {
		t.Errorf("InsecureTLS default = false, want true")
	}
	if cfg.Output.FilenamePrefix != "FC_Inventory" {
		t.Errorf("Prefix default = %q, want FC_Inventory", cfg.Output.FilenamePrefix)
	}
	if cfg.Logging.MaxSizeMB != 5 {
		t.Errorf("MaxSizeMB default = %d, want 5", cfg.Logging.MaxSizeMB)
	}
}

func TestValidate_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing host", "fc: { username: u, password: p }", "host"},
		{"missing username", "fc: { host: h, password: p }", "username"},
		{"missing password", "fc: { host: h, username: u }", "password"},
		{"bad level", "fc: { host: h, username: u, password: p }\nlogging: { level: wrong }", "level"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempConfig(t, c.body)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestLogValue_RedactsPassword(t *testing.T) {
	path := writeTempConfig(t, `
fc:
  host: "10.0.0.1"
  username: "u"
  password: "supersecret"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The redacted view must not contain the password.
	view := cfg.LogValue()
	if strings.Contains(fmtSprint(view), "supersecret") {
		t.Errorf("LogValue leaked the password: %v", view)
	}
}

// fmtSprint avoids importing fmt at the test top-level just for one
// call. It mimics fmt.Sprint for slogValue (which holds an any).
func fmtSprint(v any) string {
	switch x := v.(type) {
	case slogValue:
		// slogValue is an internal type; we cannot compare directly.
		// We do a stringification via a series of well-known field
		// names to make the redacted-password assertion.
		return stringOfAny(x.v)
	default:
		return ""
	}
}

func stringOfAny(v any) string {
	// Recursively string-format; the only purpose of this test helper
	// is to assert that "supersecret" does not appear.
	type stringer interface{ String() string }
	if s, ok := v.(stringer); ok {
		return s.String()
	}
	if s, ok := v.(string); ok {
		return s
	}
	// Use a JSON-ish serialiser via %v. Avoid fmt import? It is in
	// stdlib so it's fine. We use it via the global func to keep the
	// helper names short.
	return defaultFormat(v)
}
