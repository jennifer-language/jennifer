# Your first program

Save the following as `hello.j`:

```jennifer
# hello.j
use io;

def x as int init 21;
io.printf("%d\n", $x + $x);
```

Run it:

```sh
jennifer run hello.j
```

You should see `42`.

## What just happened

1. `use io;` enables the `io` library (nothing auto-loads).
2. `def x as int init 21;` declares an integer variable named `x` and
   initializes it to `21`. **Using** a variable takes the `$` prefix: `$x`.
3. `io.printf("%d\n", $x + $x)` formats `21 + 21` as a decimal integer and
   prints it.

Top-level statements run in source order; there is no required entry-point
function. Reusable code goes into functions (`func`), called explicitly.
