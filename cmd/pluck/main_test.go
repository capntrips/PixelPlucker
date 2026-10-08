package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func callMain(args []string) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}()

	os.Args = args
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	main()
}

func captureOutput(f func()) (string, string) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rout, wout, _ := os.Pipe()
	rerr, werr, _ := os.Pipe()
	os.Stdout = wout
	os.Stderr = werr

	out := make(chan string)
	err := make(chan string)
	go func() {
		var bufout bytes.Buffer
		//goland:noinspection GoUnhandledErrorResult
		io.Copy(&bufout, rout)
		out <- bufout.String()
	}()
	go func() {
		var buferr bytes.Buffer
		//goland:noinspection GoUnhandledErrorResult
		io.Copy(&buferr, rerr)
		err <- buferr.String()
	}()

	f()

	//goland:noinspection GoUnhandledErrorResult
	wout.Close()
	//goland:noinspection GoUnhandledErrorResult
	werr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	return <-out, <-err
}

func fileSha256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func TestListRemote(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "list-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip"}
		callMain(args)
	})

	expected := "grizzly-cd1a.260905.001.b1/ | Offset: 0 | Size: 0 | Compression: 0\ngrizzly-cd1a.260905.001.b1/flash-all.bat | Offset: 85 | Size: 590 | Compression: 8\ngrizzly-cd1a.260905.001.b1/pi01.ec.bin | Offset: 773 | Size: 516334 (504.232 kb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/gb00.ec.bin | Offset: 517203 | Size: 497848 (486.180 kb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/image-grizzly-cd1a.260905.001.b1.zip | Offset: 1015147 | Size: 15282113293 (14.233 gb) | Compression: 0\n- android-info.txt | Offset: 0 | Size: 131 | Compression: 8\n- fastboot-info.txt | Offset: 192 | Size: 182 | Compression: 8\n- super_empty.img | Offset: 436 | Size: 350 | Compression: 8\n- vbmeta.img | Offset: 846 | Size: 9799 (9.569 kb) | Compression: 8\n- vbmeta_vendor.img | Offset: 10700 | Size: 5698 (5.564 kb) | Compression: 8\n- vbmeta_system.img | Offset: 16460 | Size: 6114 (5.971 kb) | Compression: 8\n- pvmfw.img | Offset: 22636 | Size: 357874 (349.486 kb) | Compression: 8\n- vendor_kernel_boot.img | Offset: 380564 | Size: 4577161 (4.365 mb) | Compression: 8\n- vendor_boot.img | Offset: 4957792 | Size: 21628821 (20.627 mb) | Compression: 8\n- init_boot.img | Offset: 26586673 | Size: 2352884 (2.244 mb) | Compression: 8\n- dtbo.img | Offset: 28939615 | Size: 1924523 (1.835 mb) | Compression: 8\n- boot.img | Offset: 30864191 | Size: 17422432 (16.615 mb) | Compression: 8\n- userdata_exp.ai.img | Offset: 48286676 | Size: 6829389372 (6.360 gb) | Compression: 0\n- system.img | Offset: 6877676132 | Size: 1383034880 (1.288 gb) | Compression: 0\n- system_dlkm.img | Offset: 8260711067 | Size: 12910592 (12.312 mb) | Compression: 0\n- vendor_dlkm.img | Offset: 8273621719 | Size: 42065920 (40.117 mb) | Compression: 0\n- system_ext.img | Offset: 8315687699 | Size: 517607424 (493.629 mb) | Compression: 0\n- system_other.img | Offset: 8833295182 | Size: 148336640 (141.465 mb) | Compression: 0\n- vendor.img | Offset: 8981631883 | Size: 1110876160 (1.035 gb) | Compression: 0\n- product.img | Offset: 10092508098 | Size: 5113479168 (4.762 gb) | Compression: 0\n- abl.img | Offset: 15205987342 | Size: 1561687 (1.489 mb) | Compression: 8\n- bl31.img | Offset: 15207549081 | Size: 72151 (70.460 kb) | Compression: 8\n- cap.img | Offset: 15207621285 | Size: 31514 (30.775 kb) | Compression: 8\n- cpm.img | Offset: 15207652851 | Size: 318789 (311.317 kb) | Compression: 8\n- dbc.img | Offset: 15207971692 | Size: 124121 (121.212 kb) | Compression: 8\n- dbl.img | Offset: 15208095865 | Size: 161416 (157.633 kb) | Compression: 8\n- dram_init_0.img | Offset: 15208257333 | Size: 42026 (41.041 kb) | Compression: 8\n- dram_init_1.img | Offset: 15208299419 | Size: 42068 (41.082 kb) | Compression: 8\n- dram_init_10.img | Offset: 15208341547 | Size: 44467 (43.425 kb) | Compression: 8\n- dram_init_11.img | Offset: 15208386075 | Size: 44469 (43.427 kb) | Compression: 8\n- dram_init_2.img | Offset: 15208430605 | Size: 42029 (41.044 kb) | Compression: 8\n- dram_init_3.img | Offset: 15208472694 | Size: 42059 (41.073 kb) | Compression: 8\n- dram_init_4.img | Offset: 15208514813 | Size: 41992 (41.008 kb) | Compression: 8\n- dram_init_5.img | Offset: 15208556865 | Size: 42001 (41.017 kb) | Compression: 8\n- dram_init_6.img | Offset: 15208598926 | Size: 44496 (43.453 kb) | Compression: 8\n- dram_init_7.img | Offset: 15208643482 | Size: 44531 (43.487 kb) | Compression: 8\n- dram_init_8.img | Offset: 15208688073 | Size: 44481 (43.438 kb) | Compression: 8\n- dram_init_9.img | Offset: 15208732614 | Size: 44536 (43.492 kb) | Compression: 8\n- dram_phy.img | Offset: 15208777210 | Size: 99167 (96.843 kb) | Compression: 8\n- gc.img | Offset: 15208876434 | Size: 47468 (46.355 kb) | Compression: 8\n- gdmc.img | Offset: 15208923953 | Size: 180684 (176.449 kb) | Compression: 8\n- gsa_bl1.img | Offset: 15209104690 | Size: 35126 (34.303 kb) | Compression: 8\n- gsa_fw.img | Offset: 15209139872 | Size: 1389871 (1.325 mb) | Compression: 8\n- modem.img | Offset: 15210529798 | Size: 67831312 (64.689 mb) | Compression: 8\n- tzsw.img | Offset: 15278361164 | Size: 3747743 (3.574 mb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/radio-grizzly-a900a-mp_260716-260716-m-15880348.img | Offset: 15283128581 | Size: 67831966 (64.690 mb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/flash-base.sh | Offset: 15350960683 | Size: 636 | Compression: 8\ngrizzly-cd1a.260905.001.b1/flash-all.sh | Offset: 15350961417 | Size: 664 | Compression: 8\ngrizzly-cd1a.260905.001.b1/bootloader-grizzly-spacecraft-17.4-16238327.img | Offset: 15350962178 | Size: 11382364 (10.855 mb) | Compression: 8\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestListLocal(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "list-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip"}
		callMain(args)
	})

	expected := "grizzly-cd1a.260905.001.b1/ | Offset: 0 | Size: 0 | Compression: 0\ngrizzly-cd1a.260905.001.b1/flash-all.bat | Offset: 85 | Size: 590 | Compression: 8\ngrizzly-cd1a.260905.001.b1/pi01.ec.bin | Offset: 773 | Size: 516334 (504.232 kb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/gb00.ec.bin | Offset: 517203 | Size: 497848 (486.180 kb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/image-grizzly-cd1a.260905.001.b1.zip | Offset: 1015147 | Size: 15282113293 (14.233 gb) | Compression: 0\n- android-info.txt | Offset: 0 | Size: 131 | Compression: 8\n- fastboot-info.txt | Offset: 192 | Size: 182 | Compression: 8\n- super_empty.img | Offset: 436 | Size: 350 | Compression: 8\n- vbmeta.img | Offset: 846 | Size: 9799 (9.569 kb) | Compression: 8\n- vbmeta_vendor.img | Offset: 10700 | Size: 5698 (5.564 kb) | Compression: 8\n- vbmeta_system.img | Offset: 16460 | Size: 6114 (5.971 kb) | Compression: 8\n- pvmfw.img | Offset: 22636 | Size: 357874 (349.486 kb) | Compression: 8\n- vendor_kernel_boot.img | Offset: 380564 | Size: 4577161 (4.365 mb) | Compression: 8\n- vendor_boot.img | Offset: 4957792 | Size: 21628821 (20.627 mb) | Compression: 8\n- init_boot.img | Offset: 26586673 | Size: 2352884 (2.244 mb) | Compression: 8\n- dtbo.img | Offset: 28939615 | Size: 1924523 (1.835 mb) | Compression: 8\n- boot.img | Offset: 30864191 | Size: 17422432 (16.615 mb) | Compression: 8\n- userdata_exp.ai.img | Offset: 48286676 | Size: 6829389372 (6.360 gb) | Compression: 0\n- system.img | Offset: 6877676132 | Size: 1383034880 (1.288 gb) | Compression: 0\n- system_dlkm.img | Offset: 8260711067 | Size: 12910592 (12.312 mb) | Compression: 0\n- vendor_dlkm.img | Offset: 8273621719 | Size: 42065920 (40.117 mb) | Compression: 0\n- system_ext.img | Offset: 8315687699 | Size: 517607424 (493.629 mb) | Compression: 0\n- system_other.img | Offset: 8833295182 | Size: 148336640 (141.465 mb) | Compression: 0\n- vendor.img | Offset: 8981631883 | Size: 1110876160 (1.035 gb) | Compression: 0\n- product.img | Offset: 10092508098 | Size: 5113479168 (4.762 gb) | Compression: 0\n- abl.img | Offset: 15205987342 | Size: 1561687 (1.489 mb) | Compression: 8\n- bl31.img | Offset: 15207549081 | Size: 72151 (70.460 kb) | Compression: 8\n- cap.img | Offset: 15207621285 | Size: 31514 (30.775 kb) | Compression: 8\n- cpm.img | Offset: 15207652851 | Size: 318789 (311.317 kb) | Compression: 8\n- dbc.img | Offset: 15207971692 | Size: 124121 (121.212 kb) | Compression: 8\n- dbl.img | Offset: 15208095865 | Size: 161416 (157.633 kb) | Compression: 8\n- dram_init_0.img | Offset: 15208257333 | Size: 42026 (41.041 kb) | Compression: 8\n- dram_init_1.img | Offset: 15208299419 | Size: 42068 (41.082 kb) | Compression: 8\n- dram_init_10.img | Offset: 15208341547 | Size: 44467 (43.425 kb) | Compression: 8\n- dram_init_11.img | Offset: 15208386075 | Size: 44469 (43.427 kb) | Compression: 8\n- dram_init_2.img | Offset: 15208430605 | Size: 42029 (41.044 kb) | Compression: 8\n- dram_init_3.img | Offset: 15208472694 | Size: 42059 (41.073 kb) | Compression: 8\n- dram_init_4.img | Offset: 15208514813 | Size: 41992 (41.008 kb) | Compression: 8\n- dram_init_5.img | Offset: 15208556865 | Size: 42001 (41.017 kb) | Compression: 8\n- dram_init_6.img | Offset: 15208598926 | Size: 44496 (43.453 kb) | Compression: 8\n- dram_init_7.img | Offset: 15208643482 | Size: 44531 (43.487 kb) | Compression: 8\n- dram_init_8.img | Offset: 15208688073 | Size: 44481 (43.438 kb) | Compression: 8\n- dram_init_9.img | Offset: 15208732614 | Size: 44536 (43.492 kb) | Compression: 8\n- dram_phy.img | Offset: 15208777210 | Size: 99167 (96.843 kb) | Compression: 8\n- gc.img | Offset: 15208876434 | Size: 47468 (46.355 kb) | Compression: 8\n- gdmc.img | Offset: 15208923953 | Size: 180684 (176.449 kb) | Compression: 8\n- gsa_bl1.img | Offset: 15209104690 | Size: 35126 (34.303 kb) | Compression: 8\n- gsa_fw.img | Offset: 15209139872 | Size: 1389871 (1.325 mb) | Compression: 8\n- modem.img | Offset: 15210529798 | Size: 67831312 (64.689 mb) | Compression: 8\n- tzsw.img | Offset: 15278361164 | Size: 3747743 (3.574 mb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/radio-grizzly-a900a-mp_260716-260716-m-15880348.img | Offset: 15283128581 | Size: 67831966 (64.690 mb) | Compression: 8\ngrizzly-cd1a.260905.001.b1/flash-base.sh | Offset: 15350960683 | Size: 636 | Compression: 8\ngrizzly-cd1a.260905.001.b1/flash-all.sh | Offset: 15350961417 | Size: 664 | Compression: 8\ngrizzly-cd1a.260905.001.b1/bootloader-grizzly-spacecraft-17.4-16238327.img | Offset: 15350962178 | Size: 11382364 (10.855 mb) | Compression: 8\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractRemoteCompressedExt4(t *testing.T) {
	if os.Getenv("ALLOW_EXIT") == "1" {
		args := []string{"pluck", "passthrough-factory", "https://dl.google.com/dl/android/aosp/husky-cp3a.260905.009-factory-11774de0.zip", "system_dlkm.img", "extract-erofs", "/etc/build.prop"}
		callMain(args)
		return
	}
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "ALLOW_EXIT=1")

	_, err := cmd.Output()

	//goland:noinspection GoTypeAssertionOnErrors
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		got := string(e.Stderr)
		split := strings.Split(got, "\r\033[2K")
		got = split[len(split)-1]

		expected := "tmp file: successfully written\ntmp file: file removed\nError: partition: ext4 format is not currently supported\n"
		if got != expected {
			t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
		}
		if e.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, but got %d", e.ExitCode())
		}
	} else {
		t.Error("Expected error but got success")
	}
}

