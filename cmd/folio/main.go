package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/config"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/web"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: folio <config.toml>\n")
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		log.Fatalf("folio: %v", err)
	}

	store := gitstore.New(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("folio: cloning / opening %d repo(s)...", len(cfg.Repos))
	if err := store.EnsureCloned(ctx); err != nil {
		log.Fatalf("folio: %v", err)
	}

	// Expose the static/ subdirectory of the embedded FS.
	staticFS, err := fs.Sub(assets.StaticFS, "static")
	if err != nil {
		log.Fatalf("folio: static fs: %v", err)
	}

	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		log.Fatalf("folio: %v", err)
	}

	httpSrv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Printf("folio: shutting down...")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("folio: listening on %s", cfg.Server.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("folio: listen: %v", err)
	}
}
