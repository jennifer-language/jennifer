# Rejected features

Proposals that were considered and explicitly turned down. Recorded here so
the same ideas don't come back as fresh suggestions next session.

## Increment / decrement (`++`/`--`)

Considered: postfix `$i++` and prefix `++$i`.

Rejected because:

- The pre/post distinction is a real footgun - the two forms differ only
  in expression context, which is exactly where bugs hide. Swift removed
  `++`/`--` in version 3 (2016) for this exact reason.
- The savings are tiny (three characters) and only apply to `+1` / `-1`.
- Python rejected them from the start and the language hasn't suffered.
- `$i = $i + 1;` is verbose but unambiguous; the readability cost is small.

## Compound assignment (`+=`, `-=`, `*=`, `/=`, `//=`, `%=`)

Considered as an alternative to `++`/`--`.

Rejected because:

- Several operators to add and remember for marginal ergonomic gain over
  `$x = $x + E;`.
- Slippery slope: would we also need a string-concat `+=`? An `and=`?
  Where does the family end?
- Keeping a single assignment shape (`$x = EXPR;`) makes source code uniform
  and matches Jennifer's "one way to do each thing" stance.

## printf data-transformation modifiers

Considered during the M7 format-verb-modifier design: extending the
modifier list with options that transform the *value* rather than its
*visual representation*. Examples from the original draft:

- `%s|case=upper|lower|title|snake|kebab|camel|pascal|leet`
- `%s|slice=START:END`
- `%s|md=italic|bold|code|strike|header1|header2|...|link(URL)|...`
- `%s|md=table(...)` and the wider `%a|json=*`/`%a|xml=*`/`%a|yaml=*`
  family (deferred along with the rest of `%a`)
- `null=sql` (SQL-specific spelling of `NULL`)

Rejected for the modifier system because:

- **Mission creep.** The guiding rule for printf modifiers is "shape
  the printed representation, not the value." `%d|base=2` is presentation
  (the int 5 becomes the glyph sequence `101`). `%s|case=upper` is data
  transformation (`"abc"` becomes `"ABC"` - a different string value).
  Once one transform is in, every string/number/aggregate manipulation
  becomes a candidate modifier and the format-string spec swallows the
  rest of the standard library.
- **Parallel API to the libraries.** `%s|case=upper` is already
  `upper($s)`. Two ways to do the same thing breaks Jennifer's "one
  way per thing" stance and means every future string helper has to
  decide whether to ship as a function, a modifier, or both.
- **Domain leakage.** `null=sql` picks one application domain to bake
  into the formatter. `null=literal("NULL")` is already general -
  letting the user spell their own NULL keeps SQL, CSV, JSON, and any
  future format out of the printf spec.
- **`md=*` is a separate library.** Markdown rendering belongs in a
  future `markdown` library that returns strings, so the result
  composes with `printf`, `sprintf`, string concat, file writing -
  anywhere a string goes. Folding it into `%s` modifiers would lock
  markdown output to print sites.

What lives in M7 instead: presentation-only modifiers per verb -
`pad`, `max`, `align`, `mode=raw|quote|escape` for `%s`; `pad`,
`fill`, `base`, `sign`, `group`, `sep` for `%d`; `prec`, `trim`,
`sci`, `sign`, `pad`, `align` for `%f`; `case=upper|lower|title` for
`%t` (true/false isn't really a transformation - it's the verb's
choice of rendering); a shared `null=empty|null|literal(STR)`. See
[milestones.md > M7](../milestones.md#m7---printf-modifier).

Also deferred to a later milestone (not rejected, just out of M7
scope): the `%a` aggregate verb for lists and maps and the
`null=skip` mode that only makes sense with `%a`.

## printf literal-pipe lookahead

Considered during M7 as a way to soften the breaking change to pre-M7
format strings: treat the `|` after a verb as a literal whenever the
*next* byte isn't a lowercase letter, so `"%s|%s"` would keep working
because `|%` isn't a key start.

Rejected because the rule is context-sensitive in exactly the wrong
direction. A user who writes `"%s|fill text"` (intending literal `|`)
would suddenly hit a parse error because `fill` is a valid-looking
key, while `"%s|9 lives"` would silently keep working because `9`
isn't a letter. The footgun moves around with whatever word follows
the verb.

The chosen rule is the strict one: `|` immediately after a verb
always starts a modifier list. To write a literal `|` in that
position, double it (`||`), parallel to the `%%` escape for a
literal `%`. The rule is uniform and easy to remember; the migration
cost was small (five test strings in this repo).
