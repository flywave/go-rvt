package bfi

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/flywave/go-rvt/internal/cfb"
)

func u16s(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return b
}

func putStr(buf []byte, s string) []byte {
	u := utf16.Encode([]rune(s))
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(u)))
	buf = append(buf, tmp[:]...)
	return append(buf, u16s(s)...)
}

// buildRev14 assembles a minimal revision-14 stream matching the observed layout.
func buildRev14() []byte {
	var b []byte
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], 14)
	b = append(b, tmp[:]...)
	b = append(b, make([]byte, 10)...)
	b = putStr(b, "2024")
	b = putStr(b, "20230401_1515(x64)")
	b = putStr(b, `C:\tmp\demo.rvt`)
	binary.LittleEndian.PutUint32(tmp[:], 3)
	b = append(b, tmp[:]...)
	b = append(b, 0) // workshared flag
	b = putStr(b, "00000000-0000-0000-0000-000000000000")
	b = putStr(b, "ENU")
	b = append(b, 0) // marker1
	binary.LittleEndian.PutUint32(tmp[:], 7)
	b = append(b, tmp[:]...)
	b = putStr(b, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	b = putStr(b, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	b = putStr(b, "7")
	b = putStr(b, "00000000-0000-0000-0000-000000000000")
	b = append(b, 0) // marker2
	b = putStr(b, "Autodesk Revit")
	b = putStr(b, "RevitApplication")
	b = append(b, 0x0d, 0x0a)
	b = append(b, u16s("Format: 2024\r\nAuthor: Autodesk Revit")...)
	return b
}

func TestParseSynthetic(t *testing.T) {
	fi, err := Parse(buildRev14())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Revision != 14 || fi.SavedInVersion != "2024" || fi.Build != "20230401_1515(x64)" {
		t.Fatalf("identity: %+v", fi)
	}
	if fi.Language != "ENU" || fi.CMVerNum != 7 || fi.Increments != "7" {
		t.Fatalf("counters: %+v", fi)
	}
	if fi.Author != "Autodesk Revit" || fi.ClientAppName != "RevitApplication" {
		t.Fatalf("names: %+v", fi)
	}
	if fi.IsWorkshared {
		t.Fatal("should not be workshared")
	}
	if fi.Summary == "" || !strings.Contains(fi.Summary, "Format: 2024") {
		t.Fatalf("summary: %q", fi.Summary)
	}
}

func TestParseTruncated(t *testing.T) {
	full := buildRev14()
	// Cuts inside the structured region must fail; the tail summary is
	// best-effort and does not gate success.
	for _, cut := range []int{0, 4, 20, 40, 100} {
		if cut >= len(full) {
			continue
		}
		if _, err := Parse(full[:cut]); err == nil {
			t.Fatalf("cut %d: expected error", cut)
		}
	}
}

var samples = []string{
	"/Users/xuning/Downloads/2023 racbasicsampleproject (2).rvt",
	"/Users/xuning/Downloads/2022 rstadvancedsampleproject (1).rvt",
}

func TestParseRealSamples(t *testing.T) {
	type want struct {
		version, build, lang, author string
	}
	wants := map[string]want{
		"2023": {version: "2023", build: "20220401_1515(x64)", lang: "ENU", author: "Autodesk Revit"},
		"2022": {version: "2022", build: "20210129_1515(x64)", lang: "ENU", author: "Autodesk Revit"},
	}
	found := 0
	for _, p := range samples {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		found++
		f, err := cfb.Open(data)
		if err != nil {
			t.Fatalf("%s: cfb: %v", p, err)
		}
		b, err := f.Stream("BasicFileInfo")
		if err != nil {
			t.Fatalf("%s: stream: %v", p, err)
		}
		fi, err := Parse(b)
		if err != nil {
			t.Fatalf("%s: parse: %v", p, err)
		}
		w := wants[fi.SavedInVersion]
		if w.version == "" {
			t.Fatalf("%s: unexpected version %q", p, fi.SavedInVersion)
		}
		if fi.Build != w.build || fi.Language != w.lang || fi.Author != w.author {
			t.Fatalf("%s: fields %+v", p, fi)
		}
		if fi.Revision != 14 {
			t.Fatalf("%s: revision %d", p, fi.Revision)
		}
		if fi.UniqueDocumentGUID == "" || fi.CentralModelEpisodeGUID == "" {
			t.Fatalf("%s: guids empty", p)
		}
		t.Logf("%s: rev=%d ver=%s build=%s cmver=%d inc=%s udg=%s",
			p, fi.Revision, fi.SavedInVersion, fi.Build, fi.CMVerNum, fi.Increments, fi.UniqueDocumentGUID)
	}
	if found == 0 {
		t.Skip("no .rvt samples on this machine")
	}
}

func TestCheckRevision(t *testing.T) {
	cases := []struct {
		rev   uint32
		ver   string
		valid bool
	}{
		{14, "2023", true},
		{13, "2022", true},
		{14, "2022", true},
		{3, "2011", true},
		{14, "2011", false},
		{6, "2015", false},
		{9, "2015", true},
		{14, "Autodesk Revit 2024.2", true}, // year extraction
		{99, "2009", true},                  // unknown version accepted
	}
	for _, c := range cases {
		fi := &FileInfo{Revision: c.rev, SavedInVersion: c.ver}
		err := CheckRevision(fi)
		if c.valid && err != nil {
			t.Errorf("CheckRevision(%d, %s) = %v, want nil", c.rev, c.ver, err)
		}
		if !c.valid && err == nil {
			t.Errorf("CheckRevision(%d, %s) = nil, want mismatch", c.rev, c.ver)
		}
	}
}
