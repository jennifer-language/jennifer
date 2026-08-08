// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package encodinglib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestInstallRegistersEveryEncodingBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"isAscii", "lenBytes", "lenRunes", "toText", "fromText", "encode", "decode", "codecs"} {
		if in.LookupNamespacedBuiltin("encoding", name) == nil {
			t.Errorf("encoding.%s is not registered", name)
		}
	}
}
