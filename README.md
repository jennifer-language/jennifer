# Jennifer

Jennifer is a small, experimental, interpreted programming language. The
interpreter is written in Go and the shipping binary is produced with
[TinyGo](https://tinygo.org/). Source files use the `.j` extension.

This project exists primarily as a learning exercise: how to design a
language and build an interpreter end-to-end (lexer → preprocessor → parser →
tree-walking evaluator → stdlib).

Jennifer currently targets **Linux**; Windows and macOS support is planned.

## Quick start

```sh
# Build
tinygo build -o jennifer ./cmd/jennifer

# Run a program
./jennifer run examples/hello.j     # prints "42"
```

A first program:

```jennifer
import stdlib;

def app() {
    define $x as int init 21;
    printf($x + $x);
}
```

## Documentation

- [docs/user-guide.md](docs/user-guide.md) - language tutorial and reference
  for everything you can write in Jennifer today.
- [docs/technical.md](docs/technical.md) - interpreter internals: pipeline,
  token list, grammar (EBNF), AST nodes, evaluation model.
- [docs/milestones.md](docs/milestones.md) - what's implemented, what's
  coming, and the rationale behind the order.

## Testing

```sh
go test ./...
```

Development tests run under regular Go because TinyGo's test runner is
limited; the shipping binary is built with TinyGo.

## License

LGPL-3.0-only. See [LICENSE.md](LICENSE.md).
