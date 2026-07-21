" SPDX-License-Identifier: LGPL-3.0-only
" Copyright (C) 2026 mplx <jennifer@mplx.dev>
"
" Vim syntax file for the Jennifer programming language (.j).
" See editors/README.md for installation.

if exists("b:current_syntax")
  finish
endif

" Comments: # line, /* */ block (block comments nest in Jennifer).
syn match   jenniferComment "#.*$" contains=@Spell
syn region  jenniferBlockComment start="/\*" end="\*/" contains=jenniferBlockComment,@Spell

" Strings (single and double quoted) with escapes.
syn match   jenniferEscape contained "\\[nrt\\\"'0]"
syn region  jenniferString start=+"+ skip=+\\"+ end=+"+ contains=jenniferEscape
syn region  jenniferString start=+'+ skip=+\\'+ end=+'+ contains=jenniferEscape

" Numbers: hex, octal, binary, float, integer (with _ separators).
syn match   jenniferNumber "\<0[xX][0-9a-fA-F][0-9a-fA-F_]*\>"
syn match   jenniferNumber "\<0[oO][0-7][0-7_]*\>"
syn match   jenniferNumber "\<0[bB][01][01_]*\>"
syn match   jenniferFloat  "\<[0-9][0-9_]*\.[0-9][0-9_]*\>"
syn match   jenniferNumber "\<[0-9][0-9_]*\>"

" Keywords.
syn keyword jenniferControl if elseif else while for in repeat until break continue return exit try catch throw defer errdefer spawn
syn keyword jenniferKeyword export def const func struct use include import as of to init
syn keyword jenniferOperatorWord and or not
syn keyword jenniferBuiltin len
syn keyword jenniferType int float string bool bytes list map task
syn keyword jenniferConstant true false null

" User constants: UPPER_CASE names.
syn match   jenniferUserConstant "\<[A-Z]\+\(_[A-Z]\+\)*\>"

" Variables: $ sigil.
syn match   jenniferVariable "\$[A-Za-z]\+"

" Namespaced call: prefix.name.
syn match   jenniferNamespace "\<[A-Za-z]\+\ze\.[A-Za-z]"
syn match   jenniferFunction  "\.\zs[A-Za-z]\+\ze\s*("

" Bare method call.
syn match   jenniferFunction  "\<[A-Za-z]\+\ze\s*("

hi def link jenniferComment       Comment
hi def link jenniferBlockComment  Comment
hi def link jenniferString        String
hi def link jenniferEscape        SpecialChar
hi def link jenniferNumber        Number
hi def link jenniferFloat         Float
hi def link jenniferControl       Statement
hi def link jenniferKeyword       Keyword
hi def link jenniferOperatorWord  Operator
hi def link jenniferBuiltin       Function
hi def link jenniferType          Type
hi def link jenniferConstant      Constant
hi def link jenniferUserConstant  Constant
hi def link jenniferVariable      Identifier
hi def link jenniferNamespace     Include
hi def link jenniferFunction      Function

let b:current_syntax = "jennifer"
