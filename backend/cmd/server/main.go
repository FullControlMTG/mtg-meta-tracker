package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/analytics"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/auth"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/config"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/decklist"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/httpapi"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/ingest"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/jobs"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/moxfield"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/scryfall"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

func main() {
	cfg := config.Load()

	rootCtx := context.Background()
	pool, err := pgxpool.New(rootCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(rootCtx); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	st := store.New(pool)

	if err := st.EnsureSchema(rootCtx); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	if err := bootstrapAdmin(rootCtx, st, cfg); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	scry := scryfall.New(cfg.ScryfallUserAgent, cfg.ScryfallMinInterval)
	mox := moxfield.New(cfg.ScryfallUserAgent)
	syncer := ingest.NewSyncer(st, scry, mox)
	resolver := decklist.NewResolver(st, scry)
	engine := analytics.NewEngine(st, cfg)

	workerCtx, cancelWorker := context.WithCancel(rootCtx)
	defer cancelWorker()
	worker := jobs.NewWorker(st, 2*time.Second)
	worker.Register("sync_cube", func(ctx context.Context, payload json.RawMessage) error {
		var p struct {
			CubeID string `json:"cube_id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		id, err := uuid.Parse(p.CubeID)
		if err != nil {
			return err
		}
		return syncer.SyncCube(ctx, id)
	})
	worker.Register("recompute_analytics", func(ctx context.Context, payload json.RawMessage) error {
		var p struct {
			CubeID  string `json:"cube_id"`
			Trigger string `json:"trigger"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		id, err := uuid.Parse(p.CubeID)
		if err != nil {
			return err
		}
		trigger := p.Trigger
		if trigger == "" {
			trigger = "manual"
		}
		return engine.Recompute(ctx, id, trigger)
	})
	go worker.Run(workerCtx)

	scheduler := jobs.NewScheduler(st, cfg.SyncInterval)
	go scheduler.Run(workerCtx)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(st, cfg, resolver).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancelWorker()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func bootstrapAdmin(ctx context.Context, st *store.Store, cfg config.Config) error {
	if cfg.BootstrapAdminUsername == "" || cfg.BootstrapAdminEmail == "" || cfg.BootstrapAdminPassword == "" {
		return nil
	}
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	u := &domain.User{
		Username:     cfg.BootstrapAdminUsername,
		Email:        cfg.BootstrapAdminEmail,
		DisplayName:  cfg.BootstrapAdminUsername,
		Role:         domain.RoleAdmin,
		PasswordHash: &hash,
	}
	if err := st.CreateUser(ctx, u); err != nil {
		return err
	}
	log.Printf("bootstrapped admin user %q", u.Username)
	return nil
}
