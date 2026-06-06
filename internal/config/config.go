// Package config loads the YAML configuration for the fc-inventory service
// and resolves ${ENV} placeholders inside string fields.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root of the fc-inventory YAML schema.
type Config struct {
	FC         FCConfig         `yaml:"fc"`
	Collection CollectionConfig `yaml:"collection"`
	Output     OutputConfig     `yaml:"output"`
	Logging    LoggingConfig    `yaml:"logging"`
	HTTP       HTTPConfig       `yaml:"http"`
}

// FCConfig describes the FusionCompute target.
type FCConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	InsecureTLS bool   `yaml:"insecure_tls"`
}

// CollectionConfig tunes per-run collection behaviour.
type CollectionConfig struct {
	PageSize                int  `yaml:"page_size"`
	RequestTimeoutSeconds   int  `yaml:"request_timeout_seconds"`
	PerResourceTimeoutSecs  int  `yaml:"per_resource_timeout_seconds"`
	IncludeExtraFields      bool `yaml:"include_extra_fields"`
	DryRun                  bool `yaml:"dry_run"`
}

// OutputConfig controls the produced .xlsx file.
type OutputConfig struct {
	Directory      string `yaml:"directory"`
	FilenamePrefix string `yaml:"filename_prefix"`
	Overwrite      bool   `yaml:"overwrite"`
}

// LoggingConfig controls the rotating log file.
type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAgeDays int    `yaml:"max_age_days"`
	Compress   bool   `yaml:"compress"`
}

// HTTPConfig tunes the underlying net/http client transport.
type HTTPConfig struct {
	DisableKeepAlives    bool `yaml:"disable_keep_alives"`
	MaxIdleConns         int  `yaml:"max_idle_conns"`
	IdleConnTimeoutSecs  int  `yaml:"idle_conn_timeout_seconds"`
	TLSMinVersion        string `yaml:"tls_min_version"`
}

// Defaults fills in zero-valued fields with the documented defaults.
// Mirrors the behaviour of the original Python app (env-var overrides
// were only for bind/port; everything else had implicit defaults).
func (c *Config) ApplyDefaults() {
	if c.FC.Port == 0 {
		c.FC.Port = 7443
	}
	if !c.FC.InsecureTLS {
		// Default is true: FC ships with self-signed certs.
		c.FC.InsecureTLS = true
	}
	if c.Collection.PageSize == 0 {
		c.Collection.PageSize = 100
	}
	if c.Collection.RequestTimeoutSeconds == 0 {
		c.Collection.RequestTimeoutSeconds = 60
	}
	if c.Collection.PerResourceTimeoutSecs == 0 {
		c.Collection.PerResourceTimeoutSecs = 30
	}
	if c.Output.Directory == "" {
		c.Output.Directory = "."
	}
	if c.Output.FilenamePrefix == "" {
		c.Output.FilenamePrefix = "FC_Inventory"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.File == "" {
		c.Logging.File = "fc_inventory.log"
	}
	if c.Logging.MaxSizeMB == 0 {
		c.Logging.MaxSizeMB = 5
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 3
	}
	if c.Logging.MaxAgeDays == 0 {
		c.Logging.MaxAgeDays = 30
	}
	if c.HTTP.MaxIdleConns == 0 {
		c.HTTP.MaxIdleConns = 50
	}
	if c.HTTP.IdleConnTimeoutSecs == 0 {
		c.HTTP.IdleConnTimeoutSecs = 90
	}
	if c.HTTP.TLSMinVersion == "" {
		c.HTTP.TLSMinVersion = "TLS12"
	}
}

// Validate enforces required fields and basic sanity.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.FC.Host) == "" {
		return errors.New("fc.host is required")
	}
	// Auto-strip protocol prefix to match the original Python behaviour
	// (fc_client.py line 22).
	c.FC.Host = strings.TrimSpace(c.FC.Host)
	c.FC.Host = strings.TrimPrefix(c.FC.Host, "https://")
	c.FC.Host = strings.TrimPrefix(c.FC.Host, "http://")
	c.FC.Host = strings.Trim(c.FC.Host, "/")
	if c.FC.Host == "" {
		return errors.New("fc.host is empty after stripping protocol prefix")
	}
	if c.FC.Username == "" {
		return errors.New("fc.username is required")
	}
	if c.FC.Password == "" {
		return errors.New("fc.password is required (use ${ENV} to source from environment)")
	}
	if c.FC.Port < 1 || c.FC.Port > 65535 {
		return fmt.Errorf("fc.port %d is out of range", c.FC.Port)
	}
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("logging.level %q is invalid (debug|info|warn|error)", c.Logging.Level)
	}
	return nil
}

