# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# barcode_ecc.j - Reed-Solomon error correction over GF(256), the field math
# behind QR codes. Included (textual splice) into barcode.j; it declares no
# `use` of its own and relies on the includer's imports (`lists`). Private to
# the barcode module - not an exported surface. If a second consumer outside
# barcode ever appears, extract this into a cohesive `ecc.j` module.
#
# GF(256) uses the QR primitive polynomial x^8 + x^4 + x^3 + x^2 + 1 (0x11d).

# The exp / log tables of GF(256) (a value-semantic bundle threaded through the
# field ops, since a declarations-only module holds no mutable state).
def struct GF {
    exp as list of int,
    log as list of int
};

# buildGF constructs the GF(256) exp / log tables.
func buildGF() {
    def exp as list of int init [];
    def log as list of int init [];
    def i as int init 0;
    while ($i < 256) {
        $log[] = 0;
        $i = $i + 1;
    }
    def x as int init 1;
    $i = 0;
    while ($i < 255) {
        $exp[] = $x;
        $log[$x] = $i;
        $x = $x << 1;
        if (($x & 0x100) > 0) {
            $x = $x ^ 0x11d;
        }
        $i = $i + 1;
    }
    return GF{ exp: $exp, log: $log };
}

# gfMul multiplies two field elements.
func gfMul(gf as GF, a as int, b as int) {
    if ($a == 0 or $b == 0) {
        return 0;
    }
    return $gf.exp[($gf.log[$a] + $gf.log[$b]) % 255];
}

# rsGenerator builds the degree-`degree` Reed-Solomon generator polynomial
# (coefficients high-order first, length degree+1).
func rsGenerator(gf as GF, degree as int) {
    def g as list of int init [1];
    def i as int init 0;
    while ($i < $degree) {
        # multiply g by (x + exp[i])
        def next as list of int init [];
        def k as int init 0;
        while ($k <= len($g)) {
            $next[] = 0;
            $k = $k + 1;
        }
        def j as int init 0;
        while ($j < len($g)) {
            $next[$j] = $next[$j] ^ $g[$j];
            $next[$j + 1] = $next[$j + 1] ^ gfMul($gf, $g[$j], $gf.exp[$i]);
            $j = $j + 1;
        }
        $g = $next;
        $i = $i + 1;
    }
    return $g;
}

# rsEncode returns the `ecCount` Reed-Solomon error-correction codewords for a
# data codeword list.
func rsEncode(gf as GF, data as list of int, ecCount as int) {
    def gen as list of int init rsGenerator($gf, $ecCount);
    def res as list of int init [];
    for (def d in $data) {
        $res[] = $d;
    }
    def z as int init 0;
    while ($z < $ecCount) {
        $res[] = 0;
        $z = $z + 1;
    }
    def i as int init 0;
    while ($i < len($data)) {
        def coef as int init $res[$i];
        if ($coef > 0) {
            def j as int init 0;
            while ($j < len($gen)) {
                $res[$i + $j] = $res[$i + $j] ^ gfMul($gf, $gen[$j], $coef);
                $j = $j + 1;
            }
        }
        $i = $i + 1;
    }
    def ec as list of int init [];
    def m as int init len($data);
    while ($m < len($res)) {
        $ec[] = $res[$m];
        $m = $m + 1;
    }
    return $ec;
}