//goland:noinspection DuplicatedCode
func TestExtractErofsLocalCompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system_dlkm.img", "extract-erofs", "/etc/build.prop"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("build.prop")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "file path: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("build.prop")
	if err != nil {
		t.Errorf("Error getting sha256 hash of build.prop: %s", err)
	}
	expected = "a1e698b57f7c1a4f08631e0ad7fd207a234368e860ad058491f4393667fb09dc"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestExtractErofsLocalPartitionExt4(t *testing.T) {
	if os.Getenv("ALLOW_EXIT") == "1" {
		captureOutput(func() {
			args := []string{"pluck", "extract-factory", "../../husky-cp3a.260905.009-factory-11774de0.zip", "system_dlkm.img"}
			callMain(args)
		})
		args := []string{"pluck", "extract-erofs", "system_dlkm.img", "/etc/build.prop"}
		callMain(args)
		return
	}
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "ALLOW_EXIT=1")

	_, err := cmd.Output()

	//goland:noinspection GoTypeAssertionOnErrors
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		got := string(e.Stderr)
		expected := "Error: partition: ext4 format is not currently supported\n"
		if got != expected {
			t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
		}
		if e.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, but got %d", e.ExitCode())
		}
	} else {
		t.Error("Expected error but got success")
	}
}

//goland:noinspection DuplicatedCode
func TestExtractErofsRemoteUncompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system.img", "extract-erofs", "/system/build.prop"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("build.prop")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "file path: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("build.prop")
	if err != nil {
		t.Errorf("Error getting sha256 hash of build.prop: %s", err)
	}
	expected = "17065f6e88c44beb5d8ea25dd71ce22d84b1665828f2ced53a99d67d9857ea07"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractErofsLocalUncompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system.img", "extract-erofs", "/system/build.prop"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("build.prop")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "file path: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("build.prop")
	if err != nil {
		t.Errorf("Error getting sha256 hash of build.prop: %s", err)
	}
	expected = "17065f6e88c44beb5d8ea25dd71ce22d84b1665828f2ced53a99d67d9857ea07"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractErofsLocalPartition(t *testing.T) {
	_, got := captureOutput(func() {
		captureOutput(func() {
			args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system_dlkm.img"}
			callMain(args)
		})
		args := []string{"pluck", "extract-erofs", "system_dlkm.img", "/etc/build.prop"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("build.prop")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "file path: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("build.prop")
	if err != nil {
		t.Errorf("Error getting sha256 hash of build.prop: %s", err)
	}
	expected = "a1e698b57f7c1a4f08631e0ad7fd207a234368e860ad058491f4393667fb09dc"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestListAvbPropsLocal(t *testing.T) {
	got, _ := captureOutput(func() {
		captureOutput(func() {
			args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "vbmeta_system.img"}
			callMain(args)
		})
		args := []string{"pluck", "list-avb-props", "vbmeta_system.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("vbmeta_system.img")

	expected := "com.android.build.product.os_version=17\ncom.android.build.product.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.product.security_patch=2026-09-01\ncom.android.build.pvmfw.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system.os_version=17\ncom.android.build.system.fingerprint=google/generic_system_google/generic:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system.security_patch=2026-09-01\ncom.android.build.system_dlkm.os_version=17\ncom.android.build.system_dlkm.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system_ext.os_version=17\ncom.android.build.system_ext.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system_ext.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractFactoryRemoteCompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "extract-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "init_boot.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("init_boot.img")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "partition image: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("init_boot.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of init_boot.img: %s", err)
	}
	expected = "41f923f56203326c9337295df3ed866cf6d199747106b15f393919397bb0da87"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractFactoryLocalCompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "init_boot.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("init_boot.img")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "partition image: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("init_boot.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of init_boot.img: %s", err)
	}
	expected = "41f923f56203326c9337295df3ed866cf6d199747106b15f393919397bb0da87"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractFactoryRemoteUncompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "extract-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system_dlkm.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "partition image: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("system_dlkm.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of system_dlkm.img: %s", err)
	}
	expected = "1337dc27790a10ce6b72ca9edefe8f122586f1cc9b6b8369afdcd378b387c46c"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestExtractFactoryLocalUncompressed(t *testing.T) {
	_, got := captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "system_dlkm.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("system_dlkm.img")

	split := strings.Split(got, "\r\033[2K")
	got = split[len(split)-1]

	expected := "partition image: successfully written\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("system_dlkm.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of system_dlkm.img: %s", err)
	}
	expected = "1337dc27790a10ce6b72ca9edefe8f122586f1cc9b6b8369afdcd378b387c46c"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestAvbPropsRemoteCompressed(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "init_boot.img", "list-avb-props"}
		callMain(args)
	})

	expected := "com.android.build.init_boot.os_version=17\ncom.android.build.init_boot.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.init_boot.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestAvbPropsLocalCompressed(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "init_boot.img", "list-avb-props"}
		callMain(args)
	})

	expected := "com.android.build.init_boot.os_version=17\ncom.android.build.init_boot.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.init_boot.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestAvbPropsRemoteUncompressed(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "product.img", "list-avb-props"}
		callMain(args)
	})

	expected := "com.android.build.product.os_version=17\ncom.android.build.product.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.product.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestAvbPropsLocalUncompressed(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "product.img", "list-avb-props"}
		callMain(args)
	})

	expected := "com.android.build.product.os_version=17\ncom.android.build.product.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.product.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestAvbPropsLocalVbmeta(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "vbmeta_system.img", "list-avb-props"}
		callMain(args)
	})

	expected := "com.android.build.product.os_version=17\ncom.android.build.product.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.product.security_patch=2026-09-01\ncom.android.build.pvmfw.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system.os_version=17\ncom.android.build.system.fingerprint=google/generic_system_google/generic:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system.security_patch=2026-09-01\ncom.android.build.system_dlkm.os_version=17\ncom.android.build.system_dlkm.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system_ext.os_version=17\ncom.android.build.system_ext.fingerprint=google/grizzly/grizzly:17/CD1A.260905.001.B1/16238327:user/release-keys\ncom.android.build.system_ext.security_patch=2026-09-01\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestAvbHashVerifyLocal(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "vbmeta.img"}
		callMain(args)
		args = []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "abl.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "verify-avb-hash", "vbmeta.img", "abl", "abl.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("vbmeta.img")
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("abl.img")

	expected := "target verified\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

