// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

//go:build !unix

package main

// private does nothing where there are no Unix permissions (Windows).
func private(string) {}
