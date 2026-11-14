// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("crypto", `
randBytes(int) bytes
randInt(int, int) int
hkdf(string, bytes, bytes, bytes, int) bytes
pbkdf2(string, bytes, bytes, int, int) bytes
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
hashPassword(string, string) string
verifyPassword(string, string) bool
argon2id(bytes, bytes, int, int, int, int) bytes
scrypt(bytes, bytes, int, int, int, int) bytes
mtweiKeygen() crypto.Keypair
mtweiId(string, string, bytes) bytes
mtweiClientKey(bytes, bytes, bytes, bytes) bytes
jwkPublic(bytes) string
jwkToPem(string) string
csr(bytes, list of string) bytes
`)
}
