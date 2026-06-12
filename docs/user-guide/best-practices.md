# Best practices

Stylistic guidance for writing Jennifer that reads well, ages well, and
fits the way the language is shaped. Each entry is a heuristic, not a
hard rule - the language won't stop you, but the rule of thumb is
there because the alternative tends to bite later.

## Follow the style guide

The single biggest readability win is uniform source style across a
codebase. When every file uses the same spacing, brace placement, and
naming, the eye learns the shape of well-formed Jennifer and starts
spotting bugs from rhythm alone - a one-off indent or a stray space
becomes a signal. The reverse is also true: every codebase that
tolerates "personal style" eventually pays for it in review friction,
merge conflicts over whitespace, and reader-time spent on the wrong
question ("is this code unusual because it does something unusual, or
just because the author indents differently?"). Pick the agreed style
once, then stop thinking about it.

Jennifer ships its style as both a written spec and an enforcement
tool: read [Style guide](style-guide.md) for the canonical rules
(spacing, braces, naming, literal layout), then run `jennifer fmt` to
make any file conform. Running `fmt` on save - or at minimum before
every commit - is the cheapest habit you can adopt; it removes style
from the list of things you and your reviewers have to think about.

## Why 4+ levels of nesting is a code smell

The flexibility that lets `list of list of int` hold any shape gets
unreadable fast as you nest deeper. Here's a four-level type holding
"per game, per player, per character, per inventory slot, the item
name":

```jennifer
def saves as list of list of list of list of string init [
    [[["sword", "shield"], ["bow"]], [["dagger"]]],
    [[["staff", "amulet"]], [[], ["potion", "rope", "torch"]]]
];

# What does this even mean?
$saves[0][1][0][0] = "axe";
```

Three problems:

1. **No semantic names for the dimensions.** Is index 2 "the character"
   or "the inventory slot"? You can't tell without going back to read
   the declaration and counting brackets.
2. **Bug-prone access.** `$saves[0][1][0][0]` is four indices that all
   look the same. Off-by-one or off-by-level errors are silent until
   the program either panics or, worse, modifies the wrong slot.
3. **Inflexible.** Adding a fifth dimension (per save slot, per
   timestamp, ...) means rewriting every access site in the program.

The standard fix is a struct or named record (see
[Structs](types-and-values.md#structs)). Other options that work
without introducing a new type:

- **Wrap access in methods**: `getItem(save, player, character, slot)`
  reads better than four bare brackets and gives you one place to fix
  a bug. Internally the function still walks the nested lists, but
  call sites are self-documenting.
- **Flatten with composite keys**: `map of string to string` keyed on
  `"save:0/player:1/char:0/slot:0"` trades index speed for name
  clarity. Better when the structure is sparse anyway.
- **Decompose into parallel simpler structures**: one list of save
  metadata, one map from save-id to inventory, etc.

As a rule of thumb: **one level is normal, two is fine, three is
uncommon, four is almost always a sign there's a missing abstraction.**
