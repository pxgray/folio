package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/web"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "usage: folio serve [--db path]\n")
		os.Exit(1)
	}

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	defaultDB := os.Getenv("FOLIO_DB")
	if defaultDB == "" {
		defaultDB = "folio.db"
	}
	dbPath := serveCmd.String("db", defaultDB, "path to SQLite database file")
	if err := serveCmd.Parse(os.Args[2:]); err != nil {
		log.Fatalf("folio: %v", err)
	}

	store, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("folio: open db: %v", err)
	}
	defer store.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Background session cleanup.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := store.DeleteExpiredSessions(ctx); err != nil {
					log.Printf("folio: session cleanup: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	authn := auth.New(store)

	staticFS, err := fs.Sub(assets.StaticFS, "static")
	if err != nil {
		log.Fatalf("folio: static fs: %v", err)
	}

	setupComplete, err := store.IsSetupComplete(ctx)
	if err != nil {
		log.Fatalf("folio: check setup: %v", err)
	}

	var docHandler http.Handler
	var docSrv *web.Server
	var gitStore *gitstore.Store
	addr := ":8080"

	if setupComplete {
		// Load settings.
		if v, err := store.GetSetting(ctx, "addr"); err == nil && v != "" {
			addr = v
		}
		cacheDir := "~/.cache/folio"
		if v, err := store.GetSetting(ctx, "cache_dir"); err == nil && v != "" {
			cacheDir = v
		}
		staleTTL := 5 * time.Minute
		if v, err := store.GetSetting(ctx, "stale_ttl"); err == nil && v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				staleTTL = d
			}
		}

		gitStore = gitstore.New(cacheDir, staleTTL)

		// Hydrate gitstore from DB.
		repos, err := store.ListAllRepos(ctx)
		if err != nil {
			log.Fatalf("folio: list repos: %v", err)
		}
		entries := make([]gitstore.RepoEntry, 0, len(repos))
		for _, r := range repos {
			if r.Status == db.RepoStatusReady || r.Status == db.RepoStatusPending {
				entries = append(entries, gitstore.RepoEntry{
					Host:      r.Host,
					Owner:     r.RepoOwner,
					Name:      r.RepoName,
					RemoteURL: r.RemoteURL,
				})
			}
		}
		if err := gitStore.EnsureRepos(ctx, entries); err != nil {
			log.Printf("folio: EnsureRepos: %v", err)
		}

		docSrv, err = web.New(store, gitStore, assets.TemplateFS, staticFS)
		if err != nil {
			log.Fatalf("folio: web.New: %v", err)
		}
		docHandler = docSrv.Handler()
	}

	dashSrv := dashboard.New(store, gitStore, authn, docSrv, assets.TemplateFS, setupComplete)
	dashHandler := dashSrv.Handler()

	combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/-/setup") ||
			strings.HasPrefix(p, "/-/auth") ||
			strings.HasPrefix(p, "/-/dashboard") ||
			strings.HasPrefix(p, "/-/api") {
			dashHandler.ServeHTTP(w, r)
			return
		}
		if docHandler != nil {
			docHandler.ServeHTTP(w, r)
		} else {
			http.Redirect(w, r, "/-/setup", http.StatusSeeOther)
		}
	})

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      combined,
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

	log.Printf("folio: listening on %s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("folio: listen: %v", err)
	}
}
