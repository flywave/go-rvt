package rvt

import (
	"os"
	"strings"
	"testing"
)

// testdataDir returns the path to the RVT test fixtures. Tests use
// package-relative testdata/ which is populated manually (see testdata/README.md).
func testdataDir() string {
	for _, dir := range []string{"testdata", "../rvt/testdata"} {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}

var samples = []string{
	"testdata/rac_basic_2023.rvt",
	"testdata/rst_advanced_2022.rvt",
}

func TestOpenRejectsNonCFB(t *testing.T) {
	if _, err := Open([]byte("hello not a container")); err == nil {
		t.Fatal("expected error for non-CFB input")
	}
}

func TestOpenRealSamples(t *testing.T) {
	found := 0
	for _, p := range samples {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		found++
		md, err := Open(data)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if md.Format != "rvt" || md.FileSize != int64(len(data)) {
			t.Fatalf("%s: identity %+v", p, md)
		}
		if md.RevitVersion != "2023" && md.RevitVersion != "2022" {
			t.Fatalf("%s: version %q", p, md.RevitVersion)
		}
		if md.BasicFileInfo == nil || md.BFIError != "" {
			t.Fatalf("%s: bfi err=%q", p, md.BFIError)
		}
		// nested stream paths must be present
		paths := map[string]uint64{}
		for _, s := range md.Streams {
			paths[s.Path] = s.Size
		}
		for _, want := range []string{
			"BasicFileInfo", "Contents", "TransmissionData",
			"Global/ElemTable", "Global/PartitionTable", "Formats/Latest",
		} {
			if _, ok := paths[want]; !ok {
				t.Errorf("%s: missing stream %s (have %d streams)", p, want, len(md.Streams))
			}
		}
		if paths["Global/ElemTable"] == 0 {
			t.Errorf("%s: empty ElemTable", p)
		}
		if md.Transmission == nil || md.Transmission.Version != "5" {
			t.Errorf("%s: transmission %+v", p, md.Transmission)
		}
		if !strings.Contains(md.Transmission.XML, "TransmissionData") {
			t.Errorf("%s: transmission xml head %q", p, md.Transmission.XML[:min(80, len(md.Transmission.XML))])
		}
		t.Logf("%s: ver=%s streams=%d transmission=v%s elemTable=%d",
			p, md.RevitVersion, len(md.Streams), md.Transmission.Version, paths["Global/ElemTable"])
	}
	if found == 0 {
		t.Skip("no .rvt samples on this machine")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
