# Interpreter (`internal/interpreter`)

A tree-walking evaluator.

## Runtime values

`Value` is a tagged union (single concrete struct) rather than a Go interface
hierarchy. This avoids `reflect` and method-table indirection, which keeps the
binary small and predictable under TinyGo.

```go
type Value struct {
    Kind    ValueKind  # KindNull | KindInt | KindFloat | KindString |
                       #  KindBool | KindBytes | KindList | KindMap |
                       #  KindStruct | KindTask | KindObject | KindEnum |
                       #  KindFunc | KindChannel
    Int     int64
    Float   float64
    Str     string
    Bool    bool
    List    []Value      # KindList:   element data
    Map     []MapEntry   # KindMap:    insertion-ordered entries
    Bytes   []byte       # KindBytes:  raw bytes
    Fields  []StructField # KindStruct / KindEnum: ordered (Name, Value)
    Variant string       # KindEnum: the active variant
    StructName string    # KindStruct / KindEnum: the type name
    StructNS, ModPath string # library / module type identity
    ElemTyp *parser.Type # KindList: declared element type (stamped)
    KeyTyp  *parser.Type # KindMap:  declared key type   (stamped)
    ValTyp  *parser.Type # KindMap:  declared value type (stamped)
    Task    *TaskState   # KindTask:    shared handle
    Obj     *ObjectBox   # KindObject:  opaque library payload
    Fn      *parser.MethodDef # KindFunc: the function (+ FnHome)
    Chan    *ChannelState # KindChannel: shared handle
    # plus unexported caches (strRunes, mapIdx)
}
```

`ZeroFor(t parser.Type)` returns the zero value for each declared type and is
used when a `def` omits its `init` clause: `0`, `0.0`, `""`, `false`,
`null`, an empty `[]` list (typed by the declaration), or an empty `{}` map.
`Value.AsFloat()` handles int->float promotion for arithmetic and
comparison; `Value.Equal()` implements `==` (same-kind comparison, plus
an exact `int` / `float` comparison that never promotes the `int` to a
lossy `float`; deep-equal for
lists; order-insensitive key→value equality for maps).

### Parameterized Type

`parser.Type` is a recursive struct: `{ Kind TypeKind; Element,
KeyType, ValType *Type }`. Compound types nest
(`list of list of int`). Equality (`Type.Equal`) is structural.

### Value semantics

Lists and maps are **value-typed** in Jennifer: `$ys = $xs;` behaves
as a copy, function parameters bind by copy, and `const` is deep.
No aliasing is observable from user code.

**Eager-copy value semantics.** Value semantics rest entirely on
eager deep copies at every store site, not on copy-on-write:

- Every binding site takes a private copy before storing:
  `execDefine` / `execAssign` via `eagerCopy`, parameter binding via
  `bindParamValue`, and `snapshotForSpawn` via `DeepCopy` for the
  goroutine-boundary crossing. Library builtins whose pattern is
  "copy then mutate freely" (`lists.shuffle`, `lists.reverse`, ...)
  `Copy()` first. So no two live bindings ever share a compound
  backing.
- Because of that invariant the mutation sites (`execAppend`,
  `execIndexAssign`, `execFieldAssign`) fetch the target via
  `GetBinding` and mutate the binding's **own** backing in place -
  no per-write copy - which keeps append-in-a-loop amortised O(N).
- `Value.Copy()` is the public deep-copy alias; the engine is
  `DeepCopy()`.
- One optimisation trims a redundant copy: a fresh list / map /
  struct literal RHS is already private (its evaluator `Copy()`s
  every element into a brand-new container), so `execDefine` /
  `execAssign` skip the whole-value eager copy for it
  (`rhsFreshLiteral`) and only stamp the declared type. Var / index
  / field reads, const refs, and calls can hand back a reference
  into a live binding, so those are still eager-copied.
- The same reasoning applies one level in: a list element / map value
  / struct or enum **field** whose source is itself a fresh literal is
  already private, so the literal evaluator stores it without a second
  copy (`literalElementValue`). This stops a nested literal from being
  deep-copied once by its own evaluator and again by the enclosing one.
- `literalElementValue` also performs **move-on-last-use**: for
  `$v = <fresh literal embedding $v>` the resolver marks the single
  embedded `$v` reference `VarExpr.Move` (`markMoveableAssign`), and the
  evaluator stores its backing by move instead of copying - sound because
  the assignment overwrites `$v` immediately after, so the new structure
  solely owns the backing (a reassigned name is never a borrowed binding,
  and the eager-copy model gives every other binding its own backing).
  Exactly one occurrence is moved; any other is copied, so the result
  never references the moved backing twice. This turns incremental
  left-leaning construction (`$acc = Node{ parts: [$acc, ...] }`, a
  parser AST or expression tree) from quadratic into linear.

**Ownership: owned call results and moved arguments.** Two more elisions,
computed by `markOwnership` (`internal/parser/ownership.go`) at the end of
`Resolve`, make a mutate-and-return helper - `$grid = put($grid, x, v);`,
the shape every value-semantic "update" API takes - O(1) per call instead of
O(size of the value). Both rest on the same invariant as above: a function's
locals solely own their values, except a borrowed parameter / `def` (an
alias), a for-each iterator, a catch name, a match binder (left out to stay
conservative, as are constants). `ownedLocals` computes that set per function,
by name (no shadowing makes a name unambiguous within a function; a name bound
more than one way in sibling blocks is not owned).

- **`MethodDef.ReturnsOwned`** - every `return` of the function yields a value
  nothing else references once the frame exits: a scalar, a fresh literal, an
  owned local or a field / element of one, or the result of another
  `ReturnsOwned` function. A greatest fixpoint over the program's functions (start
  all true, clear on a non-owned return, repeat) handles recursion soundly: any
  value actually returned comes from a finite chain ending in a return owned on
  its own terms. `execDefine` / `execAssign` then store such a call's result
  without the eager copy (`ownedCallResult`), like a fresh literal. A module
  function's flag is read through the stamped `moduleMethodTarget`.
- **`CallExpr.MoveArg` / `QualifiedCallExpr.MoveArg`** - in
  `$v = f(..., $v, ...)`, where `$v` is an owned local named exactly once in
  the right-hand side (as a direct argument), the old value is dead on every
  path out of the statement: overwritten when the call returns, unreachable
  when it throws because the error unwinds the frame. The parameter binds it
  without the copy (`evalCall`, and `callMethodMoving` for a module call), so a
  callee that mutates its parameter does so in place. The one path that could
  still observe `$v` after a throw is a `try` in the **same function**, so a call
  inside a `try` body is never marked; a global (reachable from the callee and
  from a catch anywhere up the stack) is never in the owned set, which also
  rules out all top-level code. Callers other than this exact shape pay one
  copy, at the bind.

Crossing a module boundary, `findImpostorStruct` and the retag precheck
(`subtreeNeedsRetag`) skip the elements of a list / map whose element types are
primitive (`primitiveType`) - a typed list can hold no struct - so a module call
carrying a large `list of string` stays O(1) as well. Pinned by
`internal/parser/ownership_test.go` (which calls are marked, which functions own
their results) and `internal/interpreter/ownership_test.go` (value semantics on
every elided and every non-elided path, across a module boundary too).

**Read-only-parameter borrow.** The one carve-out to "parameter
binding always copies": when a function never writes a parameter, its
argument is bound by **alias** rather than deep-copied, since a
read-only alias is observationally identical to a copy. `Resolve`
runs `markBorrowableParams` per function - a statement-level write-scan
of the body (descending through nested control flow; because no
shadowing is allowed, a written name is unambiguously the parameter,
and `spawn` bodies mutate their own deep-copied snapshot so are
skipped) - and sets `Param.Borrow` when the parameter is
never written **and** its type is *borrow-safe*: a compound whose
`stampDeclaredType` does not recurse into element backing (a flat
`list` / `map`, or a struct / `bytes`; a `list of list` falls back to
copying so the stamp cannot mutate the shared backing).
`Interpreter.bindArg` aliases when
`Param.Borrow && (i.entryGlobalsImmutable || (m.GlobalSafe && !spawnReentry))`
and copies (`bindParamValue`) otherwise, per callee function `m`
(`methodBorrowCtx`). The gate is the soundness boundary: "never writes
the parameter" alone is not enough, because if the argument aliases a
mutable global and the body mutates that global, a borrowed read would
see the change where a copy would not. So borrow is enabled only where
no mutable global can be aliased and mutated during the call, which
holds in two cases (a module is **not** blanket-safe: it holds no mutable
global of its own, but a function reaching `meta.callMain` can mutate a
*host* global its borrowed argument aliases, so modules run
`computeEntryGlobalSafe` too and rely on `m.GlobalSafe`):

- a **single-file script that declares no mutable top-level `def`**
  (`hasMutableTopLevelGlobal` false, recorded as `entryGlobalsImmutable`
  in `Run`), so a script's own read-only helpers benefit without being
  moved into a module;
