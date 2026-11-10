// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("crypto", `
randBytes(int) bytes
randInt(int, int) int
hkdf(string, bytes, bytes, bytes, int) bytes
pbkdf2(string, bytes, bytes, int, int) bytes
encrypt(bytes, bytes) bytes
decrypt(bytes, bytes) bytes
signKeypair() crypto.Keypair
sign(bytes, bytes) bytes
verify(bytes, bytes, bytes) bool
x25519Keypair() crypto.Keypair
x25519(bytes, bytes) bytes
x25519Public(bytes) bytes
mtweiKeygen() crypto.Keypair
mtweiId(string, string, bytes) bytes
mtweiClientKey(bytes, bytes, bytes, bytes) bytes
rsaSign(string, bytes, bytes) bytes
rsaVerify(string, bytes, bytes, bytes) bool
ecdsaSign(string, bytes, bytes) bytes
ecdsaVerify(string, bytes, bytes, bytes) bool
rsaGenerateKey(int) bytes
ecGenerateKey(string) bytes
jwkPublic(bytes) string
jwkToPem(string) string
csr(bytes, list of string) bytes
mlkemKeypair() crypto.Keypair
mlkemEncapsulate(bytes) crypto.Encapsulation
mlkemDecapsulate(bytes, bytes) bytes
`)
}
