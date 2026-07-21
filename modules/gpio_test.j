# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>
#
# gpio_test.j - white-box tests for gpio.j against a mock sysfs tree. Run with:
#
#     jennifer test modules/gpio_test.j
#
# setUp points JENNIFER_GPIO_BASE at a fresh temp directory (via os.setEnv), so
# the public API writes into a mock tree instead of the real /sys/class/gpio.
# gpio.j already `use`s fs / os / convert / strings, so the overlay adds testing.
use testing;

func mockBase() {
    return os.tempDir() + "/gpio_mock";
}

func setUp() {
    def dir as string init mockBase();
    if (fs.exists($dir)) {
        fs.removeAll($dir);
    }
    fs.mkdirAll($dir);
    os.setEnv("JENNIFER_GPIO_BASE", $dir);
}

func tearDown() {
    def dir as string init mockBase();
    if (fs.exists($dir)) {
        fs.removeAll($dir);
    }
    os.setEnv("JENNIFER_GPIO_BASE", "");
}

func testSetupWritesExportAndDirection() {
    setup(17, OUT);
    def dir as string init mockBase();
    testing.assertEqual(strings.trim(fs.readString($dir + "/export")), "17");
    testing.assertEqual(strings.trim(fs.readString($dir + "/gpio17/direction")), "out");
}

func testWriteReadRoundTrip() {
    setup(17, OUT);
    write(17, 1);
    testing.assertEqual(read(17), 1);
    write(17, 0);
    testing.assertEqual(read(17), 0);
}

func testReleaseWritesUnexport() {
    setup(17, OUT);
    release(17);
    def dir as string init mockBase();
    testing.assertEqual(strings.trim(fs.readString($dir + "/unexport")), "17");
}

# setupMissing points the base at a directory that does not exist.
func setupMissing() {
    os.setEnv("JENNIFER_GPIO_BASE", "/no/such/gpio/base/here");
    setup(17, OUT);
}

func testMissingBaseErrors() {
    testing.assertThrows("setupMissing", "gpio");
}

func setupSideways() { setup(17, "sideways"); }

func testInvalidDirection() {
    testing.assertThrows("setupSideways", "gpio");
}

func writeTwo() {
    setup(17, OUT);
    write(17, 2);
}

func testInvalidValue() {
    testing.assertThrows("writeTwo", "gpio");
}

# A second setup on an already-exported pin must NOT re-write `export` (that is
# EBUSY on real sysfs); it should still update the direction. The mock tree
# would happily overwrite export, so this asserts the write is actually skipped.
func testResetupSkipsExport() {
    setup(17, OUT);
    def dir as string init mockBase();
    # Overwrite the export file so a second export write is detectable.
    fs.writeString($dir + "/export", "CLEARED");
    setup(17, IN);
    testing.assertEqual(strings.trim(fs.readString($dir + "/export")), "CLEARED");
    testing.assertEqual(strings.trim(fs.readString($dir + "/gpio17/direction")), "in");
}
