// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Package config reads the configuration from environment variables.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr        string // HTTP listen address
	DataDir     string // database, backups and attachments
	BackupKeep  int    // number of daily database backups to keep
	MaxUploadMB int    // maximum size of a single attachment
	BaseURL     string // public address of MILE, for links in notifications
	NotifyHour  int    // local hour from which the daily notifications are sent

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPTLS      string // starttls, tls, none
}

func Load() Config {
	return Config{
		Addr:        env("MILE_ADDR", ":8080"),
		DataDir:     env("MILE_DATA_DIR", "data"),
		BackupKeep:  envInt("MILE_BACKUP_KEEP", 14),
		MaxUploadMB: envInt("MILE_MAX_UPLOAD_MB", 25),
		BaseURL:     strings.TrimRight(os.Getenv("MILE_BASE_URL"), "/"),
		NotifyHour:  envHour("MILE_NOTIFY_HOUR", 9),

		SMTPHost:     os.Getenv("MILE_SMTP_HOST"),
		SMTPPort:     envInt("MILE_SMTP_PORT", 0),
		SMTPUsername: os.Getenv("MILE_SMTP_USERNAME"),
		SMTPPassword: os.Getenv("MILE_SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("MILE_SMTP_FROM"),
		SMTPTLS:      strings.ToLower(env("MILE_SMTP_TLS", "starttls")),
	}
}

func (c Config) DBPath() string    { return filepath.Join(c.DataDir, "mile.db") }
func (c Config) BackupDir() string { return filepath.Join(c.DataDir, "backups") }
func (c Config) FilesDir() string  { return filepath.Join(c.DataDir, "files") }

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func envHour(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 && v <= 23 {
		return v
	}
	return def
}
