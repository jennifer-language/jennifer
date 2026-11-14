" SPDX-License-Identifier: LGPL-3.0-only
" SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
"
" Vim syntax file for the Jennifer programming language (.j).
" See editors/README.md for installation.

if exists("b:current_syntax")
  finish
endif

let s:cpo_save = &cpo
set cpo&vim

" Operators. Defined before the comments: at the same position the later item
" wins, so `/*` still starts a comment rather than a `/` operator.
syn match   jenniferOperator "//\|<<\|>>\|<=\|>=\|==\|!=\|\.\.\|[-+*/%&|^~<>=]"

" Comments: # line, /* */ block (block comments nest in Jennifer), and /** */
" docblocks with @tags. `# pragma-jennifer-version:` / `# lint-disable:`
" directives are highlighted inside their comment.
syn match   jenniferComment "#.*$" contains=jenniferDirective,@Spell
syn match   jenniferDirective contained "\%(#\s*\)\@<=\%(pragma-jennifer-\l\+\|lint-disable\%(-file\)\=\)\ze:"
syn region  jenniferBlockComment matchgroup=jenniferBlockComment start="/\*" end="\*/" contains=jenniferBlockComment,@Spell
syn region  jenniferDocComment matchgroup=jenniferDocComment start="/\*\*/\@!" end="\*/" contains=jenniferBlockComment,jenniferDocTag,@Spell
syn match   jenniferDocTag contained "\s\zs@\l\+\>"

" Strings: double-quoted is cooked (escapes processed, `{expr}` interpolation
" slots highlighted as embedded code); single-quoted is raw (backslashes are
" literal, no escapes and no interpolation, ends at the next ').
syn match   jenniferBadEscape contained "\\."
syn match   jenniferEscape contained "\\\%([nrt\\\"'0{}]\|u\x\{4}\|U\x\{8}\)"
" A bare } is a lex error; a literal brace is written \}.
syn match   jenniferBadBrace contained "}"
" A slot holds one expression, which may contain strings of either kind and
" nested braces for a map literal.
syn cluster jenniferExpr contains=jenniferVariable,jenniferUserConstant,jenniferTypeName,jenniferConstant,jenniferNumber,jenniferFloat,jenniferString,jenniferRawString,jenniferKeyword,jenniferControl,jenniferOperator,jenniferOperatorWord,jenniferBuiltin,jenniferType,jenniferNamespace,jenniferFunction,jenniferMemberFunction,jenniferMemberConstant,jenniferMemberType,jenniferInterpBraces
syn region  jenniferInterpBraces contained transparent start=+{+ end=+}+ contains=@jenniferExpr
syn region  jenniferInterp contained matchgroup=jenniferInterpDelim start=+{+ end=+}+ contains=@jenniferExpr
syn region  jenniferString start=+"+ skip=+\\\\\|\\"+ end=+"+ contains=jenniferBadEscape,jenniferEscape,jenniferInterp,jenniferBadBrace
syn region  jenniferRawString start=+'+ end=+'+

" Numbers: hex, octal, binary, integer, float (with _ separators).
syn match   jenniferNumber "\<0[xX]\x\%(_\=\x\)*\>"
syn match   jenniferNumber "\<0[oO]\o\%(_\=\o\)*\>"
syn match   jenniferNumber "\<0[bB][01]\%(_\=[01]\)*\>"
syn match   jenniferNumber "\<\d\%(_\=\d\)*\>"
" After the integer form, so it wins on `1.5` and `1e10`.
syn match   jenniferFloat  "\<\d\%(_\=\d\)*\%(\.\d\%(_\=\d\)*\%([eE][+-]\=\d\+\)\=\|[eE][+-]\=\d\+\)\>"

" CamelCase struct, enum and variant names, then UPPER_CASE constants (the
" later definition wins on a name both match, such as `PI`).
syn match   jenniferTypeName "\<[A-Z][A-Za-z0-9]*\>"
syn match   jenniferUserConstant "\<[A-Z][A-Z0-9]*\%(_[A-Z][A-Z0-9]*\)*\>"

