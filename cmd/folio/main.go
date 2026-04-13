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
	"github.com/pxgray/folio/internal/db"
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

	store := gitstore.New(cfg.Cache.Dir, cfg.Cache.StaleTTL)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	entries := make([]gitstore.RepoEntry, 0, len(cfg.Repos))
	for _, rc := range cfg.Repos {
		entries = append(entries, gitstore.RepoEntry{
			Host:      rc.Host,
			Owner:     rc.Owner,
			Name:      rc.Repo,
			RemoteURL: rc.Remote,
		})
	}

	log.Printf("folio: cloning / opening %d repo(s)...", len(entries))
	if err := store.EnsureRepos(ctx, entries); err != nil {
		log.Fatalf("folio: %v", err)
	}

	if len(cfg.Locals) > 0 {
		locals := make([]gitstore.LocalEntry, 0, len(cfg.Locals))
		for _, lc := range cfg.Locals {
			locals = append(locals, gitstore.LocalEntry{
				Label:       lc.Label,
				Path:        lc.Path,
				TrustedHTML: lc.TrustedHTML,
			})
		}
		log.Printf("folio: registering %d local repo(s)...", len(locals))
		if err := store.OpenLocals(locals); err != nil {
			log.Fatalf("folio: %v", err)
		}
	}

	// Temporary: use in-memory db seeded from config.
	// Phase 3 will replace this with a persistent DB and setup wizard.
	dbStore, err := db.Open(":memory:")
	if err != nil {
		log.Fatalf("folio: db: %v", err)
	}
	defer dbStore.Close()
	if err := seedDBFromConfig(ctx, dbStore, cfg); err != nil {
		log.Fatalf("folio: seed db: %v", err)
	}

	staticFS, err := fs.Sub(assets.StaticFS, "static")
	if err != nil {
		log.Fatalf("folio: static fs: %v", err)
	}

	srv, err := web.New(dbStore, store, assets.TemplateFS, staticFS)
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

// seedDBFromConfig populates an in-memory db.Store from TOML config so that
// TrustedHTML and WebhookSecret settings take effect. This is a temporary bridge
// until Phase 3 introduces a persistent database and setup wizard.
func seedDBFromConfig(ctx context.Context, dbStore db.Store, cfg *config.Config) error {
	sysUser := &db.User{
		Email:   "system@folio.local",
		Name:    "system",
		IsAdmin: true,
	}
	if err := dbStore.CreateUser(ctx, sysUser); err != nil {
		return fmt.Errorf("create system user: %w", err)
	}
	for _, rc := range cfg.Repos {
		r := &db.Repo{
			OwnerID:       sysUser.ID,
			Host:          rc.Host,
			RepoOwner:     rc.Owner,
			RepoName:      rc.Repo,
			RemoteURL:     rc.Remote,
			WebhookSecret: rc.WebhookSecret,
			TrustedHTML:   rc.TrustedHTML,
			StaleTTLSecs:  int64(cfg.Cache.StaleTTL.Seconds()),
			Status:        db.RepoStatusReady,
		}
		if err := dbStore.CreateRepo(ctx, r); err != nil {
			return fmt.Errorf("insert repo %s: %w", rc.Key(), err)
		}
	}
	return nil
}