//goland:noinspection DuplicatedCode
func TestAvbHashVerifyLocalMissing(t *testing.T) {
	if os.Getenv("ALLOW_EXIT") == "1" {
		captureOutput(func() {
			args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "vbmeta.img"}
			callMain(args)
			args = []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "init_boot.img"}
			callMain(args)
		})
		args := []string{"pluck", "verify-avb-hash", "vbmeta.img", "init_boot", "init_boot.img"}
		callMain(args)
		return
	}
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("vbmeta.img")
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("init_boot.img")

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "ALLOW_EXIT=1")

	_, err := cmd.Output()

	//goland:noinspection GoTypeAssertionOnErrors
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		got := string(e.Stderr)
		expected := "Error: avb hash descriptor: failed to find target\n"
		if got != expected {
			t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
		}
		if e.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, but got %d", e.ExitCode())
		}
	} else {
		t.Error("Expected error but got success")
	}
}

//goland:noinspection DuplicatedCode
func TestListAvbRollbackIndexLocal(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "vbmeta.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "list-avb-rollback-index", "vbmeta.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("vbmeta.img")

	expected := "1788220800\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestListBootloaderLocal(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "bootloader.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "list-bootloader", "bootloader.img"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("bootloader.img")

	expected := "Header:\nmagic:              0x4642504b\nversion:            2\nheader size:        112\nentry header size:  104\nplatform:           mbu\npack version:       spacecraft-17.4-16238327\nslot type:          0\ndata align:         512\ntotal entries:      34\ntotal size:         22754404\n\nEntries:\noffset: 112\nEntry 1 {\n    name:       ufs\n    type:       partition data\n    product:    \n    offset:     0x1000 (4096)\n    size:       0xc5 (197)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 216\nEntry 2 {\n    name:       ufs\n    type:       partition data\n    product:    \n    offset:     0x1200 (4608)\n    size:       0xc5 (197)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 320\nEntry 3 {\n    name:       partition:0\n    type:       partition data\n    product:    default\n    offset:     0x1400 (5120)\n    size:       0x1690 (5776)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 424\nEntry 4 {\n    name:       partition:1\n    type:       partition data\n    product:    default\n    offset:     0x2c00 (11264)\n    size:       0xf20 (3872)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 528\nEntry 5 {\n    name:       partition:1\n    type:       partition data\n    product:    bms_redwood_partitions\n    offset:     0x3c00 (15360)\n    size:       0xfa8 (4008)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 632\nEntry 6 {\n    name:       partition:2\n    type:       partition data\n    product:    default\n    offset:     0x4c00 (19456)\n    size:       0xf20 (3872)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 736\nEntry 7 {\n    name:       partition:2\n    type:       partition data\n    product:    bms_redwood_partitions\n    offset:     0x5c00 (23552)\n    size:       0xfa8 (4008)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 840\nEntry 8 {\n    name:       partition:3\n    type:       partition data\n    product:    \n    offset:     0x6c00 (27648)\n    size:       0x2e8 (744)\n    slotted:    false\n    crc32:      0x00000000\n}\noffset: 944\nEntry 9 {\n    name:       dbl\n    type:       partition data\n    product:    \n    offset:     0x7000 (28672)\n    size:       0x52000 (335872)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1048\nEntry 10 {\n    name:       dram_init_0\n    type:       partition data\n    product:    \n    offset:     0x59000 (364544)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1152\nEntry 11 {\n    name:       dram_init_1\n    type:       partition data\n    product:    \n    offset:     0x84000 (540672)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1256\nEntry 12 {\n    name:       dram_init_2\n    type:       partition data\n    product:    \n    offset:     0xaf000 (716800)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1360\nEntry 13 {\n    name:       dram_init_3\n    type:       partition data\n    product:    \n    offset:     0xda000 (892928)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1464\nEntry 14 {\n    name:       dram_init_4\n    type:       partition data\n    product:    \n    offset:     0x105000 (1069056)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1568\nEntry 15 {\n    name:       dram_init_5\n    type:       partition data\n    product:    \n    offset:     0x130000 (1245184)\n    size:       0x2b000 (176128)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1672\nEntry 16 {\n    name:       dram_init_6\n    type:       partition data\n    product:    \n    offset:     0x15b000 (1421312)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1776\nEntry 17 {\n    name:       dram_init_7\n    type:       partition data\n    product:    \n    offset:     0x188000 (1605632)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1880\nEntry 18 {\n    name:       dram_init_8\n    type:       partition data\n    product:    \n    offset:     0x1b5000 (1789952)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 1984\nEntry 19 {\n    name:       dram_init_9\n    type:       partition data\n    product:    \n    offset:     0x1e2000 (1974272)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2088\nEntry 20 {\n    name:       dram_init_10\n    type:       partition data\n    product:    \n    offset:     0x20f000 (2158592)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2192\nEntry 21 {\n    name:       dram_init_11\n    type:       partition data\n    product:    \n    offset:     0x23c000 (2342912)\n    size:       0x2d000 (184320)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2296\nEntry 22 {\n    name:       dram_phy\n    type:       partition data\n    product:    \n    offset:     0x269000 (2527232)\n    size:       0x29000 (167936)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2400\nEntry 23 {\n    name:       gc\n    type:       partition data\n    product:    \n    offset:     0x292000 (2695168)\n    size:       0x22000 (139264)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2504\nEntry 24 {\n    name:       dbc\n    type:       partition data\n    product:    \n    offset:     0x2b4000 (2834432)\n    size:       0x40000 (262144)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2608\nEntry 25 {\n    name:       gsa_bl1\n    type:       partition data\n    product:    \n    offset:     0x2f4000 (3096576)\n    size:       0xc000 (49152)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2712\nEntry 26 {\n    name:       gsa_fw\n    type:       partition data\n    product:    \n    offset:     0x300000 (3145728)\n    size:       0x22e000 (2285568)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2816\nEntry 27 {\n    name:       bl31\n    type:       partition data\n    product:    \n    offset:     0x52e000 (5431296)\n    size:       0x30000 (196608)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 2920\nEntry 28 {\n    name:       tzsw\n    type:       partition data\n    product:    \n    offset:     0x55e000 (5627904)\n    size:       0x77d000 (7852032)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3024\nEntry 29 {\n    name:       abl\n    type:       partition data\n    product:    \n    offset:     0xcdb000 (13479936)\n    size:       0x347000 (3436544)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3128\nEntry 30 {\n    name:       cpm\n    type:       partition data\n    product:    \n    offset:     0x1022000 (16916480)\n    size:       0x9b000 (634880)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3232\nEntry 31 {\n    name:       gdmc\n    type:       partition data\n    product:    \n    offset:     0x10bd000 (17551360)\n    size:       0x52000 (335872)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3336\nEntry 32 {\n    name:       cap\n    type:       partition data\n    product:    \n    offset:     0x110f000 (17887232)\n    size:       0xc000 (49152)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3440\nEntry 33 {\n    name:       bms\n    type:       partition data\n    product:    bms_redwood_partitions\n    offset:     0x111b000 (17936384)\n    size:       0x5000 (20480)\n    slotted:    true\n    crc32:      0x00000000\n}\noffset: 3544\nEntry 34 {\n    name:       ufsfwupdate\n    type:       sideload\n    product:    \n    offset:     0x1120000 (17956864)\n    size:       0x493464 (4797540)\n    slotted:    false\n    crc32:      0x00000000\n}\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestExtractBootloaderLocal(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "bootloader.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "extract-bootloader", "bootloader.img", "abl"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("bootloader.img")
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("abl.img")

	expected := ""
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("abl.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of abl.img: %s", err)
	}

	expected = "3343fc8493c5c1ebcbaa43f32745f5a9265f758a2ab505eff35aaa293ec687f3"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestPassthroughExtractBootloader(t *testing.T) {
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "bootloader.img", "extract-bootloader", "abl"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("abl.img")

	expected := ""
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}

	var err error
	got, err = fileSha256("abl.img")
	if err != nil {
		t.Errorf("Error getting sha256 hash of abl.img: %s", err)
	}
	expected = "3343fc8493c5c1ebcbaa43f32745f5a9265f758a2ab505eff35aaa293ec687f3"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestPassthroughListBootloaderImageAr(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "abl.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "passthrough-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "abl.img", "list-bootloader-image-ar"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("abl.img")

	expected := "nonsec_ar=2\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestListBootloaderArLocal(t *testing.T) {
	captureOutput(func() {
		args := []string{"pluck", "extract-factory", "../../grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip", "bootloader.img"}
		callMain(args)
	})
	got, _ := captureOutput(func() {
		args := []string{"pluck", "list-bootloader-ar", "bootloader.img", "gsa_bl1"}
		callMain(args)
	})
	//goland:noinspection GoUnhandledErrorResult
	defer os.Remove("bootloader.img")

	expected := "sec_ar=2\n"
	if got != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", got, expected)
	}
}