" Variables: $ sigil.
syn match   jenniferVariable "\$[A-Za-z][A-Za-z0-9]*"

" Bare function call. Defined before the keywords so `if (` and `len(` stay
" keywords.
syn match   jenniferFunction "\<[a-z][A-Za-z0-9]*\ze\s*("

" Keywords. They are matches rather than `syn keyword` so a keyword-spelled
" name stays a name: after a `.` (`strings.repeat`, `json.map`, `$e.to`), as a
" field (`to as Point`, `Written{bytes: 0}`), or as a prefix (`task.wait`).
let s:kw_pre = '\%(\%(\.\.\)\@2<=\|\.\@1<!\)\<\%('
let s:kw_post = '\)\>\%(\s*:\|\s\+as\>\|\.\a\)\@!'
function! s:Keywords(group, words) abort
  execute 'syn match ' . a:group . ' "' . s:kw_pre . join(split(a:words), '\|') . s:kw_post . '"'
endfunction
call s:Keywords('jenniferControl', 'if elseif else while for in repeat until match when break continue return exit try catch throw defer errdefer spawn')
call s:Keywords('jenniferKeyword', 'export def const func struct enum use include import as of to init')
call s:Keywords('jenniferOperatorWord', 'and or not')
call s:Keywords('jenniferBuiltin', 'len')
call s:Keywords('jenniferType', 'int float string bool bytes list map task')
call s:Keywords('jenniferConstant', 'true false null')
delfunction s:Keywords
" `channel` is a type only in `channel of T`.
syn match   jenniferType "\<channel\>\ze\s\+of\>"

" Namespaces: a library or module prefix (`io.printf`, `task.wait`), the
" library after `use` (even `use task;`), and an alias (`use strings as str;`,
" `import "x.j" as x;`). Defined after the keywords, so they win on `use task;`.
syn match   jenniferNamespace "\<[a-z][A-Za-z0-9]*\ze\.\a"
syn match   jenniferNamespace "\%(\<use\s\+\)\@<=[a-z][A-Za-z0-9]*"
syn match   jenniferNamespace "\%(\<as\s\+\)\@<=\%(\%(int\|float\|string\|bool\|bytes\|list\|map\|task\|func\)\>\)\@![a-z][A-Za-z0-9]*\ze\s*;"

" The name after a `.` (not the `..` range operator).
syn match   jenniferMemberFunction "\%(\.\.\)\@2<!\.\@1<=[A-Za-z][A-Za-z0-9_]*\ze\s*("
syn match   jenniferMemberType "\%(\.\.\)\@2<!\.\@1<=[A-Z][A-Za-z0-9]*\>"
syn match   jenniferMemberConstant "\%(\.\.\)\@2<!\.\@1<=[A-Z][A-Z0-9]*\%(_[A-Z][A-Z0-9]*\)*\>"

hi def link jenniferComment         Comment
hi def link jenniferBlockComment    Comment
hi def link jenniferDocComment      Comment
hi def link jenniferDocTag          SpecialComment
hi def link jenniferDirective       PreProc
hi def link jenniferString          String
hi def link jenniferRawString       String
hi def link jenniferEscape          SpecialChar
hi def link jenniferBadEscape       Error
hi def link jenniferBadBrace        Error
hi def link jenniferInterpDelim     Special
hi def link jenniferNumber          Number
hi def link jenniferFloat           Float
hi def link jenniferControl         Statement
hi def link jenniferKeyword         Keyword
hi def link jenniferOperator        Operator
hi def link jenniferOperatorWord    Operator
hi def link jenniferBuiltin         Function
hi def link jenniferType            Type
hi def link jenniferTypeName        Type
hi def link jenniferMemberType      Type
hi def link jenniferConstant        Constant
hi def link jenniferUserConstant    Constant
hi def link jenniferMemberConstant  Constant
hi def link jenniferVariable        Identifier
hi def link jenniferNamespace       Include
hi def link jenniferFunction        Function
hi def link jenniferMemberFunction  Function

let b:current_syntax = "jennifer"

let &cpo = s:cpo_save
unlet s:cpo_save
