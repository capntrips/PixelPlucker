package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type radioTestEntry struct {
	name   string
	data   []byte
	method uint16
}

func radioTestZip(t *testing.T, entries ...radioTestEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range entries {
		f, err := w.CreateHeader(&zip.FileHeader{Name: entry.name, Method: entry.method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func extractRadioTestZip(t *testing.T, data []byte, selector string) error {
	return runRadioTestZip(t, data, selector, "extract-factory", "")
}

func runRadioTestZip(t *testing.T, data []byte, selector, command, subcommand string) error {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "factory.zip")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return findAndReadCentralDirectory(&LocalImage{file: f}, selector, command, subcommand, nil, 0, uint64(len(data)))
}

func TestRadioAliasPreservesFilename(t *testing.T) {
	const radio = "radio-kodiak_beta-a900a-qpr_260908-260909-m-16298423.img"
	payload := bytes.Repeat([]byte("radio payload"), 32)
	nested := radioTestZip(t,
		radioTestEntry{"modem.img", []byte("different modem payload"), zip.Deflate},
		radioTestEntry{radio, []byte("nested decoy"), zip.Deflate},
	)
	for _, tc := range []struct {
		name   string
		prefix string
		method uint16
	}{
		{"root-stored", "", zip.Store},
		{"root-deflated", "", zip.Deflate},
		{"directory-stored", "factory/", zip.Store},
		{"directory-deflated", "factory/", zip.Deflate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			data := radioTestZip(t,
				radioTestEntry{"factory/image-test.zip", nested, zip.Store},
				radioTestEntry{tc.prefix + radio + ".bak", []byte("decoy"), tc.method},
				radioTestEntry{tc.prefix + radio, payload, tc.method},
			)
			_, stderr := captureOutput(func() {
				if err := extractRadioTestZip(t, data, "radio.img"); !errors.Is(err, ErrCommandComplete) {
					t.Error(err)
				}
			})
			if !strings.Contains(stderr, "partition image: saved to "+radio+"\n") {
				t.Fatalf("missing resolved filename in output: %q", stderr)
			}
			got, err := os.ReadFile(radio)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("extracted radio: data=%q, error=%v", got, err)
			}
			for _, unwanted := range []string{"radio.img", "modem.img", radio + ".bak"} {
				if _, err := os.Stat(unwanted); !os.IsNotExist(err) {
					t.Fatalf("unexpected output %s: %v", unwanted, err)
				}
			}
		})
	}
}

func TestRadioAliasPassthrough(t *testing.T) {
	var payload bytes.Buffer
	header := AvbVBMetaImageHeader{Magic: [4]byte{'A', 'V', 'B', '0'}, RollbackIndex: 7}
	if err := binary.Write(&payload, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		t.Run(map[uint16]string{zip.Store: "stored", zip.Deflate: "deflated"}[method], func(t *testing.T) {
			t.Chdir(t.TempDir())
			data := radioTestZip(t, radioTestEntry{"factory/radio-test-version.img", payload.Bytes(), method})
			stdout, _ := captureOutput(func() {
				if err := runRadioTestZip(t, data, "radio.img", "passthrough-factory", "list-avb-rollback-index"); !errors.Is(err, ErrCommandComplete) {
					t.Error(err)
				}
			})
			if strings.TrimSpace(stdout) != "7" {
				t.Fatalf("passthrough output = %q, want 7", stdout)
			}
			files, err := os.ReadDir(".")
			if err != nil || len(files) != 0 {
				t.Fatalf("passthrough left output files: %v, %v", files, err)
			}
		})
	}
}

func TestRadioAliasMissingImage(t *testing.T) {
	t.Chdir(t.TempDir())
	nested := radioTestZip(t, radioTestEntry{"modem.img", bytes.Repeat([]byte("modem"), 32), zip.Store})
	data := radioTestZip(t, radioTestEntry{"factory/image-test.zip", nested, zip.Store})
	for _, command := range []string{"extract-factory", "passthrough-factory"} {
		err := runRadioTestZip(t, data, "radio.img", command, "list-avb-rollback-index")
		if err == nil || err.Error() != `partition image "radio.img" not found in factory archive` {
			t.Fatalf("%s: expected missing radio error, got %v", command, err)
		}
	}
}

func TestRadioAliasDoesNotApplyToExtractZip(t *testing.T) {
	t.Chdir(t.TempDir())
	payload := bytes.Repeat([]byte("literal radio filename"), 32)
	data := radioTestZip(t,
		radioTestEntry{"radio-test-version.img", []byte("versioned image"), zip.Deflate},
		radioTestEntry{"radio.img", payload, zip.Deflate},
	)
	if err := runRadioTestZip(t, data, "radio.img", "extract-zip", ""); !errors.Is(err, ErrCommandComplete) {
		t.Fatal(err)
	}
	if got, err := os.ReadFile("radio.img"); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("literal filename: data=%q, error=%v", got, err)
	}
	if _, err := os.Stat("radio-test-version.img"); !os.IsNotExist(err) {
		t.Fatalf("extract-zip used radio alias: %v", err)
	}
	versionedOnly := radioTestZip(t, radioTestEntry{"radio-test-version.img", payload, zip.Store})
	if err := runRadioTestZip(t, versionedOnly, "radio.img", "extract-zip", ""); err == nil || err.Error() != `file "radio.img" not found in zip archive` {
		t.Fatalf("expected missing literal file, got %v", err)
	}
}

func TestRadioAliasChecksumCleanup(t *testing.T) {
	t.Chdir(t.TempDir())
	const radio = "radio-test-version.img"
	data := radioTestZip(t, radioTestEntry{"factory/" + radio, bytes.Repeat([]byte("payload"), 32), zip.Deflate})
	cd := bytes.Index(data, []byte{'P', 'K', 1, 2})
	if cd < 0 {
		t.Fatal("missing central directory")
	}
	data[cd+16] ^= 1 // Corrupt the central directory CRC, leaving the payload intact.
	if err := os.WriteFile("radio.img", []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	err := extractRadioTestZip(t, data, "radio.img")
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	if _, err := os.Stat(radio); !os.IsNotExist(err) {
		t.Fatalf("corrupt output was not removed: %v", err)
	}
	if got, err := os.ReadFile("radio.img"); err != nil || string(got) != "existing file" {
		t.Fatalf("unrelated selector file changed: %q, %v", got, err)
	}
}

func TestRadioChangeKeepsExistingSelectors(t *testing.T) {
	for _, tc := range []struct{ selector, entry string }{
		{"bootloader.img", "factory/bootloader-test-version.img"},
		{"radio-test-version.img", "factory/radio-test-version.img"},
		{"modem.img", "modem.img"},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			t.Chdir(t.TempDir())
			payload := bytes.Repeat([]byte("payload"), 32)
			data := radioTestZip(t, radioTestEntry{tc.entry, payload, zip.Deflate})
			if err := extractRadioTestZip(t, data, tc.selector); !errors.Is(err, ErrCommandComplete) {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(tc.selector); err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("existing selector: data=%q, error=%v", got, err)
			}
		})
	}
}
