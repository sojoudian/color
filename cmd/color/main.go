// Command color is a tiny web server for colorful Kubernetes demos.
//
// It serves a single HTML page whose background color is taken from the pod
// hostname prefix (a Deployment named "blue" produces pods named
// "blue-<rs>-<pod>", hence blue pages). See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/sojoudian/color/internal/server"
)

const maxHeaderBytes = 16 << 10 // plenty for a demo app; bounds per-conn memory

// version is stamped by the linker in release builds
// (-ldflags "-X main.version=v1.2.3"); otherwise the VCS revision from the
// build info is used.
var version = "dev"

const (
	defaultPort       = "8080"
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

// namespaceFile is where the service account admission controller mounts the
// pod namespace; used as a fallback when the NAMESPACE env var is not set.
const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

func main() {
	healthcheck := flag.Bool("healthcheck", false,
		"probe the local server and exit (for use as a container HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthcheck(port()))
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log); err != nil {
		log.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	cfg := server.Config{
		Hostname:  hostname(),
		Namespace: namespace(),
		Version:   buildVersion(),
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort("", port()),
		Handler:           server.New(cfg, log),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting HTTP server",
			"addr", srv.Addr,
			"version", cfg.Version,
			"hostname", cfg.Hostname,
			"namespace", cfg.Namespace,
		)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen on %s: %w", srv.Addr, err)
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("graceful shutdown: %w", shutdownErr)
	}
	// Drain the serve result so a listener failure that raced the shutdown
	// signal is never silently swallowed.
	serveErr := <-errCh
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr)
}

// runHealthcheck hits the local readiness endpoint, printing nothing on
// success so it can serve as a distroless-friendly exec HEALTHCHECK.
func runHealthcheck(port string) int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:" + port + "/readyz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return defaultPort
}

func hostname() string {
	if h := os.Getenv("HOSTNAME"); h != "" {
		return h
	}
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// namespace resolves the pod namespace from the NAMESPACE env var (Downward
// API), falling back to the mounted service account namespace file.
func namespace() string {
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		return ns
	}
	ns, err := os.ReadFile(namespaceFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(ns))
}

// buildVersion prefers the linker-stamped version and falls back to the VCS
// revision embedded by the Go toolchain.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return s.Value[:12]
			}
		}
	}
	return version
}
