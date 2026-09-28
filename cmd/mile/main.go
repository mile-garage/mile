// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// MILE: self-hosted vehicle deadlines, expenses and fuel tracking.
//
// Usage:
//
//	mile                                    start the server
//	mile healthcheck                        check that the server responds (for Docker)
//	mile reset-password <username> <pass>   set a user's password (if locked out)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/mile-garage/mile/internal/backup"
	"github.com/mile-garage/mile/internal/config"
	"github.com/mile-garage/mile/internal/db"
	"github.com/mile-garage/mile/internal/server"
	"github.com/mile-garage/mile/internal/store"
	"github.com/mile-garage/mile/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg := config.Load()
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "healthcheck":
		err = healthcheck(cfg)
	case len(os.Args) > 1 && os.Args[1] == "reset-password":
		if len(os.Args) != 4 {
			fmt.Fprintln(os.Stderr, "usage: mile reset-password <username> <new password>")
			os.Exit(2)
		}
		err = resetPassword(cfg, os.Args[2], os.Args[3])
	case len(os.Args) > 1 && os.Args[1] == "version":
		fmt.Println(version)
	default:
		err = serve(cfg)
	}
	if err != nil {
		slog.Error("error", "err", err)
		os.Exit(1)
	}
}

func open(cfg config.Config) (*store.Store, error) {
	if err := os.MkdirAll(cfg.FilesDir(), 0o755); err != nil {
		return nil, err
	}
	d, err := db.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	// TZ decides when a day starts for the deadlines (default: Europe/Rome in the Docker image).
	return store.New(d, cfg.FilesDir(), time.Local), nil
}

func serve(cfg config.Config) error {
	st, err := open(cfg)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(version, st, web.Dist(), cfg.MaxUploadMB).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go backup.Run(ctx, st.DB, cfg.BackupDir(), cfg.BackupKeep, func() {
		if err := st.DeleteExpiredSessions(); err != nil {
			slog.Error("sessions cleanup", "err", err)
		}
	})

	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shut)
	}()
	slog.Info("MILE started", "version", version, "addr", cfg.Addr, "data", cfg.DataDir)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func resetPassword(cfg config.Config, username, password string) error {
	st, err := open(cfg)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	u, err := st.GetUserByName(username)
	if err != nil {
		return fmt.Errorf("user %q: %w", username, err)
	}
	if err := st.SetPassword(u.ID, password, ""); err != nil {
		return err
	}
	fmt.Printf("Password of %s changed; all their sessions were closed.\n", u.Username)
	return nil
}

func healthcheck(cfg config.Config) error {
	addr := cfg.Addr
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return nil
}
