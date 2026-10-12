package rvt

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestDecodeElementHeaderSynthetic(t *testing.T) {
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	var body []byte
	add16 := func(v int16) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(v))
		body = append(body, b[:]...)
	}
	add32 := func(v int32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(v))
		body = append(body, b[:]...)
	}
	add64 := func(v int64) {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(v))
		body = append(body, b[:]...)
	}

	add16(1398) // class
	add32(1)    // history count
	add64(7)    // entry.otherInt64
	add32(-1)   // entry.otherElementId
	add16(3)    // entry.atomType
	add32(9)    // entry pair int
	for i := 0; i < 6; i++ {
		add32(int32(1000 + i)) // ids
	}
	add32(0x81a)  // viewRules
	add32(0x8ba)  // abFlags
	add16(1890)   // classDef: OdBmFamilySymbol@2023
	add32(-1)     // classRefA
	add32(-65220) // classRefB
	add32(0)      // cond1: v==0 → 6B
	add16(-1)
	add32(0) // cond2
	add16(-1)
	body = append(body, []byte{0xAA, 0xBB, 0xCC, 0xDD}...) // dynamic tail

	h, err := DecodeElementHeader(body, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if h.HistoryCount != 1 || h.CategoryID != 1000 || h.MiscID != 1005 {
		t.Errorf("fields %+v", h)
	}
	if h.ViewRulesFlags != 0x81a || h.AbFlags != 0x8ba {
		t.Errorf("flags %+v", h)
	}
	if h.ClassIndex != 1890 || h.ClassName != "OdBmFamilySymbol" {
		t.Errorf("class %d %q", h.ClassIndex, h.ClassName)
	}
	// 2 cls + 4 count + 18 entry + 24 ids + 4 view + 4 ab + 10 classDef
	// + 12 conds = 78
	if h.HeadBytes != 78 {
		t.Errorf("head bytes %d", h.HeadBytes)
	}
	if h.DynamicBytes != 4 {
		t.Errorf("dynamic %d", h.DynamicBytes)
	}
}

func TestDecodeElementHeaderShort(t *testing.T) {
	if _, err := DecodeElementHeader(make([]byte, 10), nil); err == nil {
		t.Fatal("expected error for short body")
	}
}

// TestElementCensusRealSamples decodes the static head of every
// OdBmElementHeader record on the real samples.
func TestElementCensusRealSamples(t *testing.T) {
	found := 0
	for _, path := range samples {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		found++
		version := "2023"
		if strings.Contains(path, "2022") {
			version = "2022"
		}
		f, err := OpenCFB(data)
		if err != nil {
			t.Fatal(err)
		}
		var biggest string
		var sz uint64
		for _, sp := range f.StreamPaths() {
			if len(sp) > 11 && sp[:11] == "Partitions/" {
				s, _ := f.Size(sp)
				if s > sz {
					sz, biggest = s, sp
				}
			}
		}
		raw, _ := f.Stream(biggest)
		lookup := func(cls int16) string {
			name, _ := classindex.Lookup(version, cls)
			return name
		}
		part, err := ReadPartitionWithLookup(biggest[11:], raw, lookup)
		if err != nil {
			t.Fatal(err)
		}
		var hdr *SubStream
		for _, ss := range part.SubStreams {
			if len(ss.Deposits) > 0 {
				hdr = ss
			}
		}
		if hdr == nil {
			t.Fatalf("%s: no deposit sub-stream", path)
		}
		ok, unresolved, undecodable := 0, 0, 0
		for i := range hdr.Records {
			h, err := DecodeElementHeader(hdr.Records[i].Body, lookup)
			if err != nil {
				undecodable++
				continue
			}
			if h.ClassName == "" {
				unresolved++
				continue
			}
			ok++
		}
		total := len(hdr.Records)
		if ok*100 < total*95 {
			t.Errorf("%s: only %d/%d element headers decoded with class", path, ok, total)
		}
		t.Logf("%s: %d/%d decoded, %d unresolved class, %d short; top classes: %v",
			path, ok, total, unresolved, undecodable, topN(ElementClassCensus(hdr, lookup), 8))
		// every record must at least carry the 48B static head
		for i := range hdr.Records {
			if len(hdr.Records[i].Body) < elementHeaderFixedBytes {
				t.Fatalf("%s: record %d body %d < 48", path, i, len(hdr.Records[i].Body))
			}
		}
	}
	if found == 0 {
		t.Skip("no .rvt samples")
	}
}

func topN(cc []ClassCount, n int) []string {
	var out []string
	for i, c := range cc {
		if i >= n {
			break
		}
		out = append(out, fmtClassCount(c))
	}
	return out
}

func fmtClassCount(c ClassCount) string {
	return fmt.Sprintf("%s=%d", c.Name, c.Count)
}
