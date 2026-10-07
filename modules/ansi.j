# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>
# pragma-jennifer-version: >=0.24.0

/**
 * Terminal styling as explicit string wrappers. The first module built on
 * Jennifer's module system: pure Jennifer, no Go. Colour is gated on stdout
 * being a terminal (with the NO_COLOR / FORCE_COLOR override), so wrapped
 * output stays clean when it is redirected to a file or a pipe.
 * @module ansi
 * @example
 * import "ansi.j" as ansi;
 * io.printf("%s\n", ansi.bold(ansi.red("error")));
 */
use os;
use maps;
use convert;
use regex;

# The ESC control byte (27) has no string-literal escape in Jennifer, so it is
# built from a one-byte `bytes`. One SGR sequence is ESC + "[" + code + "m".
def const ESC as string init makeEsc();

func makeEsc() {
    def b as bytes;
    $b[] = 27;
    return convert.stringFromBytes($b, "utf-8");
}

# SGR codes for foreground colour, background colour, and text style.
def const FG as map of string to string init {
    "black": "30",
    "red": "31",
    "green": "32",
    "yellow": "33",
    "blue": "34",
    "magenta": "35",
    "cyan": "36",
    "white": "37",
    "gray": "90",
    "grey": "90"
};
def const BG as map of string to string init {
    "black": "40",
    "red": "41",
    "green": "42",
    "yellow": "43",
    "blue": "44",
    "magenta": "45",
    "cyan": "46",
    "white": "47"
};
def const STYLE as map of string to string init {
    "bold": "1",
    "dim": "2",
    "italic": "3",
    "underline": "4",
    "reverse": "7",
    "strike": "9"
};

# enabled reports whether to emit escapes at all: NO_COLOR forces off,
# FORCE_COLOR forces on, otherwise gate on stdout being a terminal (and
# default on when the host cannot tell). Stateless - re-read every call, so
# there is no toggle to store (a module holds no mutable state).
func enabled() {
    if (len(os.getEnv("NO_COLOR")) > 0) {
        return false;
    }
    def fc as string init os.getEnv("FORCE_COLOR");
    if (len($fc) > 0) {
        # Interpret the value, don't just test presence: FORCE_COLOR=0 (and
        # "false") means force OFF, any other value forces ON.
        if ($fc == "0" or $fc == "false") {
            return false;
        }
        return true;
    }
    return os.isTerminal("stdout");
}

# wrap puts one SGR code around s, then resets - but only when colour is on.
func wrap(s as string, code as string) {
    if (not enabled()) {
        return $s;
    }
    return ESC + "[" + $code + "m" + $s + ESC + "[0m";
}

# lookup returns the SGR code for name in table, or throws on an unknown name.
func lookup(table as map of string to string, name as string, kind as string) {
    if (not maps.has($table, $name)) {
        throw Error{
            kind: "value",
            message: "unknown ansi " + $kind + ": " + $name,
            file: "",
            line: 0,
            col: 0
        };
    }
    return $table[$name];
}

/**
 * Wrap a string in the named foreground colour.
 * @param s {string} the text to colourize
 * @param name {string} the colour name (e.g. "red", "green", "cyan")
 * @return {string} the wrapped text, or s unchanged when colour is off
 * @throws {Error} when name is not a known colour
 */
export func color(s as string, name as string) {
    return wrap($s, lookup(FG, $name, "colour"));
}
/**
 * Wrap a string in the named background colour.
 * @param s {string} the text to colourize
 * @param name {string} the background colour name (e.g. "red", "blue")
 * @return {string} the wrapped text, or s unchanged when colour is off
 * @throws {Error} when name is not a known background colour
 */
export func bgColor(s as string, name as string) {
    return wrap($s, lookup(BG, $name, "background"));
}
/**
 * Wrap a string in the named text style.
 * @param s {string} the text to style
 * @param name {string} the style name (e.g. "bold", "italic", "underline")
 * @return {string} the wrapped text, or s unchanged when colour is off
 * @throws {Error} when name is not a known style
 */
export func style(s as string, name as string) {
    return wrap($s, lookup(STYLE, $name, "style"));
}
/**
 * Wrap a string in a 24-bit truecolor foreground.
 * @param s {string} the text to colourize
 * @param r {int} the red channel (0-255)
 * @param g {int} the green channel (0-255)
 * @param b {int} the blue channel (0-255)
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func rgb(s as string, r as int, g as int, b as int) {
    return wrap(
        $s,
        "38;2;" + convert.toString(clampChannel($r)) + ";" + convert.toString(clampChannel($g)) +
            ";" + convert.toString(clampChannel($b)));
}

# clampChannel bounds a colour channel to [0, 255] so an out-of-range argument
# can't emit a malformed SGR truecolor sequence.
func clampChannel(v as int) {
    if ($v < 0) {
        return 0;
    }
    if ($v > 255) {
        return 255;
    }
    return $v;
}

/**
 * Wrap a string in a 256-colour palette foreground (xterm `38;5;n`), the
 * indexed middle rung between the named colours and 24-bit rgb.
 * @param s {string} the text to colourize
 * @param n {int} the palette index (0-15 base, 16-231 cube, 232-255 grayscale)
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func color256(s as string, n as int) {
    return wrap($s, "38;5;" + convert.toString(clampChannel($n)));
}
/**
 * Wrap a string in a 256-colour palette background (xterm `48;5;n`).
 * @param s {string} the text to colourize
 * @param n {int} the palette index (0-255)
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func bgColor256(s as string, n as int) {
    return wrap($s, "48;5;" + convert.toString(clampChannel($n)));
}
/**
 * Map a 24-bit RGB triple to the nearest 256-colour index, for terminals
 * without truecolor. Picks the closer of the 6x6x6 colour cube (16-231) and
 * the 24-step grayscale ramp (232-255).
 * @param r {int} the red channel (0-255)
 * @param g {int} the green channel (0-255)
 * @param b {int} the blue channel (0-255)
 * @return {int} a palette index in [16, 255]
 */
