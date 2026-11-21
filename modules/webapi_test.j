# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# webapi_test.j - white-box tests for webapi.j. Run with:
#
#     jennifer test modules/webapi_test.j
#
# The overlay splices webapi.j in front of this file, so the tests reach its
# private matchSegs / findRoute / requestData and its structs by bare identifier.
# Everything here is the pure core (Spec evaluation, routing, discovery, the
# builder) - no server. The engine-facing guard / envelopes / pagination are
# exercised end-to-end in the Go suite (TestWebapi).
use testing;

# A convenient authenticated identity with the given scopes.
func idWith(scopes as list of string) {
    return Identity{ok: true, subject: "u1", display: "user one", scopes: $scopes};
}

# Dummy handler func values to register on routes. These pure tests only build
# and match the route table, never invoke a handler, so a param-less body is
# enough (arity is checked at the call site, not at registration).
func getDeck() {
    return;
}
func publish() {
    return;
}

# --- Spec evaluation (the pure core) ----------------------------------------

func testEvaluatePublicProceeds() {
    def d as map of string to string init {};
    def none as Identity;
    def dec as Decision init evaluate(public(), $none, $d);
    testing.assertTrue($dec.proceed);
}

func testEvaluateBearerWithoutIdentityIs401() {
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.Bearer,
        scopes: [],
        rules: {},
        rateLimit: 0,
        produces: Produces.Json
    };
    def none as Identity;
    def d as map of string to string init {};
    def dec as Decision init evaluate($spec, $none, $d);
    testing.assertFalse($dec.proceed);
    testing.assertEqual($dec.status, 401);
}

func testEvaluateBearerWithIdentityProceeds() {
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.Bearer,
        scopes: [],
        rules: {},
        rateLimit: 0,
        produces: Produces.Json
    };
    def d as map of string to string init {};
    def dec as Decision init evaluate($spec, idWith([]), $d);
    testing.assertTrue($dec.proceed);
}

func testEvaluateScopeShortfallIs403() {
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.Bearer,
        scopes: ["publish"],
        rules: {},
        rateLimit: 0,
        produces: Produces.Json
    };
    def d as map of string to string init {};
    def dec as Decision init evaluate($spec, idWith(["read"]), $d);
    testing.assertFalse($dec.proceed);
    testing.assertEqual($dec.status, 403);
    testing.assertTrue(strings.contains($dec.message, "publish"));
}

func testEvaluateScopePresentProceeds() {
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.Bearer,
        scopes: ["publish"],
        rules: {},
        rateLimit: 0,
        produces: Produces.Json
    };
    def d as map of string to string init {};
    def dec as Decision init evaluate($spec, idWith(["read", "publish"]), $d);
    testing.assertTrue($dec.proceed);
}

func testEvaluateValidationFailsIs422() {
    def rules as map of string to list of validate.Rule init {
        "tag": [validate.required(), validate.maxLen(4)]
    };
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.None,
        scopes: [],
        rules: $rules,
        rateLimit: 0,
        produces: Produces.Json
    };
    def none as Identity;
    def bad as map of string to string init {"tag": "waytoolong"};
    def dec as Decision init evaluate($spec, $none, $bad);
    testing.assertFalse($dec.proceed);
    testing.assertEqual($dec.status, 422);
    testing.assertTrue(len($dec.failures) > 0);
    testing.assertEqual($dec.failures[0].field, "tag");
}

func testEvaluateValidationPassesProceeds() {
    def rules as map of string to list of validate.Rule init {"tag": [validate.required()]};
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.None,
        scopes: [],
        rules: $rules,
        rateLimit: 0,
        produces: Produces.Json
    };
    def none as Identity;
    def ok as map of string to string init {"tag": "v1"};
    def dec as Decision init evaluate($spec, $none, $ok);
    testing.assertTrue($dec.proceed);
}

