// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lexer"
	metalib "jennifer-lang.dev/jennifer/internal/lib/meta"
	"jennifer-lang.dev/jennifer/internal/lib/os"
	termlib "jennifer-lang.dev/jennifer/internal/lib/term"
	"jennifer-lang.dev/jennifer/internal/module"
	"jennifer-lang.dev/jennifer/internal/parser"
	"jennifer-lang.dev/jennifer/internal/preproc"
	"jennifer-lang.dev/jennifer/internal/stdlib"
	"jennifer-lang.dev/jennifer/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	// Baseline system module directory (env / compile-time default),
	// unvalidated, so meta.SYSMODDIR has a value for every subcommand. The
	// program-running paths (run / repl) re-resolve with any --sysmoddir
	// flag and validate.
	metalib.SetSysmoddir(module.ResolveSysmoddir("", os.Getenv).Dir)

	switch os.Args[1] {
	case "run":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		// Interpreter flags (--sysmoddir, -I) precede the source file;
		// everything after the file is the user program's argv. Convention
		// (matches Python sys.argv, Go os.Args): index 0 of the
		// user-visible `os.ARGS` is the script path, the rest are the
		// user-supplied args.
		file, sysmoddirFlag, vendorFlag, includes, userArgs, perr := parseRunArgs(os.Args[2:])
		if perr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", perr)
			usage()
			os.Exit(2)
		}
		sm, err := setupSysmoddir(sysmoddirFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			os.Exit(2)
		}
		// Module search path: system module dir first, then each -I dir.
		searchDirs := append([]string{sm.Dir}, includes...)
		oslib.SetUserArgs(append([]string{file}, userArgs...))
		os.Exit(runFile(file, searchDirs, vendorFlag))
	case "repl":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		sm, err := setupSysmoddir("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			os.Exit(2)
		}
		// The REPL resolves bare-name module imports through the system module
		// dir (local `./` imports resolve against the cwd, set in runRepl).
		os.Exit(runRepl([]string{sm.Dir}))
	case "tokens":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(dumpTokens(os.Args[2]))
	case "ast":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(dumpAST(os.Args[2]))
	case "fmt":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(runFmt(os.Args[2]))
	case "lint":
		os.Exit(runLint(os.Args[2:]))
	case "profile":
		os.Exit(runProfile(os.Args[2:]))
	case "test":
		os.Exit(runTest(os.Args[2:]))
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "version", "--version", "-v":
		if len(os.Args) > 2 && isVerboseFlag(os.Args[2]) {
			printVersionVerbose()
		} else {
			fmt.Println(version.Version)
		}
		os.Exit(0)
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "jennifer: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

// License info displayed by `jennifer help` and on usage errors.
// Keep in sync with the SPDX headers at the top of each source file.
const (
	licenseID   = "LGPL-3.0-only"
	copyright   = "Copyright (C) 2026 mplx <jennifer@mplx.dev>"
	description = "jennifer - Jennifer programming language interpreter"
)

func usage() {
	fmt.Fprintln(os.Stderr, description)
	fmt.Fprintln(os.Stderr, copyright)
	fmt.Fprintln(os.Stderr, "License: "+licenseID)
	fmt.Fprintln(os.Stderr, "Version: "+version.Version)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  jennifer run <file.j>    run a Jennifer program")
	fmt.Fprintln(os.Stderr, "  jennifer run -           read source from stdin")
	fmt.Fprintln(os.Stderr, "  jennifer repl            interactive REPL")
	// The development subcommands are build-tag split: the default binary
	// lists them; the constrained TinyGo build (which omits them) prints a
	// pointer at the default binary instead.
	printDevUsage(os.Stderr)
	fmt.Fprintln(os.Stderr, "  jennifer version         print the version and exit")
	fmt.Fprintln(os.Stderr, "  jennifer help            show this message")
}

