// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix

package main

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// private keeps the data readable by the user running MILE only: the
// database, its backups and the attachments hold tokens and documents. New
// files get it from the umask; those of older versions are fixed once here.
// On a filesystem with inherited ACLs (e.g. a TrueNAS dataset made with the
// Apps preset) the ACL wins over both, and the files stay as it says.
func private(dataDir string) {
	syscall.Umask(0o077)
	warned := false
	filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mode := fs.FileMode(0o600)
		if d.IsDir() {
			mode = 0o700
		}
		if info.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(p, info.Mode().Perm()&mode); err != nil && !warned {
				slog.Warn("cannot restrict the permissions of the data", "path", p, "err", err)
				warned = true
			}
		}
		return nil
	})
}
