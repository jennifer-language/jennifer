// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>
//
// main.j - demonstrates file imports.
// `import greetinglib.j;` splices the contents of greetinglib.j at this point,
// making $name and $greeting available to the surrounding scope.
import stdlib;

def app() {
    import greetinglib.j;
    printf($greeting);
    printf($name);
    printf("!\n");
}
