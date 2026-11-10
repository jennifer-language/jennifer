# crypto known-answer fixtures

Made with an independent implementation (OpenSSL 3.6 through the `openssl` CLI
and Node.js `crypto`), read by `cryptolib_ops_test.go`. The keys are test keys
generated for these fixtures and protect nothing.

- `rsa.pem` / `rsa.pub`: RSA-2048, PKCS#8 private and PKIX public PEM.
- `ec256.pem` / `ec256.pub`, `ec384.pem` / `ec384.pub`: P-256 and P-384, SEC1
  private and PKIX public PEM.
- `msg`: the signed message.
- `rsa-sha256.sig` / `rsa-sha384.sig` / `rsa-sha512.sig`:
  `openssl dgst -<hash> -sign rsa.pem msg` (RSASSA-PKCS1-v1.5, deterministic).
- `ecdsa-<key>-<hash>.sig`: Node `crypto.sign(hash, msg, {dsaEncoding:
  "ieee-p1363"})`, the JOSE R||S form.
- `oaep-sha1.bin` / `oaep-sha256.bin`: Node `publicEncrypt` with OAEP padding
  and `oaepHash`, over the 32 bytes `a 32-byte key wrapped for you!!!`.
