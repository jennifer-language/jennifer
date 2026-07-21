# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
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
    # Bind the log/exp tables locally once. Passing the whole GF struct into a
    # gfMul helper per multiplication would value-copy both ~255-entry lists
    # every call (thousands per polynomial); indexing the local lists is free.
    def exp as list of int init $gf.exp;
    def log as list of int init $gf.log;
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
            # inline gfMul(g[j], exp[i]); exp[i] is never zero (log[exp[i]] == i).
            def prod as int init 0;
            if (not ($g[$j] == 0)) {
                $prod = $exp[($log[$g[$j]] + $i) % 255];
            }
            $next[$j + 1] = $next[$j + 1] ^ $prod;
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
    def exp as list of int init $gf.exp;
    def log as list of int init $gf.log;
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
            def logCoef as int init $log[$coef];
            def j as int init 0;
            while ($j < len($gen)) {
                # inline gfMul(gen[j], coef); coef > 0 here, so only guard gen[j].
                def prod as int init 0;
                if (not ($gen[$j] == 0)) {
                    $prod = $exp[($log[$gen[$j]] + $logCoef) % 255];
                }
                $res[$i + $j] = $res[$i + $j] ^ $prod;
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