- a specific **function proven to mutate no global transitively**
  (`m.GlobalSafe`), even in a script that holds mutable globals
  elsewhere. `computeEntryGlobalSafe` (run after module load) scans each
  entry function for a local hazard - a write to a non-local name, a
  `CallValueExpr`, a module call (which can re-enter the host via
  `meta.callMain`), or a callback builtin that can invoke `.j` code (the
  higher-order `lists` layer, `meta.call` / `meta.callMain`,
  `testing.run` / `runWith` / `assertThrows` / `assertThrowsWith`, keyed by canonical
  namespace) - then a fixpoint propagates the hazard along named-call
  edges (a worklist over reverse call edges, linear in the call graph). The
  walker defaults an unrecognised node to hazardous, so a new node never
  silently escapes. A `spawn` body's global writes land in its snapshot and
  are ignored, but a spawn body that can **re-enter** the host (a module
  call, a callback builtin, a func-value or unresolved call, or a call to a
  function that transitively does one) writes the live host globals at any
  time, from outside the call chain the analysis follows; when any spawn in
  the run (in a function or the top-level code, entry program or module) can
  re-enter, the host's `spawnReentry` flag turns the `GlobalSafe` widening
  off for the whole run. The resolver leaves spawn-body calls without a
  `Method`, so the scan looks a spawn-body callee up by name in the method
  table (as `computeSpawnCaptures` does); only a name that is not a user
  function counts as unresolved, so an ordinary worker spawn keeps borrow on.
  The analysis runs for every program with mutable
  globals, not only when some parameter is borrowable, because a borrowable
  local `def` depends on it too. The callback set must be a denylist because a func-value
  argument is not statically visible (a bare function name is a
  `ConstRefExpr`, and curried func returns carry no type) - the invariant
  "any builtin invoking `.j` code via `BuiltinCtx.Invoke` / `CallByName*`
  / `CallHostWith*` is listed" lives in `globalsafe.go`.

**Argument snapshots.** An argument is the value its expression had when it
was evaluated, but evaluating `$g` does not copy the global's backing, and the
arguments of one call are evaluated left to right. In `f($g, mutatesG())` the
second argument's call writes `$g` in place after the first was evaluated and
before the call uses it. `stampArgSnapshots` (`argsnap.go`, run after the
escape analysis, entry program with mutable globals only) marks each argument
that aliases a global (`$g`, `$g[i]`, `$g.f`) and is followed by an argument
that may mutate globals (a call to a function not proven `GlobalSafe`, a
module call, a callback builtin, a func-value call, a spawn) in the call's
`ArgSnap`. A marked argument is never borrowed (user function calls, which
bind each argument as it is evaluated) and is copied right after evaluation
on the paths that evaluate every argument first (builtins, module calls,
cross-module func values). Calls whose later arguments are pure, the
overwhelming majority, are unmarked and unaffected.

The REPL (whose global set grows across inputs) keeps the copying bind.
This is the extension of the scalar copy-elision to compound Kinds that
`bindParamValue` was documented as needing a mutation-safety proof for.
It removes accidental quadratics in read-only helpers (a `markdown`
block collector that binds the whole line list once per block; the
`countNodes` budget walk; a per-element script helper over a growing
list). Pinned by `internal/parser/borrow_test.go` (write-scan + type
gate), `internal/interpreter/borrow_test.go` (soundness gates + value-
semantics parity for module, globals-free script, and mutable-globals
script), and `globalsafe_internal_test.go` / `borrow_internal_test.go`
(the `GlobalSafe` fixpoint and the `entryGlobalsImmutable` wiring).

The same borrow extends to **in-function local bindings**: `markBorrowableDefs`
flags a `def x as T init EXPR;` when `T` is borrow-safe, `x` is never written,
and every binding the initializer could alias (`aliasRoots`) is a never-written
parameter / local / const (a global among them is covered by the gate; an
unbounded initializer - a builtin, module, or func-value call - is not
borrowable). `execDefine` then stores the value by alias instead of
`eagerCopy`, gated at runtime on `env.borrowDefs` (set from `methodBorrowCtx`
on the function call frame and inherited by its block frames), as a parameter
is gated by `bindArg`. Binding a nested value to a local and reading it is then
as cheap as indexing the chain in place (a read-only
`def s as Sh init byName($bk, $nm);` does not copy the struct), and a `match`
whose subject must first be bound to a variable needs no forced copy. Pinned by `TestBorrowableDefAnalysis` (the static decision) and
`internal/interpreter/borrow_local_test.go` (value-semantics parity and every
disqualifier).

A shared-marker copy-on-write protocol (`Value.shared *bool` +
`Share()` / `Ensure()`) was tried here and removed. It was inert: a
value receiver plus by-value `Environment.Get` / `GetAt` reads meant
the flag was set on a throwaway copy and never reached the stored
binding, so `Ensure` never detached and correctness always came from
the eager copies above. Reintroducing it write-through (store
`*Binding` so the marker propagates) was considered and rejected -
see [rejected.md](rejected.md).

Borrow and that COW are near-opposites. COW is dynamic: share every
value, detect a mutation at runtime and deep-copy then - so it needs a
live marker on every read/write and keeps a **mutable** shared backing
until a write splits it, the shared mutable state the eager-copy model
avoids. Borrow is static: share only the bindings a resolve-time proof
shows are **never** written, so there is no detach and no runtime
machinery, and the shared backing is **immutable** for the alias's
lifetime (as safe to share as a `const`). COW was also barely reachable:
because every store copies, two live bindings almost never alias. Borrow
targets the common case instead - parameter binding, a store site that
always copies, into read-only helpers. It is a narrow, provably-safe copy
elision on top of eager copies (like `rhsFreshLiteral`), not a
replacement of the model.

