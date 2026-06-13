# Design decisions

Decisions that ship in the language but look, at first glance, like
they conflict with one of Jennifer's seven design stances. Each entry
explains why the feature is *not* the kind of thing the stance was
written to reject. The negative counterpart is
[Rejected features](rejected.md): proposals that *were* turned down
because they really did clash with a stance.

When in doubt, the stances list in
[../user-guide/index.md](../user-guide/index.md) is authoritative for
users; this file is the reasoning record for maintainers.

## The `$xs[] = item;` append form

Stance #1 ("one way per thing") normally rejects sugar that creates a
parallel API. `$xs[] = item;` and `$xs = lists.push($xs, item);` do
compile to the same operation, so the form looks suspect under that
rule. It ships anyway because the three properties below set it apart
from the rejected `$i++` / `+=` family - the form is not a parallel
API, it's the index-write syntax growing one more legal position.

1. **`$xs[]` re-uses an existing operator slot; it is not a new
   operator.** `$xs[i] = item;` already targets a list position via
   the `[...]` index-write syntax. `$xs[] = item;` extends that same
   operator to one position the existing syntax didn't cover -
   "just past the end" - by passing an empty index. No new token is
   introduced. Compare `$i++`: that proposed a *new* operator (`++`)
   competing with the canonical `$i = $i + 1;`. The bracket form has
   no new token to learn, no precedence to memorize, and no parse
   rule that wouldn't exist anyway.
2. **Index-write semantics, not function-call semantics.**
   `$xs[i] = item;` mutates the binding's list in place.
   `$xs[] = item;` extends that in-place behaviour to the append
   position, where the function-call form
   (`$xs = lists.push($xs, item);`) needs an explicit reassignment to
   commit the new list back into the binding. So the bracket form
   isn't a "shortcut for `lists.push`" so much as the index-write
   syntax growing one more legal position. The two forms have
   genuinely different shapes: one is a write statement that mutates
   a binding, the other is an expression that returns a new list.
3. **Write-only; no expression-context footgun.** `$xs[]` cannot
   appear on the right-hand side of any expression - reading "the
   element just past the end" has no meaning and is rejected at
   parse time. `$i++`'s real problem was that pre/post forms differ
   only in expression context, which is where the bugs hid. `$xs[]`
   has no expression context to hide in, so the analogous footgun
   cannot exist.

What this means for `lists.push`: it stays in the language and is
canonical for any context that needs the post-append list as an
expression value (passing it into another call, chaining
transformations). The two spellings are not parallel APIs that do the
same thing in the same context; they fit different syntactic
positions - the bracket form for the in-place write statement, the
function form for the expression value. That's also why the same
argument doesn't license a `bytes.push` removal once `$b[] = byte;`
ships: any future code that needs "a new bytes value with this byte
appended" as an expression still wants the function form.

## XOR (`^`) as its own operator

Stance #1 ("one way per thing") would normally argue against shipping
an operator that's algebraically derivable from operators we already
have - XOR is `(a | b) & ~(a & b)` in terms of the other bitwise
primitives. It ships anyway because XOR is a CPU primitive with
unique algebraic properties that show up at every use site:

- **Self-inverse**: `$a ^ $a == 0`.
- **Round-trip**: `($a ^ $b) ^ $b == $a` - the canonical reversible
  transform (cheap obfuscation, parity bits, the classic in-place
  swap trick).
- **Bit-toggle**: `$flags ^ $mask` flips exactly the bits set in the
  mask, leaving the rest alone.

Forcing every XOR use site to write the three-operator composition
would be the `a - b` ≡ `a + (-b)` argument: we still ship `-` because
the composed form obscures the intent at every call site. Same logic
applies here.

## `core` exposes its names as bare globals only

`core` is the only library whose names (`len`, `JENNIFER_VERSION`)
are reachable bare; there is no `core.len` / `core.JENNIFER_VERSION`
qualified form. Stance #1 ("one way per thing") would normally argue
that "the same name shouldn't have two spellings", and at first glance
the bare-name exposure looks like the second spelling. The
configuration that ships is one-way-per-thing: only the bare form
exists.

The asymmetry is deliberate. `core` exists precisely so its names can
stay short - the polymorphic structural primitive (`len`) and the
build identity constant (`JENNIFER_VERSION`) are needed by almost
every program, so writing `core.len(...)` everywhere would be pure
ceremony for no clarity gain. Limiting the bare exposure to `core`
keeps the rule simple: bare names mean `core`, everything else lives
behind a `lib.` prefix. No future library should publish bare globals
unless it clears the same "polymorphic structural primitive that
spans types" bar - the bar is intentionally high.

## Half-open ranges

`lists.range(start, end)` is half-open: `lists.range(1, 100)` returns
`[1, 2, ..., 99]` (99 elements; 100 excluded). The English-reading
stance Jennifer has applied to syntax (`repeat ... until`, word
operators, `as` / `init` / `in`) would argue for the closed form -
"from 1 to 100" in English includes 100. We deliberately don't extend
that stance to value-generating runtime operations.

The English-reading argument applies cleanly to *syntax*, which is
read once when learning the language. `lists.range` is a *runtime
operation* whose semantics are read at every use site, and the cost
of getting it wrong is paid every time the user composes ranges,
partitions an iteration, or aligns a range with indexing. The
half-open form makes those operations easier; the closed form makes
the function name read more naturally in isolation. We pick the
operation-friendly form.

The case for half-open:

- **Index alignment.** `lists.range(0, len($xs))` yields exactly the
  valid 0-based indices for an `len($xs)`-element list. Closed
  would force `lists.range(0, len($xs) - 1)` - the off-by-one trap
  the half-open form was invented to eliminate.
- **Composability.** `lists.concat(lists.range(a, b), lists.range(b, c))`
  is exactly `lists.range(a, c)`. Partitioning a range at any point
  composes cleanly with no duplication and no `+1` adjustment.
  Closed would either duplicate `b` or require
  `lists.range(b + 1, c)` on every partition.
- **Stepping uniformity.** Half-open stepping is always "emit while
  inside the open end" with no "did the step land?" question. The
  user never has to reason about whether `end - start` divides
  evenly by `step`.
- **Consistency with the rest of the stdlib.** `lists.slice`,
  `strings.substring`, and 0-based indexing are all half-open. A
  closed `range` would make it the only exception, forcing every
  user to remember the special case.
- **CS-tradition languages all picked half-open.** Python `range`,
  Go slice indexing, Rust `..`, C++ STL iterators `[begin, end)`,
  JavaScript libraries. The "natural-syntax languages picked
  closed" framing is misleading: Ruby ships *both* (`..` closed,
  `...` half-open), Swift ships *both* (`...`, `..<`), Kotlin
  ships *both* (`..`, `until`). When you can ship only one because
  of stance #1, half-open is the more general choice - closed is
  recoverable as `lists.range(start, end + 1)`, but the composition
  and index-alignment properties of half-open are not recoverable
  from closed.

The English-reading stance still wins for syntax (it costs nothing
at runtime), but it's the wrong tie-breaker for a value-generating
operation that's about behaviour, not prose. This entry exists to
record the call for future tie-breakers: when an operation's
semantics matter at every use site, the operation-friendly form
beats the prose-friendly form.

We deliberately don't ship a closed variant (`lists.rangeInclusive`
or similar) - stance #1 rejects parallel APIs, and the closed form
is recoverable as `lists.range(start, end + 1)` when the user wants
"count 1 to N inclusive." `lists.range(end)` with a single-arg
default-start form is also not shipped (stance #2: explicit over
implicit).