// parseRunArgs splits `run`'s arguments into the interpreter flags
// (--sysmoddir, and -I which extends the module search path), the source
// file, and the user program's argv. Flags precede the file; the first
// non-flag token (or `-` for stdin) is the file, and everything after it
// is passed through to the program untouched.
func parseRunArgs(args []string) (file, sysmoddir, vendor string, includes, userArgs []string, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "-" || !strings.HasPrefix(a, "-") {
			return a, sysmoddir, vendor, includes, args[i+1:], nil
		}
		switch {
		case strings.HasPrefix(a, "--sysmoddir="):
			sysmoddir = strings.TrimPrefix(a, "--sysmoddir=")
			i++
		case a == "--sysmoddir":
			if i+1 >= len(args) {
				return "", "", "", nil, nil, fmt.Errorf("jennifer run: --sysmoddir needs a directory argument")
			}
			sysmoddir, i = args[i+1], i+2
		case strings.HasPrefix(a, "--vendor="):
			vendor = strings.TrimPrefix(a, "--vendor=")
			i++
		case a == "--vendor":
			if i+1 >= len(args) {
				return "", "", "", nil, nil, fmt.Errorf("jennifer run: --vendor needs a directory argument")
			}
			vendor, i = args[i+1], i+2
		case strings.HasPrefix(a, "-I="):
			includes = append(includes, strings.TrimPrefix(a, "-I="))
			i++
		case a == "-I":
			if i+1 >= len(args) {
				return "", "", "", nil, nil, fmt.Errorf("jennifer run: -I needs a directory argument")
			}
			includes, i = append(includes, args[i+1]), i+2
		default:
			return "", "", "", nil, nil, fmt.Errorf("jennifer run: unknown flag %q", a)
		}
	}
	return "", "", "", nil, nil, fmt.Errorf("jennifer run: no source file given")
}

// setupSysmoddir resolves the system module directory from the layer
// precedence (--sysmoddir, then JENNIFER_SYSMODDIR, then the compile-time
// default), validates an explicitly-named one (a missing or non-directory
// CLI/env value refuses to start), and records the winner for
// meta.SYSMODDIR.
func setupSysmoddir(cliFlag string) (module.Sysmoddir, error) {
	sm := module.ResolveSysmoddir(cliFlag, os.Getenv)
	if err := sm.Validate(os.Stat); err != nil {
		return sm, err
	}
	metalib.SetSysmoddir(sm.Dir)
	return sm, nil
}

// printVersionVerbose backs `jennifer version -v`: the build version plus every
// system directory the resolver uses (the system module dir and the vendor
// root for `@scope/package` decks), each with the layers behind it.
func printVersionVerbose() {
	fmt.Println("jennifer " + version.Version)

	sm := module.ResolveSysmoddir("", os.Getenv)
	fmt.Printf("system module dir: %s (%s)\n", sm.Dir, sm.Source)
	fmt.Printf("  compile default: %s\n", module.CompileDefaultSysmoddir())
	if env := os.Getenv(module.SysmoddirEnv); env != "" {
		fmt.Printf("  env %s: %s\n", module.SysmoddirEnv, env)
	}

	// Vendor root for `@scope/package` deck imports. Resolved relative to the
	// current directory (the upward `vendor/` walk), since there is no program
	// context here.
	cwd, _ := os.Getwd()
	if vr := module.FindVendorRoot("", cwd); vr != "" {
		src := "nearest vendor/ above cwd"
		if os.Getenv(module.VendorEnv) != "" {
			src = "env " + module.VendorEnv
		}
		fmt.Printf("vendor root: %s (%s)\n", vr, src)
	} else {
		fmt.Printf("vendor root: (none; no %s and no vendor/ above cwd)\n", module.VendorEnv)
	}
	if env := os.Getenv(module.VendorEnv); env != "" {
		fmt.Printf("  env %s: %s\n", module.VendorEnv, env)
	}
}

// isVerboseFlag reports whether a `version` argument requests verbose output:
// --verbose, or -v repeated (-v, -vv, -vvv, ...). Verbosity has a single level;
// the repeats are accepted so `jennifer version -vvv` shows the verbose output
// rather than silently downgrading to the plain version line.
func isVerboseFlag(s string) bool {
	if s == "--verbose" {
		return true
	}
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	for _, c := range s[1:] {
		if c != 'v' {
			return false
		}
	}
	return true
}

