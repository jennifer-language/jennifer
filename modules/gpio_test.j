# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# gpio_test.j - white-box tests for gpio.j against a mock sysfs tree. Run with:
#
#     jennifer test modules/gpio_test.j
#
# setUp points JENNIFER_GPIO_BASE at a fresh temp directory (via os.setEnv), so
# the public API writes into a mock tree instead of the real /sys/class/gpio, and
# adds a Raspberry Pi 4 header chip at base 512 (Linux 6.2+ numbering), so pin 17
# is global 529. gpio.j already `use`s fs / os / convert / strings / time, so the
# overlay adds testing and task.
use testing;
use task;

func mockBase() {
    return os.tempDir() + "/gpio_mock";
}

# addChip creates a /sys/class/gpio/gpiochipN entry in the mock tree.
func addChip(base as int, label as string, ngpio as int) {
    def cd as string init mockBase() + "/gpiochip{$base}";
    fs.mkdirAll($cd);
    fs.writeString($cd + "/base", "{$base}\n");
    fs.writeString($cd + "/label", $label + "\n");
    fs.writeString($cd + "/ngpio", "{$ngpio}\n");
}

# freshTree empties the mock tree (no chips).
func freshTree() {
    def dir as string init mockBase();
    if (fs.exists($dir)) {
        fs.removeAll($dir);
    }
    fs.mkdirAll($dir);
}

func setUp() {
    freshTree();
    os.setEnv("JENNIFER_GPIO_BASE", mockBase());
    os.setEnv("JENNIFER_GPIO_CHIP_LABEL", "");
    addChip(512, "pinctrl-bcm2711", 58);
    addChip(570, "raspberrypi-exp-gpio", 8);
}

func tearDown() {
    def dir as string init mockBase();
    if (fs.exists($dir)) {
        fs.removeAll($dir);
    }
    os.setEnv("JENNIFER_GPIO_BASE", "");
    os.setEnv("JENNIFER_GPIO_CHIP_LABEL", "");
}

func exported() {
    return strings.trim(fs.readString(mockBase() + "/export"));
}

func testSetupWritesExportAndDirection() {
    setup(17, OUT);
    def dir as string init mockBase();
    testing.assertEqual(exported(), "529");
    testing.assertEqual(strings.trim(fs.readString($dir + "/gpio529/direction")), "out");
    # An output starts low; setup writes that 0 to wait out the udev delay.
    testing.assertEqual(strings.trim(fs.readString($dir + "/gpio529/value")), "0");
}

func testSetupInputLeavesValue() {
    setup(4, IN);
    testing.assertEqual(exported(), "516");
    testing.assertFalse(fs.exists(mockBase() + "/gpio516/value"));
}

func testWriteReadRoundTrip() {
    setup(17, OUT);
    write(17, 1);
    testing.assertEqual(strings.trim(fs.readString(mockBase() + "/gpio529/value")), "1");
    testing.assertEqual(read(17), 1);
    write(17, 0);
    testing.assertEqual(read(17), 0);
}

func testReleaseWritesUnexport() {
    setup(17, OUT);
    release(17);
    def dir as string init mockBase();
    testing.assertEqual(strings.trim(fs.readString($dir + "/unexport")), "529");
}

# Kernels before 6.2 (and Pi kernels before 6.6) put the header chip at base 0.
func testOldKernelBaseZero() {
    freshTree();
    addChip(0, "pinctrl-bcm2835", 54);
    addChip(504, "raspberrypi-exp-gpio", 8);
    setup(17, OUT);
    testing.assertEqual(exported(), "17");
    testing.assertTrue(fs.exists(mockBase() + "/gpio17/direction"));
}

# Raspberry Pi 5: the header is on the RP1 chip, after the SoC's own chips.
func testPi5Rp1() {
    freshTree();
    addChip(512, "gpio-brcmstb@107d508500", 32);
    addChip(544, "gpio-brcmstb@107d508520", 4);
    addChip(548, "gpio-brcmstb@107d517c00", 17);
    addChip(565, "gpio-brcmstb@107d517c20", 6);
    addChip(571, "pinctrl-rp1", 54);
    setup(17, OUT);
    testing.assertEqual(exported(), "588");
    release(17);
    testing.assertEqual(strings.trim(fs.readString(mockBase() + "/unexport")), "588");
}

func setupSeventeen() {
    setup(17, OUT);
}

func testAmbiguousChipsThrow() {
    addChip(600, "pinctrl-rp1", 54);
    testing.assertThrows("setupSeventeen", "gpio");
    testing.assertFalse(fs.exists(mockBase() + "/export"));
}

func testNoHeaderChipThrows() {
    freshTree();
    addChip(0, "gpio-mockup-A", 16);
    testing.assertThrows("setupSeventeen", "gpio");
}

func testUseChipByLabel() {
    useChip("raspberrypi-exp-gpio");
    setup(2, OUT);
    testing.assertEqual(exported(), "572");
}

func testUseChipMissingLabelThrows() {
    useChip("no-such-chip");
    testing.assertThrows("setupSeventeen", "gpio");
}

func testUseChipGlobal() {
    freshTree();
    useChip(GLOBAL);
    setup(17, OUT);
    testing.assertEqual(exported(), "17");
    write(17, 1);
    testing.assertEqual(read(17), 1);
}

func testAutoAfterOverride() {
    useChip(GLOBAL);
    useChip(AUTO);
    setup(17, OUT);
    testing.assertEqual(exported(), "529");
}

func setupLastPlusOne() {
    setup(58, OUT);
}

func testPinPastChipThrows() {
    setup(57, OUT);
    testing.assertEqual(exported(), "569");
    testing.assertThrows("setupLastPlusOne", "gpio");
}

func testMalformedChipBaseThrows() {
    fs.writeString(mockBase() + "/gpiochip512/base", "five hundred\n");
    testing.assertThrows("setupSeventeen", "gpio");
}

# A value file that is not writable yet (udev still applying permissions) is
# retried; here a task turns the blocking directory into a file after 200 ms.
func testSetupWaitsForValue() {
    def pd as string init mockBase() + "/gpio529";
    fs.mkdirAll($pd + "/value");
    def fixer as task of bool init spawn {
        time.sleep(time.fromMilliseconds(200));
        fs.removeAll($pd + "/value");
        fs.writeString($pd + "/value", "0");
        return true;
    };
    setup(17, OUT);
    task.wait($fixer);
    testing.assertEqual(strings.trim(fs.readString($pd + "/value")), "0");
}

func testSetupGivesUpAfterAboutOneSecond() {
    fs.mkdirAll(mockBase() + "/gpio529/value");
    def t0 as time.Time init time.now();
    testing.assertThrows("setupSeventeen", "gpio");
    def took as int init time.milliseconds(time.sub(time.now(), $t0));
    testing.assertTrue($took >= 800);
    testing.assertTrue($took < 5000);
}

# setupMissing points the base at a directory that does not exist.
func setupMissing() {
    os.setEnv("JENNIFER_GPIO_BASE", "/no/such/gpio/base/here");
    setup(17, OUT);
}

func testMissingBaseErrors() {
    testing.assertThrows("setupMissing", "gpio");
}

func setupSideways() {
    setup(17, "sideways");
}

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
    testing.assertEqual(strings.trim(fs.readString($dir + "/gpio529/direction")), "in");
}

func negativePin() {
    write(-1, 1);
}
func testNegativePinRejected() {
    testing.assertThrows("negativePin", "gpio");
}
