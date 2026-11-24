// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	addDataKind("crypto", "crypto")
	add("crypto", `
randBytes(int) bytes
randInt(int, int) int
deriveKey(string, bytes, bytes, int, crypto.KdfParams) bytes
encrypt(string, bytes, bytes, bytes, bytes) bytes
decrypt(string, bytes, bytes, bytes, bytes) bytes
blockEncrypt(string, bytes, bytes, bytes|null) bytes
blockDecrypt(string, bytes, bytes, bytes|null) bytes
sign(string, bytes, bytes) bytes
verify(string, bytes, bytes, bytes) bool
keypair(string) crypto.Keypair
publicKey(string, bytes) bytes
agree(string, bytes, bytes) bytes
encapsulate(string, bytes) crypto.Encapsulation
decapsulate(string, bytes, bytes) bytes
wrap(string, bytes, bytes) bytes
unwrap(string, bytes, bytes) bytes
hashPassword(string, bytes|string) string
verifyPassword(bytes|string, string) bool
mtweiKeygen() crypto.Keypair
mtweiId(string, string, bytes) bytes
mtweiClientKey(bytes, bytes, bytes, bytes) bytes
jwkPublic(bytes) string
jwkToPem(string) bytes
csr(bytes, list of string) bytes
`)
}
