# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# Cooperative cancellation and bounded waits for spawned tasks.

use io;
use task;

# 1. Cancel a runaway spawn. Without cancellation an unobserved
# non-terminating `spawn` would hang the program at exit; task.cancel stops it
# at its next loop checkpoint, and discard forgets it.
def runaway as task of int init spawn {
    def n as int init 0;
    while (true) {
        $n = $n + 1;
    }
    return $n;
};
task.cancel($runaway);
task.discard($runaway);
io.printf("cancelled a runaway spawn\n");

# 2. Clean partial result: catch the raised "task cancelled" inside the body and
# return what was computed so far.
def counter as task of int init spawn {
    def n as int init 0;
    try {
        while (true) {
            $n = $n + 1;
        }
    } catch (e) {
        # cancelled - fall through with the partial count
    }
    return $n;
};
task.cancel($counter);
def partial as int init task.wait($counter);
io.printf("clean partial count computed: %t\n", $partial >= 0);

# 3. Bounded wait: a task that finishes returns its value within the timeout.
def quick as task of int init spawn {
    return 42;
};
io.printf("waitTimeout got %d\n", task.waitTimeout($quick, 2000));
