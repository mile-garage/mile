// Package config reads the configuration from environment variables.
package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Addr        string // HTTP listen address
	DataDir     string // database, backups and attachments
	BackupKeep  int    // number of daily database backups to keep
	MaxUploadMB int    // maximum size of a single attachment
}

func Load() Config {
	return Config{
		Addr:        env("MILE_ADDR", ":8080"),
		DataDir:     env("MILE_DATA_DIR", "data"),
		BackupKeep:  envInt("MILE_BACKUP_KEEP", 14),
		MaxUploadMB: envInt("MILE_MAX_UPLOAD_MB", 25),
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
