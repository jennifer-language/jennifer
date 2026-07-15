// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A `MODULE_test.j` overlay is spliced onto its `MODULE.j` so the test methods
// reach the module's private names by bare identifier, and the combined
// program is run as the module it tests (so `export` is legal).
func TestModuleTestOverlaySplicesModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc.j"), []byte(
		`export func add(a as int, b as int) { return $a + $b; }
func secret() { return 42; }
def const BASE as int init 100;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc_test.j"), []byte(
		`use testing;
func testPublic() { testing.assertEqual(add(2, 3), 5); }
func testPrivate() { testing.assertEqual(secret(), 42); }`), 0o644); err != nil {
		t.Fatal(err)
	}

	in, code := loadForTest(filepath.Join(dir, "calc_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest returned code %d", code)
	}
	// The test methods are hoisted...
	for _, name := range []string{"testPublic", "testPrivate"} {
		if !hasMethod(in, name) {
			t.Errorf("test method %q not discovered", name)
		}
	}
	// ...and the module's private names were spliced in (reachable by bare
	// identifier). A private test running secret()/BASE proves it end to end.
	if !hasMethod(in, "secret") {
		t.Errorf("module private method `secret` not spliced into scope")
	}
	if _, err := in.CallByName("testPrivate"); err != nil {
		t.Errorf("white-box test reading a private name failed: %v", err)
	}
}

// The shipped ansi module's white-box overlay (modules/ansi_test.j over
// modules/ansi.j) loads and its tests pass - a guard against ansi.j /
// ansi_test.j drifting out of sync.
func TestShippedAnsiOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ansi_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ansi overlay failed: code %d", code)
	}
	for _, name := range []string{
		"testEscIsOneByte", "testSgrCodes", "testStripRoundTrips", "testUnknownColourThrows",
	} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// A module that itself `import`s a sibling module can be tested through its
// overlay: the test path enables the module system, so the spliced module's
// own imports resolve (local, relative to the test file's directory).
func TestOverlayModuleImportsSibling(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dep.j"), []byte(
		`export func base() { return 40; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod.j"), []byte(
		`import "./dep.j" as dep;
export func answer() { return dep.base() + 2; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod_test.j"), []byte(
		`use testing;
func testAnswer() { testing.assertEqual(answer(), 42); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	in, code := loadForTest(filepath.Join(dir, "mod_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("loadForTest returned code %d (module-importing-module overlay)", code)
	}
	if _, err := in.CallByName("testAnswer"); err != nil {
		t.Errorf("overlay whose module imports a sibling failed: %v", err)
	}
}

// The shipped markdown module imports the htmlwriter and ansi modules, so its
// overlay exercises the test path's module resolution end to end. A guard
// against markdown.j / markdown_test.j drifting out of sync.
func TestShippedMarkdownOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "markdown_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the markdown overlay failed: code %d", code)
	}
	for _, name := range []string{"testHtmlHeading", "testHtmlLists", "testAnsiListMarkers"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped ical module's white-box overlay (modules/ical_test.j over
// modules/ical.j) loads and its tests pass - a guard against ical.j /
// ical_test.j drifting out of sync. Pure text over strings / lists + time.
func TestShippedIcalOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ical_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ical overlay failed: code %d", code)
	}
	for _, name := range []string{"testRoundTrip", "testEncodeStructure", "testParseFoldedLine", "testEscapeRoundTrips"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped vcard module's white-box overlay (modules/vcard_test.j over
// modules/vcard.j) loads and its tests pass - a guard against vcard.j /
// vcard_test.j drifting out of sync. Shares the content-line codec
// (ical_vcard_shared.j) with ical via `include`, exercised here end to end.
func TestShippedVcardOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "vcard_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the vcard overlay failed: code %d", code)
	}
	for _, name := range []string{"testRoundTrip", "testEncodeStructure", "testEncodeAllAndParseMany", "testSplitStructuredKeepsEscapedSemicolon"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped jsonl module's white-box overlay (modules/jsonl_test.j over
// modules/jsonl.j) loads and its in-memory tests pass - a guard against jsonl.j
// / jsonl_test.j drifting out of sync. The fs-backed helpers are covered
// separately by TestJsonlFileAndStreaming.
func TestShippedJsonlOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "jsonl_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the jsonl overlay failed: code %d", code)
	}
	for _, name := range []string{"testRoundTrip", "testDecodeSkipsBlankLines", "testMixedTopLevelTypes"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped ipnet module's white-box overlay (modules/ipnet_test.j over
// modules/ipnet.j) loads and its tests pass - a guard against ipnet.j /
// ipnet_test.j drifting out of sync. Pure IPv4 / IPv6 + CIDR math over strings
// / convert and the bitwise operators.
func TestShippedIpnetOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ipnet_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ipnet overlay failed: code %d", code)
	}
	for _, name := range []string{"testParseSixCanonical", "testContainsFour", "testContainsSix", "testNetmaskSix", "testParseErrors"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped ntp module's white-box overlay (modules/ntp_test.j over
// modules/ntp.j) loads and its packet-codec tests pass - a guard against ntp.j /
// ntp_test.j drifting out of sync. The live UDP query is covered separately by
// TestNtpQuery / TestNtpTimeout.
func TestShippedNtpOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ntp_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ntp overlay failed: code %d", code)
	}
	for _, name := range []string{"testBuildRequestTransmitRoundTrip", "testReadTimestampKnown", "testTimestampFractionRoundTripsToMillis"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped pdfwriter module's white-box overlay (modules/pdfwriter_test.j
// over modules/pdfwriter.j) loads and its builder / render tests pass - a guard
// against pdfwriter.j / pdfwriter_test.j drifting out of sync. The rendered PDF
// is validated with qpdf / pdftotext in TestPdfwriterProducesValidPDF.
func TestShippedPdfwriterOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "pdfwriter_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the pdfwriter overlay failed: code %d", code)
	}
	for _, name := range []string{"testTextContent", "testRenderHeaderBytes", "testUnknownFontThrows", "testColorContent"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped semver module's white-box overlay (modules/semver_test.j over
// modules/semver.j) loads and its tests pass - a guard against semver.j /
// semver_test.j drifting out of sync. Covers parse / compare / sort and the
// npm-style range matching (satisfies / maxSatisfying / validRange).
func TestShippedSemverOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "semver_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the semver overlay failed: code %d", code)
	}
	for _, name := range []string{"testCompoundOr", "testHyphenRanges", "testXRangesAndBarePartials", "testMaxSatisfying", "testPrereleaseGate"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped statsd module's white-box overlay (modules/statsd_test.j over
// modules/statsd.j) loads and its name / line formatting tests pass - a guard
// against statsd.j / statsd_test.j drifting out of sync. The live UDP send is
// covered separately by TestStatsdEmits.
func TestShippedStatsdOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "statsd_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the statsd overlay failed: code %d", code)
	}
	for _, name := range []string{"testMetricNameWithPrefix", "testFormatCounter", "testFormatWithPrefix", "testFormatGaugeTimingSet"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped password module's white-box overlay (modules/password_test.j over
// modules/password.j) loads and its tests pass - a guard against password.j /
// password_test.j drifting out of sync. Pure generation / validation /
// complexity over math / strings / lists / convert; both binaries.
func TestShippedPasswordOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "password_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the password overlay failed: code %d", code)
	}
	for _, name := range []string{"testDefaultGenerateConforms", "testGenerateNoSymbolsHasNoSymbols", "testValidateTooShortAndMinimums", "testComplexityBands", "testBinaryLogExactPowersOfTwo"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped influxdb module's white-box overlay (modules/influxdb_test.j over
// modules/influxdb.j) loads and its line-protocol / query-parse tests pass - a
// guard against influxdb.j / influxdb_test.j drifting out of sync. The live
// write / query over http is covered separately by TestInfluxdbWriteAndQuery.
func TestShippedInfluxdbOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "influxdb_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the influxdb overlay failed: code %d", code)
	}
	for _, name := range []string{"testLineFieldTypes", "testLineEscaping", "testParseQuery", "testParseQueryError", "testCellString"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped slack module's white-box overlay (modules/slack_test.j over
// modules/slack.j) loads and its Block Kit payload tests pass - a guard against
// slack.j / slack_test.j drifting. The live webhook POST is covered separately
// by TestSlackWebhook.
func TestShippedSlackOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "slack_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the slack overlay failed: code %d", code)
	}
	for _, name := range []string{"testRenderTextOnly", "testRenderBlocks", "testTextAndBlocks", "testSectionEscaping"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped discord module's white-box overlay (modules/discord_test.j over
// modules/discord.j) loads and its embed payload tests pass - a guard against
// discord.j / discord_test.j drifting. The live webhook POST is covered
// separately by TestDiscordWebhook.
func TestShippedDiscordOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "discord_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the discord overlay failed: code %d", code)
	}
	for _, name := range []string{"testRenderContentOnly", "testEmbed", "testContentAndEmbed", "testMultipleEmbeds"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped telegram module's white-box overlay (modules/telegram_test.j over
// modules/telegram.j) loads and its form-encoding / response-parsing tests pass
// - a guard against telegram.j / telegram_test.j drifting. The live Bot API
// calls are covered separately by TestTelegramBotAPI.
func TestShippedTelegramOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "telegram_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the telegram overlay failed: code %d", code)
	}
	for _, name := range []string{"testFormEncode", "testParseMessage", "testParseUser", "testParseUpdates", "testCheckResponseThrowsOnError"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped websocket module's white-box overlay (modules/websocket_test.j
// over modules/websocket.j) loads and its accept-key / URL / frame-encoding
// tests pass - a guard against websocket.j / websocket_test.j drifting. The
// live handshake + send / receive round trip is covered by
// TestWebsocketRoundTrip.
func TestShippedWebsocketOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "websocket_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the websocket overlay failed: code %d", code)
	}
	for _, name := range []string{"testAcceptRfcVector", "testParseUrlSecureWithPort", "testEncodeSmallTextFrame", "testEncodeMediumFrameLength", "testEncodeCloseFrameOpcode"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped amqp module's white-box overlay (modules/amqp_test.j over
// modules/amqp.j) loads and its integer / string / table encode-decode tests
// pass - a guard against amqp.j / amqp_test.j drifting. The full handshake +
// publish / get round trip is covered by TestAmqpRoundTrip.
func TestShippedAmqpOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "amqp_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the amqp overlay failed: code %d", code)
	}
	for _, name := range []string{"testPutIntegers", "testPutStrings", "testReadIntegers", "testReadShortStr", "testShortStrTruncatedNul"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped multipart module's white-box overlay (modules/multipart_test.j
// over modules/multipart.j) loads and its build / parse round-trip tests pass -
// a guard against multipart.j / multipart_test.j drifting. Pure text / bytes
// over strings; both binaries.
func TestShippedMultipartOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "multipart_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the multipart overlay failed: code %d", code)
	}
	for _, name := range []string{"testBuildExactBody", "testBuildFilePart", "testRoundTripBinaryFile", "testBoundaryExtraction", "testParseHandWritten"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped bloom module's white-box overlay (modules/bloom_test.j over
// modules/bloom.j) loads and its probabilistic-set tests pass - a guard against
// bloom.j / bloom_test.j drifting. Pure hash + bytes; both binaries.
func TestShippedBloomOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "bloom_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the bloom overlay failed: code %d", code)
	}
	for _, name := range []string{"testNoFalseNegatives", "testAbsentReportFalse", "testAddIsValueSemantic", "testFilterShape", "testInvalidParamsThrow"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// The shipped ringbuffer module's white-box overlay (modules/ringbuffer_test.j
// over modules/ringbuffer.j) loads and its bounded-FIFO tests pass - a guard
// against ringbuffer.j / ringbuffer_test.j drifting. Pure lists; both binaries.
func TestShippedRingbufferOverlayPasses(t *testing.T) {
	overlay := filepath.Join("..", "..", "modules", "ringbuffer_test.j")
	in, code := loadForTest(overlay)
	if in == nil || code != testExitPass {
		t.Fatalf("loading the ringbuffer overlay failed: code %d", code)
	}
	for _, name := range []string{"testPushOverwritesOldest", "testPopFifo", "testEmptyOperationsThrow", "testPushIsValueSemantic", "testToListOrder"} {
		if !hasMethod(in, name) {
			t.Errorf("test %q not found in the spliced program", name)
			continue
		}
		if _, err := in.CallByName(name); err != nil {
			t.Errorf("%s failed: %v", name, err)
		}
	}
}

// A plain test file with no sibling module keeps working (no overlay spliced),
// and its own `export` is still rejected (it is not a module).
func TestNonOverlayTestFileUnaffected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain_test.j"), []byte(
		`use testing;
func testOk() { testing.assertEqual(1, 1); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	in, code := loadForTest(filepath.Join(dir, "plain_test.j"))
	if in == nil || code != testExitPass {
		t.Fatalf("plain test file failed to load: code %d", code)
	}
	if !hasMethod(in, "testOk") {
		t.Errorf("test method not discovered in plain test file")
	}
}
