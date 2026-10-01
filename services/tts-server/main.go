// Command tts-server is a small self-hostable text-to-speech HTTP API built
// on espeak-ng and Piper. See README.md and /docs for the API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/app"
	"github.com/spagu/ssg/services/tts-server/internal/config"
	"github.com/spagu/ssg/services/tts-server/internal/execx"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// shutdownGrace is how long in-flight requests get after SIGTERM.
const shutdownGrace = 30 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.LookupEnv, os.Stderr)
	stop()
	os.Exit(code)
}

// run parses flags and config, then serves until ctx is cancelled. It
// returns the process exit code.
func run(ctx context.Context, args []string, lookup config.Lookup, stderr io.Writer) int {
	fs := flag.NewFlagSet("tts-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	health := fs.Bool("healthcheck", false, "probe /healthz on the configured address and exit 0 when healthy")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stderr, "tts-server", version)
		return 0
	}
	cfg, err := config.Load(lookup)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "config:", err)
		return 2
	}
	if *health {
		return healthcheck(ctx, cfg.Addr)
	}
	log := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := serve(ctx, cfg, log); err != nil {
		log.Error("server stopped", "err", err)
		return 1
	}
	return 0
}

// serve builds the app and runs the HTTP server until ctx is done, then
// shuts down gracefully.
func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	a, err := app.Build(ctx, cfg, log, execx.ExecRunner{})
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()
	srv := app.NewServer(cfg, a.Handler, log)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.Addr, "version", version)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck is used by the container HEALTHCHECK: no curl in the image.
func healthcheck(ctx context.Context, addr string) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, port) + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- loopback probe of our own address
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
