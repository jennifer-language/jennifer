// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !unix

package fslib

// syncDirAt does nothing where a directory cannot be fsynced: Windows has no
// flush for a directory handle (NTFS journals its metadata), and WASI hosts
// have no such call either. fs.syncDir has checked the path is a directory.
func syncDirAt(string) error { return nil }
