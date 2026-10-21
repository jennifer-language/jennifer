" SPDX-License-Identifier: LGPL-3.0-only
" SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
"
" Detect Jennifer source files by their .j extension, and extensionless
" scripts by a `#!... jennifer` shebang.

autocmd BufRead,BufNewFile *.j set filetype=jennifer
autocmd BufRead,BufNewFile * if getline(1) =~# '^#!.*\<jennifer\>' | set filetype=jennifer | endif