// Expand walks all *string fields and substitutes ${VAR} with os.Getenv(VAR).
// An empty result (i.e. the env var was unset and the placeholder was the
// whole value) is reported back to the caller as a missing-env error so the
// service fails fast instead of silently authenticating with "".
func (c *Config) Expand() error {
	missing := []string{}
	walkStrings(reflect.ValueOf(c).Elem(), func(field string) string {
		return expandEnv(field, &missing)
	})
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables referenced in config: %s",
			strings.Join(missing, ", "))
	}
	return nil
}

// walkStrings applies fn to every string reachable from v (a struct root).
func walkStrings(v reflect.Value, fn func(string) string) {
	if !v.IsValid() {
		return
	}
	walkValue(v, fn)
}

func walkValue(v reflect.Value, fn func(string) string) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			v.SetString(fn(v.String()))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			walkValue(v.Field(i), fn)
		}
	}
}

// expandEnv returns the input with ${VAR} replaced by os.Getenv(VAR).
// A ${VAR} that resolves to "" and was not just empty from the start is
// recorded in missing so the caller can produce a clear error.
func expandEnv(s string, missing *[]string) string {
	return os.Expand(s, func(name string) string {
		v, ok := os.LookupEnv(name)
		if !ok {
			*missing = append(*missing, name)
			return ""
		}
		return v
	})
}

// LogValue renders a redacted view of the config for slog. The password
// is replaced with "***" and never reaches the log sink.
func (c *Config) LogValue() slogValue {
	redacted := *c
	redacted.FC.Password = "***"
	return slogValue{v: redacted}
}

// slogValue is a tiny adapter so we can attach a LogValue method without
// importing log/slog (which would create a cycle through the logging
// package's own imports).
type slogValue struct{ v any }

// Load reads, parses, defaults, validates, and expands the YAML file at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := &Config{}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true) // fail on typos
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Expand(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// RequestTimeout returns the per-request FC API timeout as a time.Duration.
func (c *Config) RequestTimeout() time.Duration {
	return time.Duration(c.Collection.RequestTimeoutSeconds) * time.Second
}

// PerResourceTimeout returns the per-resource timeout (used by fetchAll).
func (c *Config) PerResourceTimeout() time.Duration {
	return time.Duration(c.Collection.PerResourceTimeoutSecs) * time.Second
}

// AsEnvReport formats the (redacted) config as a human-readable multi-line
// string for the --dry-run banner.
func (c *Config) AsEnvReport() string {
	var b strings.Builder
	b.WriteString("[DRY-RUN] Target: " + c.FC.Host + ":" + strconv.Itoa(c.FC.Port) + " as " + c.FC.Username + "\n")
	b.WriteString("[DRY-RUN] Will collect: sites, clusters, hosts, host_details, datastores, dvswitches, portgroups, vms, vm_details\n")
	b.WriteString("[DRY-RUN] Output file: " + c.Output.Directory + "/" + c.Output.FilenamePrefix + "_<timestamp>.xlsx\n")
	if c.FC.InsecureTLS {
		b.WriteString("[DRY-RUN] TLS verify: disabled (self-signed FC certs)\n")
	} else {
		b.WriteString("[DRY-RUN] TLS verify: enabled\n")
	}
	b.WriteString("[DRY-RUN] Page size: " + strconv.Itoa(c.Collection.PageSize) + "\n")
	b.WriteString("[DRY-RUN] Log level: " + c.Logging.Level + " -> " + c.Logging.File + "\n")
	b.WriteString("[DRY-RUN] No network calls made.\n")
	return b.String()
}