func testEvaluateAuthCheckedBeforeValidation() {
    # A Bearer route with a bad body and no identity answers 401 (auth first),
    # not 422 - so an unauthenticated caller learns nothing about the body shape.
    def rules as map of string to list of validate.Rule init {"tag": [validate.required()]};
    def spec as Spec init Spec{
        summary: "",
        auth: Auth.Bearer,
        scopes: [],
        rules: $rules,
        rateLimit: 0,
        produces: Produces.Json
    };
    def none as Identity;
    def empty as map of string to string init {};
    def dec as Decision init evaluate($spec, $none, $empty);
    testing.assertEqual($dec.status, 401);
}

# --- routing / matching -----------------------------------------------------

func testMatchSegsStatic() {
    def p as map of string to string init matchSegs(["v1", "deck"], ["v1", "deck"]);
    testing.assertFalse(maps.has($p, "__nomatch"));
}

func testMatchSegsParamCapture() {
    def p as map of string to string init matchSegs(["users", ":id"], ["users", "7"]);
    testing.assertFalse(maps.has($p, "__nomatch"));
    testing.assertEqual($p["id"], "7");
}

func testMatchSegsWildcard() {
    def p as map of string to string init matchSegs(["files", "*path"], ["files", "a", "b"]);
    testing.assertFalse(maps.has($p, "__nomatch"));
    testing.assertEqual($p["path"], "a/b");
}

func testMatchSegsMismatch() {
    def p as map of string to string init matchSegs(["v1", "deck"], ["v1", "other"]);
    testing.assertTrue(maps.has($p, "__nomatch"));
}

func testMatchSegsLengthMismatch() {
    def p as map of string to string init matchSegs(["v1", "deck"], ["v1"]);
    testing.assertTrue(maps.has($p, "__nomatch"));
}

func buildApi() {
    def a as Api init new();
    $a = mount($a, 1, "/v1");
    $a = alias($a, 1);
    $a = get($a, "/deck", getDeck, public());
    $a = post(
        $a,
        "/publish",
        publish,
        Spec{
            summary: "publish a deck",
            auth: Auth.Bearer,
            scopes: ["publish"],
            rules: {},
            rateLimit: 30,
            produces: Produces.Json
        });
    return $a;
}

func testFindRouteUnderVersion() {
    def m as Matched init findRoute(buildApi(), "GET", "/v1/deck");
    testing.assertTrue($m.found);
}

func testFindRouteUnderRootAlias() {
    def m as Matched init findRoute(buildApi(), "GET", "/deck");
    testing.assertTrue($m.found);
}

func testFindRouteCarriesSpec() {
    def m as Matched init findRoute(buildApi(), "POST", "/v1/publish");
    testing.assertTrue($m.found);
    testing.assertEqual($m.spec.rateLimit, 30);
    testing.assertTrue(lists.contains($m.spec.scopes, "publish"));
}

func testFindRouteMethodMiss() {
    def m as Matched init findRoute(buildApi(), "DELETE", "/v1/deck");
    testing.assertFalse($m.found);
}

# The guard's table holds every mount and route pair, split once, and finds
# what joining and splitting each pattern per request found.
func testRouteTableMatchesTheSlowPath() {
    def a as Api init buildApi();
    testing.assertEqual(len($a.table), len($a.mounts) * len($a.routes));
    def slow as Api init $a;
    $slow.table = [];
    $slow = mount($slow, 9, "/v9");
    $slow.table = [];
    def fast as Api init mount($a, 9, "/v9");
    for (def method in ["GET", "POST", "DELETE", "HEAD"]) {
        for (def path in ["/v1/deck", "/deck", "/v1/publish", "/v9/deck", "/v1/nothing", "/"]) {
            def f as Matched init findRoute($fast, $method, $path);
            def s as Matched init findRoute($slow, $method, $path);
            testing.assertEqual($f.found, $s.found);
            testing.assertEqual($f.feature, $s.feature);
            testing.assertEqual($f.params, $s.params);
            testing.assertEqual($f.mount, $s.mount);
        }
    }
}

func testFindRoutePathMiss() {
    def m as Matched init findRoute(buildApi(), "GET", "/v1/nothing");
    testing.assertFalse($m.found);
}

# --- builder ----------------------------------------------------------------

