# `dot` - Graphviz DOT graph description

Import with `import "dot.j" as dot;`. Builds a graph of nodes and edges with
attributes and renders it to `.dot` text for an external Graphviz tool to lay
out and draw. Pure Jennifer over `strings` and `lists`, so it runs on either
binary. It emits the text description only; graph **layout** is left to
Graphviz. Every builder returns a fresh `Graph`; the graph you pass in is never
mutated.

```jennifer
use io;
import "dot.j" as dot;

def g as dot.Graph init dot.digraph("pipeline");
$g = dot.graphAttr($g, "rankdir", "LR");
$g = dot.nodeAttr($g, "shape", "box");
$g = dot.nodeWith($g, "lexer", {"label": "Lexer"});
$g = dot.edgeWith($g, "lexer", "parser", {"label": "tokens"});
io.printf("%s", dot.render($g));
```

Pipe the rendered text to Graphviz to produce an image:

```sh
jennifer run graph.j | dot -Tsvg > graph.svg
```

Runnable: [`examples/modules/dot_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/dot_demo.j).

## Surface

| Call                                | Returns | Notes                                                                              |
| ----------------------------------- | ------- | ---------------------------------------------------------------------------------- |
| `dot.digraph(name)`                 | `Graph` | A new directed graph: renders `digraph`, edges `a -> b`.                           |
| `dot.graph(name)`                   | `Graph` | A new undirected graph: renders `graph`, edges `a -- b`.                           |
| `dot.node(g, id)`                   | `Graph` | Add a bare node (optional: an id first seen on an edge is created implicitly).     |
| `dot.nodeWith(g, id, attrs)`        | `Graph` | Add a node with a `map of string to string` of DOT attributes (`{"shape": "box"}`).|
| `dot.edge(g, src, dst)`             | `Graph` | Add a bare edge between two node ids.                                              |
| `dot.edgeWith(g, src, dst, attrs)`  | `Graph` | Add an edge with its own attributes (`{"label": "calls"}`).                        |
| `dot.graphAttr(g, key, value)`      | `Graph` | Set a graph-level attribute (`rankdir` = LR, a graph `label`, ...).               |
| `dot.nodeAttr(g, key, value)`       | `Graph` | Set a default node attribute, rendered once as `node [ ... ]`.                     |
| `dot.edgeAttr(g, key, value)`       | `Graph` | Set a default edge attribute, rendered once as `edge [ ... ]`.                     |
| `dot.render(g)`                     | `string`| Render the graph to DOT text (with a trailing newline).                           |

Each builder call rebuilds the `Graph` it is given (a value holding the
module's structs is rebuilt at every module call), so thousands of `dot.edge`
calls in a loop are quadratic (2000 edges take seconds). For a large graph
append to the fields directly, which needs no module call:

```jennifer
use io;
import "dot.j" as dot;

def g as dot.Graph init dot.digraph("big");
for (def i in 0..2000) {
    $g.edges[] = dot.Edge{from: "n{$i}", to: "n{$i + 1}", attrs: {}};
}
$g.nodes[] = dot.Node{id: "n0", attrs: {"shape": "box"}};
def text as string init dot.render($g);
io.printf("%d edges, %d characters\n", len($g.edges), len($text));   # 2000 edges, 41823 characters
```

Attribute maps keep insertion order, so the rendered attribute list is stable.
String values (labels, the graph name, attribute values) are DOT-escaped: a
backslash and a double-quote are backslash-escaped (so Graphviz's own `\N` /
`\l` label escapes cannot be written), and a newline or CR becomes the
two-character `\n` DOT escape. DOT has no escape for the other control
characters (a NUL ends a Graphviz string), so `render` throws
`Error{kind: "dot"}` for an id, graph name or value holding one; a tab is kept
as it is. Attribute names are written unquoted, so a name that is not a DOT
identifier (`[A-Za-z_][A-Za-z0-9_]*`) is an `Error{kind: "dot"}`, from the
setter, or from `render` for a map passed to `nodeWith` / `edgeWith`.

## Types

| Struct | Fields |
| ------ | ------ |
| `Graph` | `directed as bool` (`digraph` vs `graph`), `name as string`, `nodes as list of Node`, `edges as list of Edge`, `graphAttrs` / `nodeAttrs` / `edgeAttrs as map of string to string` |
| `Node` | `id as string`, `attrs as map of string to string` |
| `Edge` | `from as string`, `to as string`, `attrs as map of string to string` |

The fields are public for inspection, but the builders (`digraph` / `graph` /
`node*` / `edge*` / `*Attr`) are the intended construction path.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Numbers and data: decimal, stats, linalg, ml, plot and dot](https://academy.jennifer-lang.dev/talks/mplx/numbers/) | a Graphviz graph built from data |

## API reference

Every exported function, struct and constant of `dot`, with its signature, generated
from the module's doc comments: [the `dot` API reference](../api/dot.md).
