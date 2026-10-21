// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package libsig

func init() {
	add("crypto", `
randBytes(int) bytes
randInt(int, int) int
hmacEqual(bytes, bytes) bool
hkdf(bytes, bytes, bytes, int, string) bytes
pbkdf2(bytes, bytes, int, int, string) bytes
encrypt(bytes, bytes) bytes
decrypt(bytes, bytes) bytes
signKeypair() crypto.Keypair
sign(bytes, bytes) bytes
verify(bytes, bytes, bytes) bool
mtweiKeygen() crypto.Keypair
mtweiId(string, string, bytes) bytes
mtweiClientKey(bytes, bytes, bytes, bytes) bytes
rsaSign(bytes, bytes, string) bytes
rsaVerify(bytes, bytes, bytes, string) bool
ecdsaSign(bytes, bytes, string) bytes
ecdsaVerify(bytes, bytes, bytes, string) bool
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