func testPublicIsZeroSpec() {
    def s as Spec init public();
    match ($s.auth) {
        when None { testing.assertTrue(true); }
        when Bearer { testing.assertTrue(false); }
    }
    testing.assertEqual(len($s.scopes), 0);
    testing.assertEqual($s.rateLimit, 0);
}

func testFeatureDefaultsToMethodPath() {
    def a as Api init new();
    $a = get($a, "/deck", getDeck, public());
    testing.assertEqual($a.routes[0].feature, "GET /deck");
}

func testFeatureOverride() {
    def a as Api init new();
    $a = get($a, "/deck", getDeck, public());
    $a = feature($a, "deck-lookup");
    testing.assertEqual($a.routes[0].feature, "deck-lookup");
}

func testFeatureWithoutRouteThrows() {
    def a as Api init new();
    def threw as bool init false;
    try {
        feature($a, "x");
    } catch (e) {
        $threw = true;
    }
    testing.assertTrue($threw);
}

func testDeprecateMarksSunset() {
    def a as Api init new();
    $a = mount($a, 2, "/v2");
    $a = deprecate($a, 2, "2027-01-01");
    testing.assertEqual($a.mounts[0].sunset, "2027-01-01");
}

# Deprecation / Sunset dates: YYYY-MM-DD is midnight UTC, RFC 3339 keeps its
# offset; anything else is refused when the Api is built, not per request.
func testDateSeconds() {
    testing.assertEqual(dateSeconds("2026-01-01"), 1767225600);
    testing.assertEqual(dateSeconds("2027-06-30T12:00:00+02:00"), 1814349600);
}

func deprecateMalformed() {
    deprecate(mount(new(), 1, "/v1"), 1, "30.06.2027");
}

func deprecateImpossibleDay() {
    deprecatedSince(mount(new(), 1, "/v1"), 1, "2027-02-30");
}

# RFC 9745 section 4: the sunset must not be earlier than the deprecation.
func sunsetBeforeDeprecation() {
    def a as Api init deprecatedSince(mount(new(), 1, "/v1"), 1, "2027-01-01");
    deprecate($a, 1, "2026-12-31");
}

func testDeprecateRejectsBadDates() {
    testing.assertThrows("deprecateMalformed", "webapi");
    testing.assertThrows("deprecateImpossibleDay", "webapi");
    testing.assertThrows("sunsetBeforeDeprecation", "webapi");
}

func testDeprecatedSinceMarksMountsOfThatVersion() {
    def a as Api init new();
    $a = mount($a, 1, "/v1");
    $a = alias($a, 1);
    $a = mount($a, 2, "/v2");
    $a = deprecatedSince($a, 1, "2026-01-01");
    $a = deprecate($a, 1, "2027-01-01");
    testing.assertEqual($a.mounts[0].since, "2026-01-01");
    testing.assertEqual($a.mounts[1].since, "2026-01-01");
    testing.assertEqual($a.mounts[1].sunset, "2027-01-01");
    testing.assertEqual($a.mounts[2].since, "");
    testing.assertEqual($a.mounts[2].sunset, "");
}

# guard sets the Deprecation / Sunset headers from the mount the request
# matched, and runs the matched route's handler: findRoute carries both.
func testFindRouteCarriesMountAndHandler() {
    def a as Api init buildApi();
    $a = mount($a, 0, "/v0");
    $a = deprecate($a, 0, "2027-01-01");
    def old as Matched init findRoute($a, "GET", "/v0/deck");
    testing.assertTrue($old.found);
    testing.assertEqual($old.mount.sunset, "2027-01-01");
    def cur as Matched init findRoute($a, "GET", "/v1/deck");
    testing.assertEqual($cur.mount.sunset, "");
    testing.assertEqual($cur.mount.path, "/v1");
}

func testIdentityJsonRoundTrip() {
    def doc as json.Value init json.set(json.map(), "/ok", true);
    $doc = json.set($doc, "/subject", "u1");
    $doc = json.set($doc, "/display", "user one");
    $doc = json.set($doc, "/scopes", json.list(["read", "publish"]));
    def who as Identity init identityFromJson(json.decode(json.encode($doc)));
    testing.assertEqual($who, idWith(["read", "publish"]));
}

