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
    testing.assertEqual(bestMediaType("text/html;q=bogus, junk", ["text/html"]), "text/html");
    testing.assertEqual(bestMediaType("image/png", $both), "");
}