Aliasing correctness is exercised by
[`internal/interpreter/value_alias_test.go`](https://github.com/jennifer-language/jennifer/blob/main/internal/interpreter/value_alias_test.go) -
every "shared then mutated" corner case (nested lists, structs
containing lists, function-argument mutation, chained lvalues,
etc.). Anyone changing the mutation logic must add coverage there.

### Type stamping

After a literal like `[1, 2, 3]` evaluates, the resulting `Value` has
no `ElemTyp`. The declared type lives only on the receiving binding's
`DeclType`. To make subsequent operations - index-writes, parameter
checks, iteration types - have inner-type info without re-consulting
the declaration, the interpreter calls `stampDeclaredType(v, declType)`
at every binding boundary. The helper writes `ElemTyp` / `KeyTyp` /
`ValTyp` onto the value and recurses into nested compound elements so
deep type tracking is preserved for nested `$xs[i][j] = ...` writes.

### Index access

- **Reads** (`evalIndex`): out-of-bounds list indices and missing map
  keys are positioned runtime errors. Reads return the slot value by
  copy via Go's struct semantics, but the inner slice headers still
  alias - which is fine because reads can't mutate anything.
- **Writes** (`execIndexAssign` / `execFieldAssign`): both
  route through the unified `collectLvalueSteps` +
  `applyLvalueWrite` walker. The walker descends through the
  chain (any mix of `[index]` and `.field` nodes) on a fresh copy
  of the root binding, writes via `writeIndexedSlot` (index leaf)
  or the per-struct-field type check (field leaf), then commits
  back through `env.Assign`. Const enforcement fires once against
  the root binding (deep constness). Map writes to a missing key
  extend the map (insertion order preserved); writes to an
  existing key update in place. Struct field writes are
  type-checked against the declared field type stored in the
  `StructDef`.

### Structs

User-defined struct types live in `Interpreter.structs`
(`map[string]*parser.StructDef`), populated at `Run` time by hoisting
every top-level `def struct` before any other statement executes.
This mirrors the function-hoisting pass, and the same duplicate-name
check applies (`Run` rejects two structs with the same name; the
REPL silently redefines).

Per-instance values use `KindStruct` and carry the struct name plus
a `[]StructField` (each entry is `{Name, Value}`). The field slice is
stored in declaration order so `%v` rendering and `Equal` checks are
deterministic. `Copy()` deep-copies every field so value semantics
hold; `MatchesDeclared` matches by name (`p` typed `Point` matches
`Point`-typed declarations only).

The `def x as Name;` no-init path is handled by `zeroStructFor`,
which recurses through nested struct-typed fields so every leaf
field gets its declared zero. Unknown struct names are rejected here
and at `execDefine` time before the init expression is evaluated, so
the user sees `"unknown struct type"` rather than a misleading
type-mismatch error.

Module structs resolve by their canonical path, not their stem:
`zeroStructFor` and `lookupStructDef` take the `ModPath` carried on the
value / declared type and resolve through `moduleByPath`, falling back to
the stem-keyed `moduleByNS` only when no path is available. This is what
keeps field writes and zero-init working when two loaded modules share a
file stem (`a/util.j` and `b/util.j`) - a stem-keyed lookup is ambiguous
there and `moduleByNS` deliberately returns nil rather than guessing.

### Library-provided namespaced structs

Libraries register their own struct types via
`Interpreter.RegisterNamespacedStruct(libName, structName, fields)`.
The definition lands in `i.NSStructs` keyed by `nsKey{NS, Name}`,
parallel to `NSBuiltins` and `NSConstants`. Users write
`def x as os.Result;` for the type and `os.Result{ ... }` for the
literal; both forms resolve at use time via `resolveNamespacePrefix`
so aliases (`use os as o; def x as o.Result;`) work the same way as
they do for namespaced function calls and constants.

At runtime the value carries both `StructName` and an optional
`StructNS` tag. `MatchesDeclared` and `Equal` compare on the
`(NS, Name)` pair so a library `os.Result` is a distinct type from a
user-defined `Result`; `Display` prefixes the namespace
(`os.Result{exitCode: 0, ...}`). Field access, chained lvalues
(`$r.exitCode`, `$line.from.x = 5;`), value semantics, and deep
`const` all reuse the user-struct machinery - only the
type-resolution path differs.

User code may not register namespaced structs; the API is Go-side
only. Programs that want to declare their own structs keep using
the `def struct Name { ... };` bare form.

### Iteration

`execForEach` opens a fresh per-iteration scope so the loop variable
binding doesn't leak out and `def`-rebindings don't accumulate. It
borrows that frame from `envPool` (like `execBlock`) rather than
allocating a fresh map + slot slice each pass; `execFor` pools its
header frame the same way. For lists it walks elements in order; for
maps it walks keys in insertion order. The underlying map
representation is an ordered slice (`[]MapEntry`) rather than a Go
`map[K]V` to make this iteration deterministic and testable.

**Map hash index.** Because `Map` is an ordered slice, a naive
`indexInto` / `writeIndexedSlot` linear-scans it with `Value.Equal`
per entry, so building a map with `$m[$k] = $v` over N keys is
O(N^2). Each map `Value` carries an unexported side index
`mapIdx map[string]int` (encoded scalar key -> position) that turns
lookup, update, and the existence check on insert into O(1), so the
build is O(N). The index is *advisory*: it is consulted only when
`mapIndexUsable()` confirms it is complete - non-nil and
`len(mapIdx) == len(Map)` - and every operation falls back to the
linear scan otherwise, which is always correct. That length stamp is
what makes the index safe under value semantics: a map copied by
value shares the slice header until it diverges, a duplicate-key
literal has no 1:1 index, and a non-hashable key (any compound, or a
func / task / channel handle) is never indexed - all three fail the
stamp and scan. Only hashable scalar keys (string / int / bool / null /
float) are encoded (`mapKeyEncode`), mirroring `Value.Equal` exactly so
distinct keys never collide: an integer-valued, in-range float encodes
like the equal int (so `1.0` and `1` hash alike, as they compare `==`,
and `-0.0` normalises to `0`), any other float by its bit pattern, and a
NaN (unreachable) stays off the index. The index is (re)built by `DeepCopy` (every
binding-boundary copy owns a fresh one), by `evalMapLit`, and lazily on
the first indexed write.

## Environment

`Environment` is a parent-linked scope frame. Each frame carries two
storage backends, but a given binding lives in exactly one of them:

- **`slots []Binding`** - the slot-indexed view and the primary
  storage for **resolved** code. Sized from `Block.NumSlots` at frame
  construction (`NewEnvironmentSized`) or grown on demand from
  `DefineAt`. A resolved binding (`Slot >= 0`) is written here and
  **only** here - not the name map - so a call / block frame does no
  per-binding heap allocation.
  Each `Binding` carries its own `Name`, so the rare name-based readers
  can find a slot binding by scanning this (small, pooled) slice.
- **`vars map[string]Binding`** - the name-keyed view, the
  **fallback only**: it holds bindings the resolver did not stamp with a
  slot (`Slot < 0`) - the REPL (each turn is a fresh parse with no
  resolver context) and ad-hoc / hand-built ASTs. Resolved frames leave
  it empty.
- **`root *Environment`** - cached pointer to the
  outermost ancestor. Set at construction via `rootFor(parent,
  self)`; both `NewEnvironment*` and `borrowBlockEnv` populate it.
  `effectiveGlobal(env)` reads this field in O(1) with a
  defensive parent-chain walk as fallback. `releaseBlockEnv`
  clears it on release so pooled envs don't retain refs across
  borrow cycles.

`Binding{Value, DeclType, IsConst, Slot, Name}` carries `Slot` (`-1`
for name-only `Define`; non-negative for resolver-stamped `DefineAt`)
and `Name` (the identifier, so a slot-backed binding is self-describing
without a parallel name-map entry).

Storing the declared type lets `Assign` reject type-mismatching writes (you
cannot assign a string to a variable declared as int).

`lookupLocal(name)` is the shared per-frame lookup: it scans `slots` by
`Name` first (the resolved common case), then falls back to `vars`. All
the name-based readers below walk the parent chain calling it.

**Name-based API** (fallback path - REPL, spawn snapshot, ad-hoc AST):

- **`Define(name, val, declType, isConst)`** - adds to the current
  frame's `vars`; errors if the name exists *anywhere in the chain* (the
  spec forbids shadowing). Sets `Binding.Slot = -1`.
- **`Assign(name, val)`** - walks up the chain (`lookupLocal`) to find
  the binding; errors if it is a constant, the value's kind doesn't match
  its declared type, or the name is undefined. Writes back to the slot it
  came from, or to `vars` for a name-only binding.
- **`Get(name)`** and **`GetBinding(name)`** - walk up the chain via
  `lookupLocal`.
- **`copyBindingsInto(dst)`** - reconstructs a frame's name->value view
  (slots by `Name`, then `vars`) as detached deep copies; the one place
  `snapshotForSpawn` rebuilds captured scope from slot storage (globals
  are slot-backed too).

**Slot-based API** (fast path):

- **`DefineAt(slot, name, val, declType, isConst)`** - for `slot >= 0`
  installs the binding at `slots[slot]` (growing the slice if needed)
  with `Binding.Name = name` and does **not** touch `vars`; for
  `slot < 0` it takes the name-map fallback (with the enclosing-scope
  shadow check).
- **`GetAt(depth, slot, name)`** - walks `depth` parents then indexes
  `slots[slot]`. Falls back to `vars[name]` at the same depth when
  the slot is out of range (covers execution paths added to a
  resolver-less frame).
- **`GetBindingAt(depth, slot, name)`** - metadata companion to
  `GetAt`.
- **`AssignAt(depth, slot, name, val)`** - const / type-mismatch
  checks match the name path; on success writes to `slots[slot]` (the
  slot is authoritative for a slot-backed binding).

`NewEnvironmentSized(parent, numSlots)` is the constructor
that pre-sizes `slots` from the resolver's hint, avoiding a grow on
every `DefineAt` in hot loops. `NewEnvironment(parent)` (no size)
is used by REPL paths and by ad-hoc paths where the slot
count isn't known upfront.

`execBlock` opens a fresh child `Environment` for each `{...}` block, so
variables declared inside don't leak out. `for` opens its own scope wrapping
init/cond/step/body so the init variable is visible throughout the loop
without escaping it.

### Resolver / runtime scope alignment

The resolver's static scope stack has to match the runtime's env
chain exactly, or `(Depth, Slot)` addresses land in the wrong place.
Three carve-outs where the resolver deviates from "one AST scope =
one runtime frame":

