package rvt

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/internal/zg"
	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestM4RealSamples validates the object-record census, ElemTable parse and
// the ElemTable↔partition deposit linkage on the real samples.
func TestM4RealSamples(t *testing.T) {
	found := 0
	for _, path := range samples {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		found++
		md, err := Open(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		version := md.RevitVersion

		// ElemTable: 2023 sample = 8276 records, no id/id2 mismatches.
		// (The 2022 sample's ElemTable deflate is locally corrupted — R21.)
		if strings.Contains(path, "2023") && md.ElemTable == nil {
			t.Errorf("%s: no elem table info", path)
		}
		if strings.Contains(path, "2023") {
			if md.ElemTable.RecordCount != 8276 {
				t.Errorf("%s: elemtable records %d (want 8276)", path, md.ElemTable.RecordCount)
			}
			if md.ElemTable.IDErrors != 0 {
				t.Errorf("%s: elemtable id errors %d", path, md.ElemTable.IDErrors)
			}
			t.Logf("%s: elemtable %d records, ids %d..%d, footer %dB",
				path, md.ElemTable.RecordCount, md.ElemTable.FirstID, md.ElemTable.LastID, md.ElemTable.FooterBytes)
		}

		// UnitSystem: at least three Global tables report metric (0).
		metric := 0
		for _, ti := range md.Tables {
			if ti.UnitSystem == "metric" {
				metric++
			}
		}
		if metric < 3 {
			t.Errorf("%s: only %d metric tables", path, metric)
		}

		// ProjectInformation: ZIP with one Atom project XML (R44).
		if len(md.ProjectEntries) != 1 || !strings.Contains(md.ProjectEntries[0], ".project.xml") {
			t.Errorf("%s: project entries %v", path, md.ProjectEntries)
		}

		// Object-record census on the biggest partition.
		f, err := OpenCFB(data)
		if err != nil {
			t.Fatal(err)
		}
		var biggest string
		var biggestSize uint64
		for _, s := range md.Streams {
			if strings.HasPrefix(s.Path, "Partitions/") && s.Size > biggestSize {
				biggestSize = s.Size
				biggest = s.Path
			}
		}
		raw, err := f.Stream(biggest)
		if err != nil {
			t.Fatal(err)
		}
		lookup := func(cls int16) string {
			name, _ := classindex.Lookup(version, cls)
			return name
		}
		part, err := ReadPartitionWithLookup(biggest[len("Partitions/"):], raw, lookup)
		if err != nil {
			t.Fatal(err)
		}
		var hdr *SubStream
		for _, ss := range part.SubStreams {
			if len(ss.Deposits) > 0 {
				hdr = ss
			}
		}
		if hdr == nil || len(hdr.Records) == 0 {
			t.Fatalf("%s: no object records walked", path)
		}
		// ≥99% of walked records are OdBmElementHeader objects; the few
		// class-0 records (deposit parts without a class) and possible
		// resync false positives are counted, not fatal.
		headers := 0
		for _, r := range hdr.Records {
			if r.ClassName == "OdBmElementHeader" {
				headers++
			}
		}
		if len(hdr.Records) == 0 || headers*100 < len(hdr.Records)*99 {
			t.Fatalf("%s: only %d/%d OdBmElementHeader records", path, headers, len(hdr.Records))
		}
		coverage := 100 * float64(hdr.WalkStats.BytesWalked) / float64(len(hdr.Data))
		t.Logf("%s: census %d records (%d OdBmElementHeader, %d other), walk coverage %.1f%% (resyncs %d)",
			path, len(hdr.Records), headers, len(hdr.Records)-headers, coverage, hdr.WalkStats.Resyncs)
		if coverage < 90 {
			t.Errorf("%s: walk coverage %.1f%%", path, coverage)
		}

	}
	if found == 0 {
		t.Skip("no .rvt samples on this machine")
	}
}

// TestM4ElemTableDepositLinkage checks that the ElemTable footer embeds the
// last deposit counter of the biggest partition.
func TestM4ElemTableDepositLinkage(t *testing.T) {
	data, err := os.ReadFile(samples[0]) // 2023
	if err != nil {
		t.Skip("no sample")
	}
	md, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if md.ElemTable == nil || md.ElemTable.Err != "" || md.ElemTable.FooterBytes == 0 {
		t.Skip("elem table footer unavailable")
	}
	f, _ := OpenCFB(data)
	etRaw, _ := f.Stream("Global/ElemTable")
	idx := bytes.Index(etRaw, []byte{0x1f, 0x8b, 0x08})
	mm, err := zg.DecompressOne(etRaw, idx)
	if err != nil {
		t.Fatal(err)
	}
	et, err := ParseElemTable(mm.Data)
	if err != nil {
		t.Fatal(err)
	}
	var last uint32
	for _, pi := range md.Partitions {
		if pi.LastCounter > last {
			last = pi.LastCounter
		}
	}
	var link [4]byte
	binary.LittleEndian.PutUint32(link[:], last)
	if !bytes.Contains(et.Footer, link[:]) {
		t.Errorf("elemtable footer %x does not embed last deposit counter %d", et.Footer, last)
	}
	t.Logf("elemtable footer %x embeds deposit counter %d ✓", et.Footer, last)
}