// loadModuleProgram is the interpreter's module loader: it reads, lexes,
// preprocesses, and parses a resolved module file into a program the loader
// then resolves and runs. Errors carry the module file's position.
func loadModuleProgram(path string) (*parser.Program, error) {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read module %q: %v", path, err)
	}
	toks, err := lexer.TokenizeWithFile(string(srcBytes), path)
	if err != nil {
		return nil, err
	}
	toks, err = preproc.Process(toks, filepath.Dir(path), path)
	if err != nil {
		return nil, err
	}
	return parser.ParseTokens(toks)
}

func runFile(path string, searchDirs []string, vendorFlag string) int {
	return runFileHook(path, searchDirs, vendorFlag, nil)
}

// runFileHook is runFile with an optional callback invoked once the source has
// parsed cleanly, just before execution begins. `jennifer serve` uses it to
// print its "running" banner only after a clean parse, so the banner never
// precedes a syntax error.
func runFileHook(path string, searchDirs []string, vendorFlag string, afterParse func()) int {
	var (
		src     string
		label   string // path used in error messages
		absPath string // file tag for tokens (preproc cycle check)
		baseDir string // base for relative file imports
	)
	if path == "-" {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: reading stdin: %v\n", err)
			return 1
		}
		src = string(bytes)
		label = "<stdin>"
		absPath = "<stdin>"
		// File imports from stdin resolve relative to the current working
		// directory - there's no source-file location to anchor against.
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			return 1
		}
		baseDir = cwd
	} else {
		if filepath.Ext(path) != ".j" {
			fmt.Fprintf(os.Stderr, "jennifer: source file must have .j extension, got %q\n", path)
			return 2
		}
		srcBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jennifer: %v\n", err)
			return 1
		}
		src = string(srcBytes)
		label = path
		abs, _ := filepath.Abs(path)
		absPath = abs
		baseDir = filepath.Dir(abs)
	}

	tokens, err := lexer.TokenizeWithFile(src, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	tokens, err = preproc.Process(tokens, baseDir, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	prog, err := parser.ParseTokens(tokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, err.Error())
		printErrorContext(src, absPath, err)
		return 1
	}
	if afterParse != nil {
		afterParse()
	}
	// A `.j` script can put the terminal in raw mode via `term.makeRaw`. If it
	// aborts before `term.restore` - an uncaught error, `exit`, or a panic - the
	// shell would be left wedged (no echo, no line editing). Jennifer has no
	// `finally`, but the CLI has Go's defer: restore any raw-mode terminal on the
	// way out.
	defer termlib.RestoreAll()
	// The defer above does not run when a terminating signal (SIGINT / SIGTERM /
	// SIGHUP) takes its default disposition, since that kills the process without
	// unwinding. installTermSignalRestore traps those: an *uncaught* one restores
	// the terminal and then re-raises to die as normal; a signal the script caught
	// via os.catchSignal is left to the script. SIGKILL remains uncoverable.
	stopTermRestore := installTermSignalRestore()
	defer stopTermRestore()
	in := interpreter.New()
	installLibraries(in)
	// Enable `import "..."` module resolution: local imports resolve
	// relative to this file's directory; bare names walk searchDirs (the
	// system module dir, then any -I dirs). Each module loads into a fresh
	// sub-interpreter that installLibraries populates.
	in.EnableModules(baseDir, searchDirs, loadModuleProgram, installLibraries)
	// `@scope/package` deck imports resolve under the vendor root: --vendor, else
	// JENNIFER_VENDOR, else the nearest `vendor/` above this file.
	in.SetVendorRoot(module.FindVendorRoot(vendorFlag, baseDir))
	// SIGUSR1 -> a live diagnostics snapshot from the interpreter (where is it
	// executing?), printed and then execution continues. No-op on non-Unix.
	stopDiag := installDiagSignal(in)
	defer stopDiag()
	runErr := in.Run(prog)

	// The exit-time loud-fail. Even when Run returned cleanly,
	// any spawned task that ended with an error and was never
	// task.wait'd / task.discard'd has its error printed to stderr
	// and bumps the exit code. Tasks that are still in flight at exit
	// have not been observed either, so the scan blocks on each
	// until it finishes (the goroutine has nowhere to go if we don't
	// drain it; the alternative is silently dropping its outcome).
	unwaited := in.UnwaitedTaskErrors()
	var unwaitedExit *interpreter.ExitSignal
	for _, e := range unwaited {
		if ex, ok := e.(*interpreter.ExitSignal); ok {
			// An unwaited spawn invoked `exit EXPR;` - that exit code
			// becomes the program's exit code, dominating any other
			// unwaited error.
			if unwaitedExit == nil {
				unwaitedExit = ex
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: unwaited spawn error: %s\n", label, e.Error())
		printErrorContext(src, absPath, e)
	}

	if runErr != nil {
		// `exit;` / `exit EXPR;` - user-requested clean
		// termination from the main flow. Propagate the requested exit
		// code without printing a runtime error trace.
		if ex, ok := runErr.(*interpreter.ExitSignal); ok {
			return ex.Code
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, runErr.Error())
		printErrorContext(src, absPath, runErr)
		return 1
	}
	if unwaitedExit != nil {
		return unwaitedExit.Code
	}
	if len(unwaited) > 0 {
		return 1
	}
	return 0
}

// installLibraries activates every standard library on a fresh interpreter.
// Shared by `run` and `profile` so the two never drift on which libraries a
// program can `use`.
func installLibraries(in *interpreter.Interpreter) {
	stdlib.InstallAll(in)
}

// positioned is the interface every Jennifer error type implements. It lets
// the CLI extract structured position info (file, line, col) without parsing
// the error's string form.
type positioned interface {
	Position() (file string, line, col int)
}

// printErrorContext shows the offending source line with a caret when the error
// carries a position. If the error originates in a different file than `src`
// (e.g. inside an imported `.j`), it loads that file and prints the snippet
// from there. Best-effort: silently does nothing if the file can't be read.
// Writes to os.Stderr; for callers that need a different destination (e.g.
// the REPL's CRLF-translating wrapper) use printErrorContextTo.
func printErrorContext(src, mainFile string, err error) {
	printErrorContextTo(os.Stderr, src, mainFile, err)
}

// printErrorContextTo is the writer-parametric form. The REPL uses it to
// route caret output through a crlfWriter when stdin is in raw mode.
func printErrorContextTo(w io.Writer, src, mainFile string, err error) {
	p, ok := err.(positioned)
	if !ok {
		return
	}
	file, line, col := p.Position()
	if line == 0 && col == 0 {
		return
	}

	// If the error names a different file, load it. Otherwise use src.
	source := src
	if file != "" && file != mainFile && file != "<stdin>" {
		bytes, ferr := os.ReadFile(file)
		if ferr != nil {
			// Can't load the imported file - skip the snippet.
			return
		}
		source = string(bytes)
	}

	lines := strings.Split(source, "\n")
	if line < 1 || line > len(lines) {
		return
	}
	srcLine := lines[line-1]
	fmt.Fprintf(w, "  %s\n", srcLine)
	if col > 0 {
		fmt.Fprintf(w, "  %s^\n", caretIndent(srcLine, col))
	}
}

// caretIndent builds the indent string that places a caret under column `col`
// (1-based, rune-counted - matching the lexer). Tabs in the source are
// replicated as tabs in the indent so the caret aligns regardless of the
// terminal's tab-stop width; other characters become single spaces.
func caretIndent(srcLine string, col int) string {
	var b strings.Builder
	i := 0
	for _, r := range srcLine {
		if i >= col-1 {
			break
		}
		if r == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
		i++
	}
	return b.String()
}
