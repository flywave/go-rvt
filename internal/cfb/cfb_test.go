package cfb

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/internal/cfb/cfbtest"
)

func TestOpenFlatFixture(t *testing.T) {
	data := cfbtest.Write(map[string][]byte{
		"alpha": []byte("hello mini"),
		"beta":  bytes.Repeat([]byte{0xAB}, 8192), // > mini cutoff → FAT chain
	})
	f, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	paths := f.StreamPaths()
	if len(paths) != 2 || paths[0] != "alpha" || paths[1] != "beta" {
		t.Fatalf("paths: %v", paths)
	}
	b, err := f.Stream("alpha")
	if err != nil || string(b) != "hello mini" {
		t.Fatalf("alpha: %q %v", b, err)
	}
	b, err = f.Stream("beta")
	if err != nil || len(b) != 8192 || b[0] != 0xAB {
		t.Fatalf("beta: len=%d err=%v", len(b), err)
	}
}

var samples = []string{
	"/Users/xuning/Downloads/2023 racbasicsampleproject (2).rvt",
	"/Users/xuning/Downloads/2022 rstadvancedsampleproject (1).rvt",
}

// TestOpenRealRvtSample validates the recursive path walk against a real
// Revit file: nested storages (Global/, Formats/, Partitions/) must appear
// with full slash paths.
func TestOpenRealRvtSample(t *testing.T) {
	found := 0
	for _, p := range samples {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		found++
		f, err := Open(data)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		streams := map[string]uint64{}
		for _, sp := range f.StreamPaths() {
			sz, _ := f.Size(sp)
			streams[sp] = sz
		}
		want := map[string]uint64{
			"BasicFileInfo":      0, // any size
			"Contents":           0,
			"TransmissionData":   0,
			"Global/ElemTable":   0,
			"Global/Latest":      0,
			"Formats/Latest":     0,
			"ProjectInformation": 0,
			"RevitPreview4.0":    0,
		}
		for name := range want {
			sz, ok := streams[name]
			if !ok {
				t.Errorf("%s: missing stream %s", p, name)
				continue
			}
			if name != "Contents" && sz == 0 && name != "Global/PartitionTable" {
				t.Errorf("%s: empty stream %s", p, name)
			}
		}
		// Partitions/<n> exists with dynamic name
		foundPart := false
		for name, sz := range streams {
			if strings.HasPrefix(name, "Partitions/") && sz > 1<<20 {
				foundPart = true
				t.Logf("%s: partition stream %s (%d bytes)", p, name, sz)
			}
		}
		if !foundPart {
			t.Errorf("%s: no large Partitions/* stream", p)
		}
		if f.IsStorage("Global") != true {
			t.Errorf("%s: Global should be storage", p)
		}
		if _, err := f.Stream("Global"); err == nil {
			t.Errorf("%s: Global stream read should fail", p)
		}
	}
	if found == 0 {
		t.Skip("no .rvt samples on this machine")
	}
}
