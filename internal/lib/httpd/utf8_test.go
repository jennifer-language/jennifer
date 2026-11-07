// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package httpdlib

import (
	"net/http"
	"net/url"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestRequestTextInvalidUTF8: path, rawQuery, query and header refuse
// bytes that are not valid UTF-8 (a decoded %FF, raw obs-text), naming the
// function and the byte offset. The engine answers such requests 400 before
// accept, so this second line is driven with a request built directly.
// Before the fix they returned the bytes as a string.
func TestRequestTextInvalidUTF8(t *testing.T) {
	ResetForTest()
	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/a\xff", RawQuery: "y=\xfe&x=%C3"},
		Header: http.Header{"X-Test": {"ok\xe9"}},
	}
	rs := &reqState{r: r, done: make(chan struct{}), status: 200}
	id := registerReq(rs)
	defer unregisterReq(id)
	req := makeRequest(id)

	_, pathErr := pathFn(noCtx, []Value{req})
	_, rawErr := rawQueryFn(noCtx, []Value{req})
	_, queryErr := queryFn(noCtx, []Value{req, interpreter.StringVal("x")})
	_, headerErr := headerFn(noCtx, []Value{req, interpreter.StringVal("X-Test")})
	want := map[string]error{
		"httpd.path: invalid UTF-8 at byte 2":     pathErr,
		"httpd.rawQuery: invalid UTF-8 at byte 2": rawErr,
		"httpd.query: invalid UTF-8 at byte 0":    queryErr,
		"httpd.header: invalid UTF-8 at byte 2":   headerErr,
	}
	for msg, err := range want {
		if err == nil || err.Error() != msg {
			t.Errorf("want %q, got %v", msg, err)
		}
	}
}