# --- discovery --------------------------------------------------------------

func testDiscoveryReflectsRoutesAndMounts() {
    def doc as json.Value init discovery(buildApi(), "jennifer-registry", "1.0");
    def s as string init json.encode($doc);
    testing.assertTrue(strings.contains($s, "jennifer-registry"));
    testing.assertTrue(strings.contains($s, "/v1"));
    testing.assertTrue(strings.contains($s, "GET /deck")); # a default feature label
    testing.assertTrue(strings.contains($s, "publish")); # from "POST /publish"
}

func testDiscoveryMarksDeprecation() {
    def a as Api init new();
    $a = mount($a, 1, "/v1");
    $a = deprecate($a, 1, "2027-06-30");
    def s as string init json.encode(discovery($a, "reg", "1.0"));
    testing.assertTrue(strings.contains($s, "2027-06-30"));
    testing.assertTrue(strings.contains($s, "deprecated"));
}

func testDiscoveryListsDeprecatedSince() {
    def a as Api init new();
    $a = mount($a, 1, "/v1");
    $a = deprecatedSince($a, 1, "2026-01-01");
    def doc as json.Value init discovery($a, "reg", "1.0");
    testing.assertTrue(json.asBool($doc, "/apis/0/deprecated"));
    testing.assertEqual(json.asString($doc, "/apis/0/deprecatedSince"), "2026-01-01");
    testing.assertFalse(json.has($doc, "/apis/0/sunset"));
}

# --- small helpers ----------------------------------------------------------

func testIsDigits() {
    testing.assertTrue(isDigits("123"));
    testing.assertFalse(isDigits(""));
    testing.assertFalse(isDigits("12a"));
    testing.assertFalse(isDigits("-1"));
    # ROBUSTNESS: an 18-digit value fits in int64 and is accepted; a 19+-digit
    # value is rejected (treated as absent) so page() never calls convert.toInt on
    # a string that would overflow and turn an oversized limit/offset into a 500.
    testing.assertTrue(isDigits("999999999999999999")); # 18 digits, fits int64
    testing.assertFalse(isDigits("99999999999999999999")); # 20 digits, would overflow
}

# A malformed or out-of-range paging value is a failure (guard answers it with
# a 400), never a silent default: "abc", "-1", "0" and a 19-digit offset used
# to become the default limit or offset 0.
func testPageFailures() {
    testing.assertEqual(len(pageFailures("", "")), 0);
    testing.assertEqual(len(pageFailures("1", "0")), 0);
    testing.assertEqual(len(pageFailures("999", "999999999999999999")), 0);
    for (def bad in ["abc", "-1", "0", "+5", " 5", "1.5", "9999999999999999999"]) {
        def f as list of validate.Failure init pageFailures($bad, "");
        testing.assertEqual(len($f), 1);
        testing.assertEqual($f[0].field, "limit");
    }
    for (def bad in ["abc", "-1", "+0", "9999999999999999999"]) {
        def f as list of validate.Failure init pageFailures("", $bad);
        testing.assertEqual(len($f), 1);
        testing.assertEqual($f[0].field, "offset");
    }
    testing.assertEqual(len(pageFailures("0", "x")), 2);
}

# addressKey: IPv4 as is, IPv6 grouped by /64, an IPv4-mapped address unmapped
# (never lumped into ::/64 with every other mapped client), unparseable as is.
func testAddressKey() {
    testing.assertEqual(addressKey("192.0.2.7"), "192.0.2.7");
    testing.assertEqual(addressKey("2001:db8:1:2:aaaa:bbbb:cccc:dddd"), "2001:db8:1:2::/64");
    testing.assertEqual(addressKey("2001:db8:1:2::9"), "2001:db8:1:2::/64");
    testing.assertEqual(addressKey("2001:db8:1:3::9"), "2001:db8:1:3::/64");
    testing.assertEqual(addressKey("::ffff:192.0.2.7"), "192.0.2.7");
    testing.assertEqual(addressKey("fe80::1%eth0"), "fe80::1%eth0");
    testing.assertEqual(addressKey("not-an-ip"), "not-an-ip");
}

