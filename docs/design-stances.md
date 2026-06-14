# Design stances

The seven decisions below shape every feature in Jennifer. They are
deliberately uncompromising - "convenience" is rejected when it creates
parallel ways to do the same thing, or hides what the code does. Every
feature proposal is evaluated against these; a feature that violates a
stance needs a strong justification (and, if turned down, an entry in
[technical/rejected.md](technical/rejected.md)). A feature that ships
despite *appearing* to violate a stance gets a reasoning record in
[technical/design-decisions.md](technical/design-decisions.md).

| #   | Stance                                                   | What it rules in / out                                                                                                                                                                                                                                                                      |
| --- | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **One way per thing.**                                   | Reject sugar that creates parallel APIs (no `++`/`--`, no `+=`, no two `printf` flavors for the same job). One canonical form is easier to read than three convenient ones.                                                                                                                 |
| 2   | **Explicit over implicit.**                              | Sigils mark use-site references (`$x`), `def` carries the type, libraries are imported per topic (`use io;`; nothing auto-loads), conditions must be `bool` (no truthiness), conversions are spelled out (`convert.toInt(v)`, `convert.toFloat(v)`). Nothing important hides.               |
| 3   | **Presentation, not transformation, in format strings.** | `printf` verb modifiers shape how a value is rendered (`%d\|base=2`, `%f\|prec=4`). Transforming the value itself (`upper`, `substring`, markdown rendering) is a library call. Keeps `printf` small and orthogonal to the rest of the standard library.                                    |
| 4   | **Strict at boundaries.**                                | Undefined math, missing map keys, out-of-bounds reads, and type mismatches are positioned runtime errors. No NaN, no silent garbage.                                                                                                                                                        |
| 5   | **Value semantics for collections.**                     | Lists and maps copy on assignment and on parameter binding - no aliasing. `const` is deep: it rejects both rebinding and content mutation at any depth.                                                                                                                                     |
| 6   | **No shadowing.**                                        | A name binds once in any visible scope. Inner scopes inherit outer bindings but cannot redeclare them.                                                                                                                                                                                      |
| 7   | **Topic-based, opt-in libraries.**                       | The standard library is split by topic, never bundled. Every library is enabled explicitly with `use NAME;` - no library auto-loads.                                                                                                                                                       |