- **`try` body runs in the enclosing env**, not a fresh frame:
  `execTry` calls `execStmts(body.Stmts, env)` directly. The resolver
  walks try-body statements inline in the current scope to match.
  Only the catch handler gets a proper scope push (matches the
  runtime's `catchEnv := NewEnvironment(env)`).
- **For-header init lives in `forEnv`**, body lives in a nested
  block frame: `execFor` creates one env for the header and
  `execBlock` nests another for the body. The resolver tracks them
  as separate scopes.
- **Spawn bodies are unresolved.** The runtime's
  `snapshotForSpawn` produces a two-frame duplex (globals-snap +
  locals-snap) that doesn't align with the resolver's single-frame
  view of "the enclosing scope." Rather than invent depth arithmetic
  to reconcile the two, the resolver skips spawn-body statements
  entirely and every reference inside falls back to name-based
  lookup at runtime. Spawn is coarse-grained concurrency dispatch,
  not hot-loop territory, so the cost is bounded.

### Function call frames

Three compounded moves cut the recursive-call cost:

- **Environment pool.** `environment.go` exports `borrowBlockEnv` /
  `releaseBlockEnv` on top of a package-level `sync.Pool`. Every
  `execBlock`, every `evalCall`, and every `CallByName` borrows a
  frame on entry and returns it before returning to the caller.
  Release zeroes both the `vars` map (delete-in-place) and every
  used slot entry so the pool doesn't retain compound-value
  backings live between uses. Jennifer has no closures - no value,
  no library, no AST node can capture an env pointer past its
  block's dynamic extent - so the pool is safe by construction.
  The two envs that outlive their creating call (`i.global`, the
  goroutine-root snapshots from `snapshotForSpawn`) stay on the
  non-pooled `NewEnvironment` path. `snapshotForSpawn` copies the
  *launching* goroutine's own root frame - `effectiveGlobal(env)`,
  which is `i.global` in serial code but the enclosing spawn's
  detached global snapshot inside a spawn body - never the live
  `i.global`, so a nested spawn snapshotting on a background
  goroutine can't race the main goroutine's global writes.
- **Pre-resolved callees.** `CallExpr` carries a `Method
  *MethodDef` pointer (see `internal/parser/ast.go`). During
  `Resolve` the resolver stamps the pointer when the callee names
  a hoisted top-level user function. `evalCall` consults the pointer
  first; only when it's `nil` (REPL turns, hand-built ASTs) does
  it fall back to `i.methods[c.Callee]`. Builtins keep the
  pointer `nil` because the namespaced / global registries still
  need the `use`-activation check on every call.
- **Slot-based parameter binding.** The resolver's `resolveMethod`
  assigns parameters to slots `0..N-1` in the call frame. At
  runtime `evalCall` borrows the call frame via
  `borrowBlockEnv(effectiveGlobal(env), len(m.Params))` and binds
  each parameter through `DefineAt(idx, ...)`. No map hashing per
  parameter; the resolver's slot numbers align with the
  interpreter's storage layout automatically.

### Namespaced-call fast paths

Four more fast paths on top of the function-call frame work:

- **Pre-resolved namespaced calls / constants.** `QualifiedCallExpr`
  carries an opaque `Fn any` field; `QualifiedConstRefExpr` carries
  an opaque `Const any` field (see `internal/parser/ast.go`, kept
  `any` to avoid a parser -> interpreter import cycle). Because
  `parser.Resolve` runs before `processImports`, the pre-fill
  can't live in the resolver pass. `Interpreter.resolveQualifiedRefs`
  runs from `Run` after `processImports` and `loadModuleImports`
  (so module-alias refs stamp too - see [Execution model](#execution-model))
  and walks the AST once, stamping the exact `Builtin` / `Value` the runtime would
  otherwise look up per call. `evalQualifiedCall` and
  `evalQualifiedConst` use the pre-filled pointer via type
  assertion; unresolvable prefixes stay nil and hit the original
  `resolveNamespacePrefix + NSBuiltins` path with its original
  error messages.
- **Int-int / float-float comparison fast paths.** `evalComparison`
  checks both operand Kinds before falling through to
  `AsFloat`, like the int-int block in `evalArithmetic`. Numeric loops (`$i < N`) skip two float
  conversions per iteration.
- **Immutable-Value copy elision in arg binding.** `evalCall`'s
  arg loop routes through `bindParamValue(v, declType)`. For
  scalar Kinds (`int / float / bool / null / string`) it returns
  `v` unchanged - `Value.Copy` and `stampDeclaredType` are
  no-ops for those Kinds. Compound Kinds still go through the
  full copy + stamp path so value semantics + declared-type
  propagation stay correct.
- **`effectiveGlobal` caching via `Environment.root`.**
  `effectiveGlobal(env)` is an O(1) read of the cached root (set by
  `rootFor(parent, self)`, cleared by `releaseBlockEnv`; see
  [Environment](#environment)), with a defensive parent-chain walk as
  fallback for hand-built envs.

### Expression micro-optimizations

- **Constant folding at parse time.** `BinaryExpr` and
  `UnaryExpr` carry a `Folded Expr` field
  (`internal/parser/ast.go`). The resolver runs
  `tryFoldBinary` / `tryFoldUnary` (`internal/parser/fold.go`)
  after each subtree's operands resolve; when both operands
  are literal (transitively through their own `Folded` fields
  via the `asLit` helper), the operation runs at parse time
  and the result gets stamped on `Folded`. `evalBinary` /
  `evalUnary` check `Folded` first and delegate to it,
  skipping the operand walk + op switch. Runtime errors
  (division by zero, negative shift count) leave the node
  unfolded so the runtime hits the same error at the same
  source position - the fold pass never surfaces a parse-time
  error the runtime wouldn't have raised. Two exactness
  carve-outs mirror the runtime: an int/int comparison folds on
  the exact `int64` values (not a lossy `float64`), and a
  **mixed int/float comparison is left unfolded entirely** so the
  runtime's exact `compareIntFloat` decides it - promoting the
  int to `float64` at fold time would lose precision above 2^53
  and make `9007199254740993 == 9007199254740992.0` wrongly fold
  to `true`. `!=` folds wherever `==` does (it is the negation).
  Mixed int/float *arithmetic* still folds (the int->float
  promotion there matches the language's arithmetic semantics).

### String interpolation

A cooked `"..."` string with one or more `{expr}` slots lexes to a single
`TOKEN_STRING_INTERP` whose `Parts []lexer.StringPart` alternate literal chunks and
expression-source slots. `readCookedString` drives this: `\{` / `\}` decode to
literal braces, an unescaped `{` calls `scanInterpSlot` (which brace-counts nested
`{}` so a map literal in a slot balances, and copies a nested string literal
verbatim via `copyNestedString` so its braces / quotes do not disturb the count),
and a bare `}` is a positioned lex error. Each expression slot records its
**absolute** source line/col; the parser's `buildInterpString` sub-lexes it with
`lexer.TokenizeAt` (seeding that position so diagnostics and resolver annotations
point at the real spot) and sub-parses it to a single `expr` - a trailing non-EOF
token is a "slot must be a single expression" error, an empty slot is rejected. The
result is a `parser.InterpStringExpr` (`Parts []InterpPart`, each a literal `Lit`
or an `Expr`). The resolver descends into each slot's `Expr` (undefined-variable /
shadowing stay parse-time errors), so `lint` and `profile` - which walk the same
tree - see slot expressions as ordinary code (`lint` adds the `L204`
interpolation-slot-complexity check; `profile` attributes a slot's call to the
slot's own position). Evaluation stringifies each slot value with `Display` (the
`convert.toString` form, so no `use convert` is needed and any kind interpolates)
and concatenates (sugar over concat + `convert.toString`, kept as a node so
the toolchain sees slot code). A raw `'...'`
string never interpolates (it lexes to a plain `TOKEN_STRING`); `fmt` re-emits a
`TOKEN_STRING_INTERP` from its captured `Raw`, so slots survive a format verbatim.

## Execution model

1. `Interpreter.Run(prog)` calls `parser.Resolve(prog)` first
   so the AST carries `(Depth, Slot)` annotations before any structural
   check runs. Resolve is idempotent: re-running on an already-resolved
   program produces the same annotations. Any undefined-variable or
   shadowing error surfaces here as a positioned parse-time
   diagnostic, not a runtime error. So does a top-level read of a `def
   const` defined further down (`laterConsts`): top-level statements run
   in source order, so that read could never succeed. Function bodies are
   resolved after the top level, against the complete globals frame, so a
   function may name a constant defined lower down; if such a function runs
   before the definition has (during top-level setup), the read finds the
   still-empty global slot and `evalExpr`'s `ConstRefExpr` arm reports that
   ordering problem rather than a misleading "use `$NAME`" hint.
2. Records `Imports` into `i.imported`, activating library namespaces.
   Module aliases are not loaded yet - that happens in step 4 - which
   is why the qualified-ref pre-resolution is deferred until after it.
3. Collects every `MethodDef` into `i.methods` (functions are hoisted: callable
   regardless of source order). During collection it enforces two rules:
   no duplicate function names, and no function name that collides with a
   registered builtin whose owning library has been imported (the no-shadowing
   rule extended to builtins - see `evalCall` below).
4. Creates the global `Environment` (`i.global`). Before executing the
   body, `loadModuleImports(prog)` loads and initialises every
   `import "..."` module (see [Module loading](#module-loading)), so an
   imported module is fully initialised before the importer's body runs.
   Then `resolveQualifiedRefs(prog)` walks the AST once and pre-fills
   every `QualifiedCallExpr.Fn` / `QualifiedConstRefExpr.Const` against
   the now-populated namespace AND module-alias tables. A library builtin,
   a library const, a module-alias const (`m.CONST` - its
   boundary-retagged, deep-const value cached on the node), and a
   module-alias function (`m.fn(...)` - a `moduleMethodTarget{mod,
   *MethodDef}` cached in `Fn`, dispatched through `dispatchModuleMethod`
   -> `CallMethodWith`) all resolve to a direct dispatch instead of a
   per-access lookup (an unexported / missing name stays unstamped, so its
   runtime error is unchanged). This pass runs after BOTH import
   mechanisms (the tables didn't exist during Resolve) and is skipped by
   the REPL, which builds its namespaces incrementally. The same walk
   collects every `StructLit`, and `checkStructLits`
   (`structlit_check.go`) then checks each literal's field set against its
   resolved struct / enum-variant declaration (same-file, library, or
   module), so an unknown or missing field fails the load with the message
   `evalStructLit` / `evalEnumLit` would raise, before any statement runs.
   A library struct literal may leave fields out (each takes its zero value,
   built by `zeroPayload` as for a zero declaration), so only an unknown
   field fails one; `Error` may leave out its location fields.
   A literal whose type does not resolve is left to the runtime path. Finally executes
   `prog.TopLevel` statements in source order in the global scope.
5. Function calls execute the body in a fresh call frame whose parent is
   `effectiveGlobal(env)` (an O(1) `env.root` field read; in
   serial code that's `i.global`, inside a `spawn` body it's the
   spawn-globals snapshot). The call frame is borrowed from
   the environment pool pre-sized to the parameter count; parameters
   bind through `DefineAt` into slots `0..N-1`; the callee is looked up
   via the pre-resolved `CallExpr.Method` pointer when set, falling
   back to `i.methods` when it isn't (REPL turns). Scalar-Kind
   arguments skip the Copy + declared-type-stamp step via
   `bindParamValue`. Top-level variables are visible inside functions
   (subject to the no-shadowing rule). The body's return value (bare
   `return;` -> `null`; `return EXPR;` -> the expression's value;
   falling off the end -> `null`) propagates back to the caller.

`EvalInteractive` (the REPL entry point) skips step 1 - each REPL turn
is a fresh parse whose scope can't be resolved without the accumulated
global context from prior turns. The runtime handles this by leaving
resolver annotations at their `-1` sentinel and using the name-based
Environment API. The perf cost is limited to REPL sessions, which
aren't hot loops.

There is no required entry point. A program with only imports and function
defs is valid and runs to completion immediately (those functions are
never called).

## Module loading

`import "PATH.j" [as NAME];` (an `ast.ModuleImportStmt`) loads another
`.j` file as a module. `internal/interpreter/module.go` holds the
loader; `internal/module` holds the path resolver.

`EnableModules(baseDir, searchDirs, load, setup)` wires the system onto
the root interpreter and builds a `moduleReg` shared across the whole
run:

- `cache map[string]*loadedModule` - the run-once table keyed by
  resolved absolute path.
- `stack []string` - canonical paths currently loading, for cycle
  detection.
- `search []string` - the module search path (system module dir, then
  each `-I` dir).
- `load func(string) (*parser.Program, error)` - lex + preprocess +
  parse a resolved file. The CLI passes `main.go`'s
  `loadModuleProgram`; tests pass an equivalent.
- `setup func(*Interpreter)` - install the standard library into a
  module's fresh sub-interpreter. The CLI passes `installLibraries`.

`loadModuleImports(prog)` runs from `Run` (step 4) before the body. For
each import it calls `loadModule(path, at)`:

1. `module.Resolve(path, baseDir, search)` -> canonical absolute path
   (local `./`/`../` and absolute `/` resolve directly; a bare name
   walks `search`, where a name found in two search dirs is a hard
   error). Resolution errors are positioned at the import statement.
2. **Cycle check** - if the canonical path is already on `reg.stack`,
   error `module cycle: A -> B -> C -> A` naming every edge.
3. **Run-once** - if it's already in `reg.cache`, return the cached
   `*loadedModule` without re-running.
4. `reg.load(canonical)` parses the module (parse errors stay
   positioned in that file).
5. A **fresh sub-interpreter** is the module's own scope: `sub := New();
   reg.setup(sub); sub.modReg = reg; sub.baseDir = dir(canonical)`.
   Sharing `reg` means the sub's own imports use the same cache and
   stack.
6. Push the canonical path, `sub.Run(modProg)` (which recurses into the
   sub's imports, then runs its body), pop the path, and cache the
   result.

The recursion is what delivers the guarantees: **run-once** from the
cache, **depth-first post-order init** from initialising a module before
its importer's body, and **cycle detection** from the load stack. Load
errors (a parse error or a `throw` during a module's top level)
propagate out of `Run` as ordinary errors and fail the program; they are
**not** catchable, because an `import` is a top-level declaration, not an
expression, so it cannot sit inside a `try`/`catch` (the parser rejects
`import` in a block).

### Module scope and namespacing

A module's top level is **declarations-only**:
`checkModuleDeclarationsOnly` (run in `loadModule` after parse) allows only
`def const` in `TopLevel` - structs, functions, and imports live in their own
`Program` slices - so a mutable `def` or a free-standing statement is a
positioned load-time error. Scripts run through the CLI never reach this
check, so a `jennifer run` program keeps mutable top-level `def` and
free-standing statements.

`loadModuleImports` binds each import's alias (the `as NAME` clause, or the
file stem via `moduleStem`) into the importer's `moduleAliases`
(`map[string]*loadedModule`), collision-checked against library prefixes
(`nsPrefixes`) and other module aliases; re-binding an alias to the module it
already names is a no-op (as a duplicate `use` is), only a different module is a
collision. Consumer-side resolution rides the
existing qualified-reference eval layer, since the parser resolver and
`resolveQualifiedRefs` already defer unknown prefixes to runtime:

- `evalQualifiedCall` checks `moduleAliases` before the library path and,
  on a hit, dispatches `alias.fn(args)` through `callModuleMethod` /
  `dispatchModuleMethod` (or straight through `dispatchModuleMethod` when
  `resolveQualifiedRefs` stamped the call) - arguments evaluated in the
  consumer's env, then `m.interp.callMethodWithDepth` runs the body against
  the **module's** globals and functions, threading the caller's call-depth
  counter. Arity / type
  mismatches reposition at the consumer's call site; a `runtime error`,
  `throw`, or `exit` from the module body propagates unchanged.
- `evalQualifiedConst` reads `alias.CONST` from the module's global scope
  (`m.interp.global.GetBinding`), or returns the boundary-retagged value
  `resolveQualifiedRefs` cached on the node.

`use` non-transitivity, run-once sharing, and `-race` safety all fall out
of the fresh-sub-interpreter-per-module model: a module's interpreter holds
only immutable constants and read-only functions, so concurrent calls from
parallel `spawn` bodies share no mutable state.

### Exports, visibility, and cross-module struct identity

`export` marks a top-level `func` / `def struct` / `def const` as a
module's public surface (`parseExported`); `collectExports` records the
set on the `loadedModule`, and `callModuleMethod` / `moduleConst` /
`evalStructLit` / the `def`-type check reject an unexported `alias.member`
with a positioned "not exported from module" error. `export` is illegal in
a script: `Run` calls `rejectExportInScript` unless `isModule` is set (only
`loadModule`, and the `jennifer test` overlay via `SetModuleContext`, set
it). `checkReferentialClosure` (at load) rejects an exported struct field
or exported function parameter typed as a *private* module struct; only the
module's own bare struct types are checked, so library / namespaced types
cross freely.

**Cross-module struct identity** is boundary translation, not internal
tagging. A module's own structs stay bare (`StructNS ""`) inside the module,
where all the existing struct machinery works unchanged. `retagStructs`
re-tags them to `(module-stem, name)` as a value crosses *out* to an
importer (a `callModuleMethod` return, a `moduleConst` read) and back to
bare on the way *in* (a `callModuleMethod` argument), recursing through
struct fields, list elements, and map values; library / other-module
structs (a different namespace) are untouched. So the consumer can type a
module struct (`def p as points.Point`, or `def ps as list of points.Point`;
the `def`-check's `resolveDeclaredStructNS` stamps the type's namespace to
the module stem, recursing into list / map / task element types so a
collection element type matches the identity `retagStructs` gives the
values), construct one (`points.Point{...}` in `evalStructLit`), read its
fields, and pass it back - all type-checking - while `a.Point` and `b.Point`
stay distinct `(namespace, name)` pairs. Struct identity is keyed by the
module's **canonical path**, carried in the `ModPath` field on both `Value` and
`parser.Type` (empty for library / user structs) and compared by `Value.Equal` /
`MatchesDeclared`; `StructNS` holds the file **stem** purely for display, so two
module files sharing a basename (`a/util.j`, `b/util.j`, or `@jennifer/benchmark` vs
`@alice/benchmark`) are distinct types while both still render as
`benchmark.Point`. The boundary retag threads `(StructNS, ModPath)` so a foreign
struct that only shares the stem is left untouched, and function parameter types
are stamped alongside `def` types so a `func f(s as mod.Struct)` param carries
the same identity the passed value does. The retag copies only compound values
at the boundary (module calls are not a hot path). Declared-type
stamping happens once, single-threaded, before execution:
`resolveDeclaredTypesOnce` (run from `Run` after `loadModuleImports`) walks
every declared type - top-level, function bodies, and spawn bodies - stamps it,
and marks the AST node `parser.Type.Resolved`. The per-execution
`resolveDeclaredStructNS` in `execDefine` early-returns on a resolved node, so a
`def` inside a loop, or a shared function / spawn body reached from concurrent
goroutines, re-reads the marker instead of re-stamping - no write-write race on
the shared type node. The pass is best-effort (an unresolvable type is left for
`execDefine` to error on at its original position), and the marker also sidesteps
a latent idempotency gap: re-resolving an already-stamped *library* alias would
otherwise hit the "canonical namespace is aliased" rejection. (An importer alias
to a module stem is also recognised on any later pass via `moduleByNS`.)

### Func values across the module boundary

A `func` value is a handle to a top-level function (`Value.Fn *parser.MethodDef`).
It also carries **`Value.FnHome *Interpreter`** - the interpreter that *defined*
the function - stamped when a bare function name is evaluated as a value
(`i.methods[name]` -> `FuncVal(m)` with `FnHome = i`). This matters only across a
module boundary: a func value is a value like any other, so it is routinely
passed *into* a module (a `lists.map` predicate, an `args.dispatch` handler, a
`webapi` authenticator) or returned *out* of one. Without a home, `evalCallValue`
would run the body in the **caller's** interpreter, whose namespace table does
not carry the func's own `use` / `import` aliases - so a body that references
`strings.upper` or constructs `mymod.Thing{...}` would fail with "unknown
namespace" against the wrong interpreter.

So `evalCallValue`, when `fv.FnHome != nil && fv.FnHome != i`, dispatches the call
to the home: it evaluates the arguments in the caller, then runs
`home.callMethodWithDepth(fv.Fn, env.depth, args...)` - the body executes in its
home context, its aliases resolve, and `env.depth` threads the caller's
call-depth counter so recursion bouncing through a cross-boundary func call still
trips the catchable guard (as it does through `dispatchModuleMethod` /
`meta.callMain`). Same-interpreter calls (`FnHome == i`, or a legacy `nil` home)
keep the ordinary `callUserMethod` path with no retag.

`callMethodWithDepth` raises its **argument-count and argument-type mismatches as
`*runtimeError`** (not a plain Go error), so a wrong-arity or wrong-kind call to a
cross-boundary func value (or module function) is **catchable** by a surrounding
`try`/`catch`, exactly like the same-interpreter `callUserMethod` path - `execTry`
only converts `*runtimeError` / `*ErrorSignal` into a catch value, so a plain
error would escape every handler. This keeps a framework's safety net intact
against a mis-declared handler: a two-parameter
`web.onError` handler (the hook is called with one argument) fails *catchably*, so
`web`'s dispatch logs it and still answers the request instead of leaving the
connection hung.

Struct arguments and the return value cross the boundary through **`crossRetag(v,
from, to)`**, which reuses `retagStructs` (above) in a two-pass form:
`to`'s own structs - tagged with `to`'s identity as `from` sees them - are
un-tagged to bare inside `to`, then `from`'s own structs - bare inside `from` -
take `from`'s identity so `to` recognises them as that module's structs. Because
a struct belongs to exactly one module, the two passes target disjoint owners, so
the *same* helper serves both directions (arguments crossing in, the result
crossing out) and every pairing (host->module, module->host, module->module). The
webapi authenticator is the worked case: a host func value returning a
`webapi.Identity` (built through the host's `webapi` alias, so tagged
`(webapi-stem, webapi-path)` in the host) is called from inside the `webapi`
module and its return is retagged bare, exactly the identity the module's own
code expects. `FnHome` is a shared pointer, copied with the (immutable) func value
like `Fn`, and never mutated. Pinned by `cmd/jennifer/funcvalue_home_test.go` and
`args_dispatch_test.go`.

## Builtins and libraries

Each library lives in its own Go package under `internal/lib/<name>/` and
registers its functions (and constants) on the interpreter. User-facing
reference docs are split per library:

- [libraries/io.md](../libraries/io.md) - `printf`, `sprintf`, format verbs
- [libraries/convert.md](../libraries/convert.md) - `toInt`, `toFloat`, `toString`, `toBool`, `typeOf`
- [libraries/math.md](../libraries/math.md) - `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`
- [libraries/strings.md](../libraries/strings.md) - `upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`/`trimLeft`/`trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`
- [libraries/os.md](../libraries/os.md) - `os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`, `os.ARGS`, `os.getEnv`, `os.hasFlag`, `os.flag`, `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill`
- [libraries/meta.md](../libraries/meta.md) - `meta.VERSION` (build version), `meta.BUILD` (toolchain), `meta.SYSMODDIR`, `meta.CAPABILITIES`, `meta.call` / `meta.callMain`
- [libraries/index.md](../libraries/index.md) - catalog and organizing principles (every other library has its own page there)
- `len(EXPR)` is a language built-in primary, not a library function. See [grammar.md](grammar.md).

What follows is the implementation contract, not the user-facing API.

Library functions are Go closures registered with the interpreter:

```go
type BuiltinCtx struct {
    Out    io.Writer  // stdout-like effects write here
    Err    io.Writer  // stderr-like effects write here
    In     io.Reader  // stdin-consuming builtins read here
    InREPL bool       // true when the call originates from the REPL
    File string; Line, Col int // call-site position (RaiseError anchors here)
    Depth  *int       // caller's call-depth counter (meta.call / callMain thread it)
    Cancel *TaskState // the running spawn's task state (task.cancelled)
    // unexported: interp (backs Invoke / HostSharedState), root (caller's global frame)
}
type Builtin func(ctx BuiltinCtx, args []Value) (Value, error)

# In a library package:
func Install(in *interpreter.Interpreter) {
    in.RegisterNamespaced("io", "printf", printf)
    in.RegisterNamespaced("io", "sprintf", sprintf)
    in.RegisterNamespaced("io", "readLine", readLine)
    in.RegisterNamespaced("io", "eof", eofFn)
}
```

`BuiltinCtx` gives builtins access to the output / input streams, the
REPL flag, the call-site position, and (through `Invoke`) a way to call
back a func-value argument. `Interpreter.In` defaults to `os.Stdin`; the REPL sets
`Interpreter.InREPL = true` so `readLine` / `eof` refuse rather than
racing the line editor for input.

`Interpreter.Builtins` stores `builtinEntry{Lib, Fn}` per name for the
rare globals-publishing library (`RegisterGlobal`; no shipped library
uses it today - the removed `core` was the last). A bare call to
`foo(...)` resolves in this order:

1. User-defined function `foo` in `i.methods`.
2. Builtin `foo` - **but only if its owning library has been `use`d**. The
   error otherwise quotes the right library name: `` `foo` requires `use <lib>;` ``.

The no-shadowing check at hoist time uses the same lookup: a user function
that collides with an imported library's builtin is rejected.

User-defined **constants** (via `def const NAME as TYPE init EXPR;`)
live in the same Environment as variables and resolve through
`evalExpr`'s `ConstRefExpr` case (bare-identifier lookup). They
participate in the no-shadowing rule like everything else.

Library-provided constants (`math.PI`, `math.E`, `time.UTC`,
`time.PROGRAM_START`, `os.PLATFORM`, ...) are namespaced and
registered through `RegisterNamespacedConst`. They resolve
through `QualifiedConstRefExpr` - see the "Namespaced libraries"
subsection below. The `RegisterGlobalConst` flat-namespace
constant API and the bare-IDENT `ConstRefExpr` fallback for
library constants are not used by any shipping library; the
fallback path remains in the interpreter as exported API surface.

### Namespaced libraries

Domain libraries register through the namespaced API:

```go
in.RegisterNamespaced("os", "platform", platformFn)
in.RegisterNamespacedConst("os", "JENNIFER_OS", interpreter.StringVal("linux"))
```

Both entries are keyed by `(namespace, name)` in `NSBuiltins` /
`NSConstants`. The library's name doubles as the namespace prefix
(future libraries may decouple them, but today they always match).
Registering through the namespaced API also flags the lib in
`knownNamespaces`.

**Only libraries flagged in `knownNamespaces` may be aliased.**
`processImports` rejects `use NAME as ALIAS;` for any library that
registered exclusively through `RegisterGlobal` / `RegisterGlobalConst`
(the flat API) with the message `library "NAME" has no namespaced
names; ` + "`as ALIAS`" + ` aliasing is meaningless here` - such a
library has no prefix to rename. Every shipped library registers namespaced, so every one can be
aliased.

`processImports` builds two maps from each `use NAME [as ALIAS];`:

- `nsPrefixes[prefix] = canonicalNamespace` - the prefix that's
  active at call sites; `prefix == canonical` for `use os;`,
  `prefix == alias` for `use os as o;`.
- `nsAliasedAway[canonical] = alias` - records that the canonical
  name has been shadowed by an alias, so a later `os.foo()` after
  `use os as o;` errors with a `did you mean ` + "`o`" + `?` hint.

Resolution at a `QualifiedCallExpr` / `QualifiedConstRefExpr` goes
through `resolveNamespacePrefix(prefix)`:

1. If `prefix` is in `nsPrefixes`, use the canonical namespace it
   points at.
2. Else, if `prefix` is in `nsAliasedAway`, emit the "did you mean
   `<alias>`?" hint.
3. Else, if `prefix` is the canonical name of a *known* namespaced
   lib the program forgot to `use`, emit a `requires ` + "`use prefix;`"
   reminder.
4. Else, emit `unknown namespace`.

The no-shadowing rule for top-level functions (`checkMethodNoShadow`)
adds one more clause: a function name that matches an active namespace
prefix is rejected (`func os() {}` errors after `use os;`, but is
fine after `use os as o;` because only `o` is reserved as a prefix).

Implementation-only notes per library (for the user-facing API, follow the
links above):

**`internal/lib/io`**: `printf` and `sprintf` share a `formatArgs` helper
with three shapes - 0 args errors; first-arg-is-string triggers format
substitution; single non-string arg writes the value's `Display()` form
(the "just print this value" shortcut). `printf` writes to
`Interpreter.Out`; `sprintf` returns a `KindString` value and ignores
the writer.

**`internal/lib/math`**: `floor`/`ceil`/`round` accept int (identity) or
float and return `int`. `round` uses Go's `math.Round` (half away from
zero). `math.PI` and `math.E` are registered via
`RegisterNamespacedConst` and resolved through `QualifiedConstRefExpr`
like every other namespaced constant; the namespace prefix is
reserved for the rest of the program once `use math;` runs.

**`internal/lib/convert`**: conversions live behind the namespace -
`convert.toInt(v)`, `convert.toFloat(v)`, `convert.toString(v)`,
`convert.toBool(v)`. A bare type keyword in expression position (`int(...)`,
`bool(...)`) is a positioned parse error whose hint names the matching
`convert` builtin. `typeOf` is a normal IDENT call.
`convert.toBool(v)` implements canonical-only conversion at all source kinds
(`0`/`1` for int, `0.0`/`1.0` for float, `"true"`/`"false"` for string) -
non-canonical values produce a positioned error, not silent coercion.

**`internal/lib/strings`**: all indices and lengths are **rune-based**
(Unicode code points), implemented via `unicode/utf8`. (String length
is the language built-in `len`, which counts runes.) `indexOf` returns a rune index (translated from the byte index Go's
`strings.Index` returns); `substring` uses a small
`byteOffsetForRune` helper to convert rune-indexed bounds back to byte
slicing on the underlying string. `repeat` guards against multiplication
overflow before calling Go's `strings.Repeat` to avoid the panic in the
standard library. The Go package is named `stringslib` to avoid colliding
with the standard `strings` package, which it uses.

## Runtime errors

`*runtimeError` carries optional `File`/`Line`/`Col` and a `Kind` tag
(defaults to `"runtime"` when the originating site doesn't
specialise it). Errors render as `runtime error at FILE:L:C: <msg>`
(or `runtime error at L:C: <msg>` when the file is unknown). All
five Jennifer error types - `*lexer.LexError`, `*preproc.PreprocessError`,
`*parser.ParseError`, `*runtimeError`, and `*ErrorSignal` -
implement a small `Position() (file string, line, col int)` interface.
The CLI uses that interface (no string parsing) to look up the right
file and print a caret under the offending source line.

### Catchable errors

`try { body } catch (NAME) { handler }` runs the body and, on an
error, binds the thrown value to `$NAME` in a fresh per-handler
scope. Two sentinel paths can produce the catchable error:

- **`*ErrorSignal`** - raised by `throw EXPR;` (`execThrow`). Carries
  the thrown `Value` plus the throw's source position. A thrown `Error`
  with no location (`file` "", `line` / `col` 0) has the throw site
  stamped into it (`stampThrowSite`), which is what lets an `Error`
  literal omit those three fields (`errorLocationZero`, honoured by both
  `evalStructLit` and the load-time `checkStructLits`); an `Error` that
  already carries a location - a caught runtime error rethrown - keeps
  it. Uncaught signals reach the CLI through the same `positioned`
  interface as `*runtimeError`.
- **`*runtimeError`** - raised by any builtin or language operation
  (out-of-bounds index, missing map key, type mismatch, etc.). When
  one reaches an enclosing `try`, `execTry` wraps it via
  `runtimeErrorToValue` into an `Error` struct (`kind`, `message`,
  `file`, `line`, `col`) and binds it like any other thrown value.

`*ExitSignal` is **not** routed through this path - the spec puts
process exit outside the recoverable-error scope, so `execTry`
propagates it untouched. `blockResult` flags (`hasReturn`,
`hasBreak`, `hasContinue`) flow through `execTry` unchanged so the
surrounding function / loop sees them.

The canonical `Error` struct is auto-hoisted into `i.structs` by
both `Run` and `EvalInteractive` before any user struct definition
runs (`canonicalErrorStructDef()`). User code may not redefine it -
the existing duplicate-struct check fires with
`struct "Error" is defined more than once`.

`runtimeError.Kind` is the symbolic tag surfaced as `$err.kind` in
the catch block. The default is `"runtime"`; a few sites raise a
specific tag (via `RaiseError` / `KindedError`): `"limit"` (the
call-depth and recursion-memory guards, `archive`'s decompression caps),
`"timeout"` (`net` deadlines, `task.waitTimeout` / `waitAnyTimeout`),
`"closed"` (a `net` call on a closed socket), `"assertion"` (the
`testing` asserts).

## Errors and positions (cross-file)

The pipeline plumbs file information through three layers:

1. The lexer attaches the source file path to every token (`Token.File`).
   `TokenizeWithFile(source, file)` is the entry point; the no-arg
   `Tokenize` leaves `File` blank for unattributed input.
2. The preprocessor preserves each spliced token's `File` field when
   splicing `include "path.j";`, so tokens from an included file keep that
   file's path, line, and column. (A module `import` is parsed as its own
   program, so its nodes carry the module file's path directly.)
3. The parser propagates `File` from tokens to every AST node (each
   `pos` struct carries `File`, `Line`, `Col`). Synthesized nodes (e.g.
   `BinaryExpr`) copy the file from the left operand.

When the interpreter raises a `*runtimeError`, it pulls file/line/col from
the offending node via a small `posFor(node)` helper. The CLI's
`printErrorContext` type-asserts the `positioned` interface, and if the
reported file differs from the program's main file it loads that file from
disk before slicing out the snippet to display.

## Concurrency

The `spawn` keyword, the `task of T` type kind, and
the `task` library together form Jennifer's
concurrency surface (with `channel of T` and the `channel` library as
the streaming counterpart). The user-facing model is
[docs/user-guide/concurrency.md](../user-guide/concurrency.md);
this section describes the runtime side.

### Goroutine mapping

`spawn { ... }` is a primary expression: `parser.SpawnExpr`
carries the body as `[]Stmt`. The interpreter handles it through
`evalSpawn`:

1. Build a fresh capture environment with `snapshotForSpawn(env, caps)`.
2. Allocate a `TaskState` with a freshly made `Done chan struct{}`.
3. Register the state in the interpreter's per-run task registry.
4. `go i.runSpawn(state, ex, spawnEnv)`.
5. Return a `Value` of kind `KindTask` wrapping the state pointer.

`runSpawn` closes `state.Done` from a `defer` so all observers
(`task.wait`, `task.waitAll`, `task.waitAny`, the exit-time scan)
see the close as a happens-before edge before reading `state.Result`
or `state.Err`. The goroutine itself executes the body via the
existing `execBlock` over the captured env; this is the same path
top-level statements take, so the spawn body sees the full
interpreter (libraries, structs, function definitions, namespacing).

`return EXPR;` in the body becomes `state.Result`. A `blockResult`
with `hasReturn=false` but no error means an implicit `null` return
(matches function-call semantics). `break` or `continue` that
escapes its loop inside the body becomes a positioned error
("`break` outside a loop" / "`continue` outside a loop") via
`unhandledLoopFlowError`; loop-flow can't cross the spawn
boundary, mirroring how it can't cross a function-call boundary.

### Value-semantics capture

`snapshotForSpawn(env)` is the data-race story. It builds a
two-frame chain: a "globals" frame holding deep copies of the
launching goroutine's global bindings (filtered - see below), and a "locals" frame chained on top holding
deep copies of every non-global binding visible at the spawn
site. The spawn body runs against the locals frame; user-function
call frames inside the spawn parent through `effectiveGlobal` and
land on the globals frame.

```go
func (i *Interpreter) snapshotForSpawn(env *Environment, caps *parser.SpawnCaptures) *Environment {
    root := effectiveGlobal(env)  // the launching goroutine's own global frame
    globalSnap := NewEnvironment(nil)
    var want map[string]bool       // nil = copy every global
    if caps != nil && !caps.AllGlobals { want = caps.Globals }
    root.copyBindingsFilteredInto(globalSnap.vars, want) // under host.hostMu when root is the shared host global
    localSnap := NewEnvironment(globalSnap)
    for cur := env; cur != nil && cur != root; cur = cur.parent {
        cur.copyBindingsInto(localSnap.vars) // nearer frames win
    }
    return localSnap
}
```

Bindings are slot-backed, so `copyBindingsInto` / `copyBindingsFilteredInto`
rebuild the name->value view from each frame's `slots` (plus any name-map
fallback bindings) as detached deep copies; the snapshot frames themselves
are name-keyed, which is why the resolver leaves spawn bodies unresolved.

The two-frame shape is what lets user-function calls inside the
spawn keep their normal scoping. A user function's call frame
inherits from the *global* surface only - never from the
caller's locals (Jennifer's "no inheriting caller scope" model).
Inside a spawn, the call frame's parent comes from
`effectiveGlobal(env)`:

```go
func effectiveGlobal(env *Environment) *Environment {
    if env == nil { return nil }
    if env.root != nil { return env.root } // O(1) cached root
    cur := env                             // defensive walk for hand-built envs
    for cur.parent != nil { cur = cur.parent }
    return cur
}
```

In serial code `env` chains to `i.global`, so `effectiveGlobal`
returns `i.global`. In a spawn body `env` chains to the snapshot's
globals frame, so `effectiveGlobal` returns that frame. Both paths
honour the no-shadowing rule the same way (parameters never
collide with captured locals, only with true globals), and the
spawn body's user-function calls are race-free because they never
touch the live `i.global` the parent goroutine may be writing.

Deep-copy reuses the same `Value.Copy()` path as `$ys = $xs;` and
function-parameter binding, so lists, maps, bytes, and structs
copy at any depth.

The globals frame is copied **filtered**: a parse-time pass
(`computeSpawnCaptures` in `spawncapture.go`) stamps each
`SpawnExpr` with the global names its body reads - directly, and
transitively through the user functions it calls by name - and
`snapshotForSpawn` copies only those. A body that reads no global
copies none, so a program holding a large global (a cache, a config,
a per-request server worker) does not pay that copy per spawn. The
pass is sound by over-approximation: any shape it cannot bound - a
call through a func value, a module call (which can re-enter the
host), a callback builtin, or an unrecognised node - stamps
`AllGlobals`, and the snapshot copies the whole frame. It reuses the call-graph fixpoint of the borrow pass's
`computeEntryGlobalSafe`, resolving callees in a spawn body against
the function table (the resolver skips spawn bodies, so
`CallExpr.Method` is nil there). The locals frame is always copied
whole - the body may reference any of them and they are cheap. The
snapshot is still built on the launching goroutine before the body
runs, so the narrowed set stays a launch-time copy, not a live view.

The exceptions are the shared handles. A `task of T` value
copies the *pointer* to the underlying `TaskState`,
not the state - multiple variables pointing at "the same spawn"
must observe it together. A `channel of T` likewise shares its
`ChannelState` (only the values sent through it are copied), and a
`func` value or an opaque `KindObject` shares its immutable payload. Without this, `def u as task of T init $t;`
would clone the in-flight goroutine handle and break observation
counting.

### Task registry and loud-fail

The interpreter carries

```go
type Interpreter struct {
    // ...
    tasksMu sync.Mutex
    tasks   []*TaskState
}
```

`evalSpawn` calls `registerTask(state)`. Each `TaskState` carries
an `Observed bool` flag that any of the "I saw this"
operations flips: `task.wait` (both on success return and on
rethrow), `task.discard`, `task.waitAll` (drains every
survivor before re-raising), and `task.waitTimeout` once the task
finishes within the bound.

`Interpreter.UnwaitedTaskErrors()` runs at the end of CLI
execution when the program ran to its end:

```go
func (i *Interpreter) UnwaitedTaskErrors() []error {
    i.tasksMu.Lock()
    snapshot := append([]*TaskState(nil), i.tasks...)
    i.tasksMu.Unlock()
    var errs []error
    for _, t := range snapshot {
        if t == nil || t.Observed { continue }
        <-t.Done                       // happens-before edge
        if t.Err != nil { errs = append(errs, t.Err) }
    }
    return errs
}
```

It blocks on `<-t.Done` for every unobserved task, since a
non-blocking scan could miss a late-arriving error and exit cleanly.
The cost: an unobserved goroutine that never finishes (a
`spawn { while (true) {} }` without `task.discard`) hangs the
program. The user guide flags this trade-off.

A program that ended **early** - `Run` returned an `*ExitSignal` (an
`exit`) or an uncaught error - uses `FinishedTaskErrors()` instead: the
same scan without the wait, reporting only unobserved tasks that are
already done. `exit` ends the whole program, so a task blocked where no
cancellation reaches (`net.accept`, a socket read, `term.readByte`) must
not keep it alive; the process exit abandons it.

`cmd/jennifer/main.go` consumes the slice: after `Run(prog)`
returns, it walks the scan's result, prints each one to
stderr in `spawn error (unwaited): MSG` form, and bumps the
process exit code if any were present. `ExitSignal` from a body
is special-cased in the loud-fail surface (treat as a normal
program-level exit, not a "task error") so user-explicit
shutdowns don't print spurious "unwaited" lines.

### task library Go layer

`internal/lib/task` registers nine namespaced builtins through
the standard `RegisterNamespaced` path - the five observation
builtins below, plus cooperative cancellation (`cancel` /
`cancelled`) and bounded waits (`waitTimeout` / `waitAnyTimeout`):

| Builtin            | Path                                                                                          |
| ------------------ | --------------------------------------------------------------------------------------------- |
| `task.wait`        | block on `<-state.Done`; `MarkObserved`; return `Result` or wrap `Err` as runtimeError        |
| `task.poll`        | `BoolVal(state.IsDone())` via the non-blocking select on `state.Done`                         |
| `task.discard`     | `MarkObserved`; return `Null()` immediately (does not block)                                  |
| `task.waitAll`     | iterate list, wait each, mark all observed; return list-of-results or first error in order    |
| `task.waitAny`     | register a waker with every task, poll `IsDone` in list order, sleep on the waker; return the lowest done index |

`task.waitAny` / `waitAnyTimeout` and `channel.select` wait on a
dynamic number of Go channels, which a static `select { ... }` cannot
express. They do not use `reflect.Select` (TinyGo ships it as a stub
that panics) or a helper goroutine per case (each costs a full
`-stack-size` stack under `jennifer-tiny`, and a helper's channel
receive would consume a value the select might not return). Instead
`internal/interpreter/waitset.go` gives `TaskState` and `ChannelState`
a wait set: a waiter registers one buffered wake-up channel with every
source, polls each source without blocking, and only then sleeps on
the wake-up channel. A task finishing, a channel send, and a channel
close poke the registered waiters. Registering **before** polling rules
out a lost wake-up. One window remains for channels: a sender that
has poked but not yet parked on an unbuffered (or full) channel is not
receivable, so `ChannelState.Send` keeps a pending-send count and
`channel.select` briefly yields and re-polls while it is non-zero
instead of sleeping. `channel.select` rotates the channel it polls
first so a busy channel cannot starve the rest; `task.waitAny` returns
the lowest done index. Pinned by `TestSelectFanInStress` /
`TestSelectWakesOnLaterSend` (channel) and
`TestWaitAnyWakesOnLaterCompletion` (task), run under `-race`.

`MarkObserved` is a thin wrapper around setting the flag under
the registry mutex (no atomics - the field is read only by the
exit-time scan, which already takes the mutex). The pattern is
"observation = explicit consent that this task's outcome is
yours"; the loud-fail path is the only place reads happen
outside the consenting frame.

### Type stamping for `task of T`

`parser.TypeTask` joins `TypeList` / `TypeMap` / `TypeBytes` /
`TypeStruct` in the `Type.Kind` enum. `Type.Element` holds the
`T` for `task of T`; `Type.String` and `Type.Equal` handle
recursion the same way as `list of T`. `MatchesDeclared` rejects
non-task values and (when the declared element type is concrete)
walks the wrapped task's `ElemTyp` to enforce element-type
compatibility - so `def t as task of int init spawn { return "x"; };`
fails at the use site, not deep inside the spawn body.

### CLI integration

Every entry path (`main.go`'s batch path, `repl.go`, `lint`, and the
test harnesses) installs `task` with the rest of the standard library
through `stdlib.InstallAll(in)`. Only the batch path (`main.go`) runs the
loud-fail scan (`UnwaitedTaskErrors` / `FinishedTaskErrors`); the REPL
does not scan between inputs or at exit, so a REPL session observes a
spawn's error only through `task.wait` / `task.waitAll`.

### What's not there

Channels (`channel of T` + the `channel` library), cooperative
cancellation (`task.cancel` / `task.cancelled`, observed at loop
checkpoints) and bounded waits (`task.waitTimeout` /
`task.waitAnyTimeout`) all ship - see
[task.md](../libraries/task.md) and [channel.md](../libraries/channel.md).
What remains out:

- **Hard abort.** Cancellation is cooperative only: a spawn blocked
  in a syscall or a channel op (not at a loop checkpoint) cannot be
  interrupted by `task.cancel`.
- **Structured concurrency.** No automatic scope-bounded
  termination. The loud-fail registry is the lighter-weight answer.
- **Refcounted copy-on-write for `Value`.** The O(N) deep-copy
  cost of spawning over a large captured collection is a known
  cost of the value-semantics model and the same cost that hits
  `$ys = $xs;` in serial code. A refcounted copy-on-mutation
  optimisation in the `Value` runtime would help both paths; not
  scheduled.
