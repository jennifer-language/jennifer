#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Render a small CMS-style page with the tengine template engine: a base layout
 * pulls in a page's `{{ define }}d` content, which assigns a variable, ranges the
 * posts with an index, branches with `if` / `else if`, and uses the `default`,
 * `title`, `urlize`, and `html` pipes.
 * @module tengine_demo
 */
use io;
use json;
import "../../modules/tengine.j" as tengine;

def set as tengine.Set init tengine.newSet();
$set = tengine.add($set, "base",
    "<h1>{{ .site | title }}</h1>\n{{ template \"content\" . }}");
$set = tengine.add($set, "page",
    "{{ define \"content\" }}{{ $tagline := .tagline | default \"a small site\" }}<p>{{ $tagline }}</p>\n<ul>\n{{- range $i, $post := .posts }}\n  <li>{{ $i }}. <a href=\"/{{ $post.title | urlize }}\">{{ $post.title | html }}</a>{{ if $post.draft }} (draft){{ else if $post.featured }} *{{ end }}</li>\n{{- end }}\n</ul>{{ end }}");

def data as json.Value init json.decode(
    "{\"site\":\"my blog\",\"posts\":[" +
    "{\"title\":\"Hello & Welcome\",\"featured\":true}," +
    "{\"title\":\"Work In Progress\",\"draft\":true}," +
    "{\"title\":\"Just Another Post\"}]}");

io.printf("%s\n", tengine.render($set, "base", $data));