export func rgbToColor256(r as int, g as int, b as int) {
    def rc as int init clampChannel($r);
    def gc as int init clampChannel($g);
    def bc as int init clampChannel($b);

    # Colour-cube candidate: quantize each channel to one of six levels.
    def ri as int init channelTo6($rc);
    def gi as int init channelTo6($gc);
    def bi as int init channelTo6($bc);
    def cubeIdx as int init 16 + 36 * $ri + 6 * $gi + $bi;
    def cubeDist as int init dist2($rc, $gc, $bc, cubeLevel($ri), cubeLevel($gi), cubeLevel($bi));

    # Grayscale candidate: ramp values 8, 18, ... 238 (step 10).
    def avg as int init ($rc + $gc + $bc) // 3;
    def step as int init ($avg - 8 + 5) // 10;
    if ($step < 0) {
        $step = 0;
    }
    if ($step > 23) {
        $step = 23;
    }
    def grayVal as int init 8 + 10 * $step;
    def grayDist as int init dist2($rc, $gc, $bc, $grayVal, $grayVal, $grayVal);

    if ($grayDist < $cubeDist) {
        return 232 + $step;
    }
    return $cubeIdx;
}

# channelTo6 quantizes a 0-255 channel to the nearest of the six xterm cube
# levels (0, 95, 135, 175, 215, 255), returning its index 0..5.
func channelTo6(v as int) {
    if ($v < 48) {
        return 0;
    }
    if ($v < 115) {
        return 1;
    }
    return ($v - 35) // 40;
}

# cubeLevel is the channel value for a cube level index 0..5.
func cubeLevel(i as int) {
    if ($i <= 0) {
        return 0;
    }
    if ($i == 1) {
        return 95;
    }
    if ($i == 2) {
        return 135;
    }
    if ($i == 3) {
        return 175;
    }
    if ($i == 4) {
        return 215;
    }
    return 255;
}

# dist2 is the squared Euclidean distance between two RGB points.
func dist2(r1 as int, g1 as int, b1 as int, r2 as int, g2 as int, b2 as int) {
    def dr as int init $r1 - $r2;
    def dg as int init $g1 - $g2;
    def db as int init $b1 - $b2;
    return $dr * $dr + $dg * $dg + $db * $db;
}

/**
 * Remove every SGR escape - the inverse of the wrappers, regardless of whether
 * colour is currently enabled.
 * @param s {string} the text to strip
 * @return {string} the text with all SGR escapes removed
 */
export func strip(s as string) {
    return regex.replace(ESC + "\\[[0-9;]*m", $s, "");
}

/**
 * Wrap a string in black foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func black(s as string) {
    return color($s, "black");
}
/**
 * Wrap a string in red foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func red(s as string) {
    return color($s, "red");
}
/**
 * Wrap a string in green foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func green(s as string) {
    return color($s, "green");
}
/**
 * Wrap a string in yellow foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func yellow(s as string) {
    return color($s, "yellow");
}
/**
 * Wrap a string in blue foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func blue(s as string) {
    return color($s, "blue");
}
/**
 * Wrap a string in magenta foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func magenta(s as string) {
    return color($s, "magenta");
}
/**
 * Wrap a string in cyan foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func cyan(s as string) {
    return color($s, "cyan");
}
/**
 * Wrap a string in white foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func white(s as string) {
    return color($s, "white");
}
/**
 * Wrap a string in gray foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func gray(s as string) {
    return color($s, "gray");
}
/**
 * Wrap a string in the bold text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func bold(s as string) {
    return style($s, "bold");
}
/**
 * Wrap a string in the dim text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func dim(s as string) {
    return style($s, "dim");
}
/**
 * Wrap a string in the italic text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func italic(s as string) {
    return style($s, "italic");
}
/**
 * Wrap a string in the underline text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func underline(s as string) {
    return style($s, "underline");
}
/**
 * Wrap a string in the reverse (inverted) text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func reverse(s as string) {
    return style($s, "reverse");
}
/**
 * Wrap a string in the strikethrough (crossed-out) text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func strike(s as string) {
    return style($s, "strike");
}