# --- content negotiation ---

func testBestMediaTypeHonoursQuality() {
    def both as list of string init ["application/json", "text/html"];
    testing.assertEqual(
        bestMediaType("application/json, text/html;q=0.1", $both),
        "application/json");
    testing.assertEqual(bestMediaType("text/html;q=0", $both), "");
    testing.assertEqual(bestMediaType("text/html", $both), "text/html");
    testing.assertEqual(
        bestMediaType("text/html;q=0.5, application/json;q=0.4", $both),
        "text/html");
}

# Only the first MAX_ACCEPT_RANGES ranges and MAX_ACCEPT_PARAMS pieces of
# each are read, so a header of separators costs no more than a short one.
func testAcceptIsReadBounded() {
    testing.assertEqual(
        len(parseAccept(strings.replicate("text/html,", 30000))),
        MAX_ACCEPT_RANGES);
    def params as string init "text/html" + strings.replicate(";a=b", 30000) + ";q=0";
    testing.assertEqual(bestMediaType($params, ["text/html"]), "text/html");
    testing.assertEqual(bestMediaType(strings.replicate(",", 60000), ["text/html"]), "");
    testing.assertEqual(firstPieces("a,b,c", ",", 2), ["a", "b"]);
    testing.assertEqual(firstPieces("a,b", ",", 5), ["a", "b"]);
    testing.assertEqual(firstPieces("", ",", 3), [""]);
}

func testBestMediaTypeTieAndWildcards() {
    def both as list of string init ["application/json", "text/html"];
    testing.assertEqual(bestMediaType("", $both), "application/json"); # absent: first offered
    testing.assertEqual(bestMediaType("*/*", $both), "application/json"); # tie: first offered
    # A browser's header: html exact at 1, json only through */* at 0.8.
    def browser as string init "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8";
    testing.assertEqual(bestMediaType($browser, $both), "text/html");
    # The most specific range decides: text/* at 1 but text/html excluded.
    testing.assertEqual(
        bestMediaType("text/*, text/html;q=0", ["text/html", "text/plain"]),
        "text/plain");
    testing.assertEqual(bestMediaType("TEXT/HTML; Q=0.7", ["text/html"]), "text/html");
    testing.assertEqual(bestMediaType("text/html;q=bogus, junk", ["text/html"]), "");
    testing.assertEqual(bestMediaType("image/png", $both), "");
}

# A q outside RFC 9110's qvalue (0..1, at most three decimals) ignores its
# range: q=2 cannot outrank a real q=1, nor q=-1 exclude a type.
func testQualityOutsideZeroOneIgnoresTheRange() {
    def both as list of string init ["application/json", "text/html"];
    testing.assertEqual(bestMediaType("application/json;q=2, text/html;q=0.5", $both), "text/html");
    testing.assertEqual(bestMediaType("text/html;q=-1, */*;q=0.1", ["text/html"]), "text/html");
    testing.assertEqual(
        bestMediaType("text/html;q=0.1234, application/json;q=0.001", $both),
        "application/json");
    testing.assertEqual(bestMediaType("text/html;q=1.000", ["text/html"]), "text/html");
    testing.assertEqual(parseQuality("0.125"), 0.125);
    testing.assertEqual(parseQuality("1"), 1.0);
    testing.assertEqual(parseQuality("0."), 0.0);
    testing.assertEqual(parseQuality("1.5"), -1.0);
    testing.assertEqual(parseQuality("1e0"), -1.0);
}

# web serves a GET route for HEAD, so guard must find (and check) the GET
# route's spec for a HEAD request, or the handler runs unguarded.
func testFindRouteHeadUsesGetSpec() {
    def m as Matched init findRoute(buildApi(), "HEAD", "/v1/deck");
    testing.assertTrue($m.found);
    testing.assertEqual($m.spec, findRoute(buildApi(), "GET", "/v1/deck").spec);
}
