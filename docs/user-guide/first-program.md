# Your first program

Save the following as `hello.j`:

```jennifer
# hello.j
use io;

def x as int init 21;
io.printf($x + $x);
```

Run it:

```sh
./jennifer run hello.j
```

You should see `42`.

## What just happened

1. `use io;` makes Jennifer's standard library functions available.
2. `def x as int init 21;` declares an integer variable named `x` and
   initializes it to `21`. Notice that **using** a variable requires the `$`
   prefix.
3. `io.printf($x + $x)` calls the standard library function with the result of
   `21 + 21`.

Top-level statements run in source order - there is no required entry-point
function. You can still group reusable code into functions (`func`) and call them
explicitly.
