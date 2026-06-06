// Command fc-inventory is a one-shot CLI service that collects inventory
// from a Huawei FusionCompute VRM and writes a RVTools-style .xlsx
// workbook. All parameters come from a YAML config file; the service
// runs once and exits.
//
// Usage:
//
//	fc-inventory [--config fc-inventory.yaml] [--log-level info] [--dry-run]
//	fc-inventory version
//	fc-inventory mock-server [--port 17443]
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sukritphiboon/fc-inventory/internal/collector"
	"github.com/sukritphiboon/fc-inventory/internal/config"
	"github.com/sukritphiboon/fc-inventory/internal/excel"
	"github.com/sukritphiboon/fc-inventory/internal/fcclient"
	"github.com/sukritphiboon/fc-inventory/internal/logging"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// exitCode constants match the documented CLI conventions.
const (
	exitOK         = 0
	exitRuntime    = 1
	exitConfig     = 2
	exitCancelled  = 130
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "fc-inventory error:", err)
		os.Exit(exitFor(err))
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fc-inventory",
		Short: "FusionCompute inventory collector (Go CLI service)",
		Long: `fc-inventory is a one-shot CLI service that connects to a Huawei
FusionCompute VRM REST API, pulls sites/clusters/hosts/VMs/datastores/networks,
and writes a RVTools-style 10-sheet .xlsx workbook.

All parameters come from a YAML config file. Override the path with --config.
Authentication to FC uses auto-detect login (6 API versions x 3 auth methods x 3 ports).

Run 'fc-inventory collect' (the default) to perform one collection.
Run 'fc-inventory mock-server' to start a local mock FC API for testing.
Run 'fc-inventory version' to print the binary version.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// If no subcommand is given, default to collect. This is
		// implemented by having the root's RunE mirror collect's
		// behaviour; the flags live on the root for the no-subcommand
		// path and on collectCmd for the explicit path.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollect(rootCfgPath, rootLogLevel, rootDryRun, rootOutputDir, rootFilename)
		},
	}

	// Bind the collect-style flags to the root so `fc-inventory --config X`
	// works without the explicit "collect" subcommand.
	root.PersistentFlags().StringVar(&rootCfgPath, "config", "./fc-inventory.yaml", "path to YAML config file")
	root.PersistentFlags().StringVar(&rootLogLevel, "log-level", "", "override logging.level (debug|info|warn|error)")
	root.PersistentFlags().BoolVar(&rootDryRun, "dry-run", false, "parse config and show plan, no network calls")
	root.PersistentFlags().StringVar(&rootOutputDir, "output", "", "override output.directory from config")
	root.PersistentFlags().StringVar(&rootFilename, "filename", "", "override output.filename_prefix from config")

	root.AddCommand(collectCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(mockServerCmd())

	return root
}

// root-* flag holders shared with the rootCmd RunE.
var (
	rootCfgPath   string
	rootLogLevel  string
	rootDryRun    bool
	rootOutputDir string
	rootFilename  string
)

func collectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Run one collection and write the .xlsx file (default subcommand)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollect(rootCfgPath, rootLogLevel, rootDryRun, rootOutputDir, rootFilename)
		},
	}

	cmd.Flags().StringVar(&rootCfgPath, "config", "./fc-inventory.yaml", "path to YAML config file")
	cmd.Flags().StringVar(&rootLogLevel, "log-level", "", "override logging.level (debug|info|warn|error)")
	cmd.Flags().BoolVar(&rootDryRun, "dry-run", false, "parse config and show plan, no network calls")
	cmd.Flags().StringVar(&rootOutputDir, "output", "", "override output.directory from config")
	cmd.Flags().StringVar(&rootFilename, "filename", "", "override output.filename_prefix from config")

	return cmd
}

func versionCmd() *cobra.Command {
	var short bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the binary version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if short {
				fmt.Println(version)
			} else {
				fmt.Printf("fc-inventory %s\n", version)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&short, "short", false, "print just the semver string")
	return cmd
}

func mockServerCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "mock-server",
		Short: "Start a local mock FusionCompute API server (for offline testing)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMockServer(port)
		},
	}
	cmd.Flags().IntVar(&port, "port", 17443, "TCP port for the mock server")
	return cmd
}

// runCollect is the main one-shot collection flow.
func runCollect(cfgPath, logLevelFlag string, dryRun bool, outputDir, filename string) error {
	// 1. Load config (this is where env-var interpolation and validation happen).
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return errConfig{err: err}
	}

	// 2. Apply CLI overrides.
	if logLevelFlag != "" {
		cfg.Logging.Level = logLevelFlag
	}
	if outputDir != "" {
		cfg.Output.Directory = outputDir
	}
	if filename != "" {
		cfg.Output.FilenamePrefix = filename
	}
	if dryRun {
		cfg.Collection.DryRun = true
	}

	// 3. Wire up logging.
	logger, err := logging.Setup(&cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging setup error: %v\n", err)
		return errConfig{err: err}
	}
	// Re-install default to ensure subsequent log/slog.* calls use our handler.
	slog.SetDefault(logger)
	logger.Info("fc-inventory starting", "version", version, "config", cfgPath)

	// 4. Dry-run short-circuit.
	if cfg.Collection.DryRun {
		fmt.Print(cfg.AsEnvReport())
		return nil
	}

	// 5. Compute the output path. Delete the old one if overwrite=true
	//    (mirrors app.py:112-116).
	ts := time.Now().Format("20060102_150405")
	outputPath := filepath.Join(cfg.Output.Directory, fmt.Sprintf("%s_%s.xlsx", cfg.Output.FilenamePrefix, ts))
	if cfg.Output.Overwrite {
		if matches, _ := filepath.Glob(filepath.Join(cfg.Output.Directory, cfg.Output.FilenamePrefix+"_*.xlsx")); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}

	// 6. Build the FC client and collector.
	progress := func(percent int, step string) {
		fmt.Fprintf(os.Stderr, "[%3d%%] %s\n", percent, step)
	}
	fcClient := fcclient.New(cfg)
	col := collector.New(fcClient, cfg, progress)

	// 7. Run the collection under a context that SIGINT/SIGTERM cancels.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	sheets, err := col.CollectAll(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Warn("collection cancelled by signal")
			return errCancelled{err: err}
		}
		logger.Error("collection failed", "err", err)
		return errRuntime{err: err}
	}

	// 8. Write the .xlsx.
	if err := excel.Build(sheets, outputPath); err != nil {
		logger.Error("excel write failed", "err", err)
		return errRuntime{err: err}
	}
	logger.Info("excel written", "path", outputPath, "sheets", len(sheets))
	fmt.Fprintf(os.Stderr, "Excel saved: %s\n", outputPath)
	return nil
}

// exitFor maps an error to an exit code, honouring the documented
// conventions (0 ok, 1 runtime, 2 config, 130 cancelled).
func exitFor(err error) int {
	switch err.(type) {
	case errConfig:
		return exitConfig
	case errRuntime:
		return exitRuntime
	case errCancelled:
		return exitCancelled
	default:
		return exitRuntime
	}
}

// Typed error wrappers so exitFor can distinguish.
type errConfig struct{ err error }
func (e errConfig) Error() string { return e.err.Error() }
func (e errConfig) Unwrap() error { return e.err }

type errRuntime struct{ err error }
func (e errRuntime) Error() string { return e.err.Error() }
func (e errRuntime) Unwrap() error { return e.err }

type errCancelled struct{ err error }
func (e errCancelled) Error() string { return e.err.Error() }
func (e errCancelled) Unwrap() error { return e.err }

// runMockServer starts an in-process mock FC API on the given port.
// It uses the same canned JSON fixtures as the ./mockfc subcommand so
// the e2e flow can be exercised without a real FusionCompute.
//
// The mock accepts the third login attempt and returns 401 for the
// first two (exercising the auto-detect loop's "try next auth method"
// branch). All subsequent resource GETs resolve to JSON files in
// testdata/mock_fc by stripping /service/ and appending .json.
//
// The listener is HTTPS with an ephemeral self-signed cert because the
// client always uses `https://` URLs and disables TLS verify. This
// faithfully mirrors the real-FC deployment (self-signed certs).
func runMockServer(port int) error {
	dataDir := findMockDataDir()
	addr := "127.0.0.1:" + strconv.Itoa(port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	tlsCfg, err := generateSelfSignedTLS()
	if err != nil {
		return err
	}
	tlsLn := tls.NewListener(ln, tlsCfg)
	fmt.Fprintf(os.Stderr, "mock-fc listening on https://%s (self-signed cert; data: %s)\n", addr, dataDir)
	srv := &http.Server{Handler: newMockHandler(dataDir)}
	return srv.Serve(tlsLn)
}

// generateSelfSignedTLS builds a self-signed cert + key valid for
// 127.0.0.1 and localhost, 24h expiry. Used only by the mock server.
func generateSelfSignedTLS() (*tls.Config, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mock-fc"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

func newMockHandler(dataDir string) http.Handler {
	mux := http.NewServeMux()
	var attempts uint64
	mux.HandleFunc("/service/session", func(w http.ResponseWriter, r *http.Request) {
		n := attempts
		attempts++
		log.Printf("mock-fc: login attempt #%d %s", n+1, r.Method)
		if n < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"errorCode":"10000001","errorMessage":"mock: auth failed"}`)
			return
		}
		w.Header().Set("X-Auth-Token", "MOCK-TOKEN")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"token":"MOCK-TOKEN"}`)
	})
	mux.HandleFunc("/service/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		path := r.URL.Path
		// Strip the "/service/" prefix to get the resource name. The
		// client passes paths like /service/sites, /service/{uri}/vms,
		// /service/vms/1, /service/vms/1/nics, etc.
		const prefix = "/service/"
		if !strings.HasPrefix(path, prefix) {
			http.NotFound(w, r)
			return
		}
		base := strings.TrimPrefix(path, prefix)
		// Try the last path segment first (e.g. "vms" -> vms.json).
		tail := base
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			tail = base[idx+1:]
		}
		candidates := []string{tail + ".json"}
		// Also try the full path with "/" -> "_" for nested fixtures
		// (e.g. "vms_1_nics" for /vms/1/nics).
		if strings.Contains(base, "/") {
			candidates = append(candidates, strings.ReplaceAll(base, "/", "_")+".json")
		}
		for _, c := range candidates {
			full := filepath.Join(dataDir, c)
			if data, err := os.ReadFile(full); err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"errorCode":"404","errorMessage":"mock-fc: no fixture for %s"}`, path)
	})
	return mux
}

func findMockDataDir() string {
	candidates := []string{
		"testdata/mock_fc",
		"../testdata/mock_fc",
		"../../testdata/mock_fc",
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(p, "sites.json")); err == nil {
			return p
		}
	}
	wd, _ := os.Getwd()
	for d := wd; d != filepath.Dir(d); d = filepath.Dir(d) {
		p := filepath.Join(d, "testdata", "mock_fc")
		if _, err := os.Stat(filepath.Join(p, "sites.json")); err == nil {
			return p
		}
	}
	log.Fatalf("could not find testdata/mock_fc starting from %s", wd)
	return ""
}
