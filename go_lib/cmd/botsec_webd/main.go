package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_lib/core/webbridge"

	// Import all plugins to trigger init() registration.
	_ "go_lib/plugins/dintalclaw"
	_ "go_lib/plugins/nullclaw"
	_ "go_lib/plugins/openclaw"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "web bridge listen address")
	webRoot := flag.String("web-root", os.Getenv("BOTSEC_WEB_STATIC_DIR"), "optional static web root directory")
	flag.Parse()

	startPprofIfNeeded()

	server := &http.Server{
		Addr:              *addr,
		Handler:           webbridge.NewServer(*webRoot).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("[webbridge] listening on http://%s\n", *addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		fmt.Printf("[webbridge] received signal: %s, shutting down\n", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "[webbridge] server failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func startPprofIfNeeded() {
	pprofPort := os.Getenv("BOTSEC_PPROF_PORT")
	if pprofPort == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	addr := "127.0.0.1:" + pprofPort
	fmt.Printf("[pprof] profiling server: http://%s/debug/pprof/\n", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			fmt.Fprintf(os.Stderr, "[pprof] server failed: %v\n", err)
		}
	}()
}
