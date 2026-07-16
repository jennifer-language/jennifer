# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>

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
    "black": "30", "red": "31", "green": "32", "yellow": "33",
    "blue": "34", "magenta": "35", "cyan": "36", "white": "37",
    "gray": "90", "grey": "90"
};
def const BG as map of string to string init {
    "black": "40", "red": "41", "green": "42", "yellow": "43",
    "blue": "44", "magenta": "45", "cyan": "46", "white": "47"
};
def const STYLE as map of string to string init {
    "bold": "1", "dim": "2", "italic": "3", "underline": "4", "reverse": "7"
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
        throw Error{kind: "value", message: "unknown ansi " + $kind + ": " + $name, file: "", line: 0, col: 0};
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
    return wrap($s, "38;2;" + convert.toString($r) + ";" + convert.toString($g) + ";" + convert.toString($b));
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
export func black(s as string) { return color($s, "black"); }
/**
 * Wrap a string in red foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func red(s as string) { return color($s, "red"); }
/**
 * Wrap a string in green foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func green(s as string) { return color($s, "green"); }
/**
 * Wrap a string in yellow foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func yellow(s as string) { return color($s, "yellow"); }
/**
 * Wrap a string in blue foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func blue(s as string) { return color($s, "blue"); }
/**
 * Wrap a string in magenta foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func magenta(s as string) { return color($s, "magenta"); }
/**
 * Wrap a string in cyan foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func cyan(s as string) { return color($s, "cyan"); }
/**
 * Wrap a string in white foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func white(s as string) { return color($s, "white"); }
/**
 * Wrap a string in gray foreground colour.
 * @param s {string} the text to colourize
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func gray(s as string) { return color($s, "gray"); }
/**
 * Wrap a string in the bold text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func bold(s as string) { return style($s, "bold"); }
/**
 * Wrap a string in the dim text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func dim(s as string) { return style($s, "dim"); }
/**
 * Wrap a string in the italic text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func italic(s as string) { return style($s, "italic"); }
/**
 * Wrap a string in the underline text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func underline(s as string) { return style($s, "underline"); }
/**
 * Wrap a string in the reverse (inverted) text style.
 * @param s {string} the text to style
 * @return {string} the wrapped text, or s unchanged when colour is off
 */
export func reverse(s as string) { return style($s, "reverse"); }
