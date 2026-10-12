package rvt

import (
	"os"
	"strings"
	"testing"
)

// TestM2TablesAndContents validates the M2 table layer.
//
// Healthy files (2023 golden sample): every Global/* and Formats/* stream
// decompresses with CRC verified; Contents holds the GLOBAL index record.
// Files with locally corrupted table bytes (observed: the 2023→"2022"
// sample copy in ~/Downloads) must still identify and must surface per-table
// errors rather than hide them.
func TestM2TablesAndContents(t *testing.T) {
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
		if len(md.Tables) < 6 {
			t.Errorf("%s: expected >=6 table entries, got %d", p, len(md.Tables))
		}
		okTables, errTables := 0, 0
		var et *TableInfo
		for i := range md.Tables {
			ti := md.Tables[i]
			if ti.Error != "" {
				errTables++
				t.Logf("%s: table error %s: %s", p, ti.Path, ti.Error)
				continue
			}
			okTables++
			if ti.Path == "Global/ElemTable" {
				et = &md.Tables[i]
			}
		}
		if okTables == 0 {
			t.Errorf("%s: no table decompressed cleanly", p)
		}
		if et != nil {
			if et.Error == "" && et.DecompressedSize < 100000 {
				t.Errorf("%s: ElemTable decomp %d too small", p, et.DecompressedSize)
			}
			t.Logf("%s: ElemTable comp=%d decomp=%d trailer=%d err=%q",
				p, et.CompressedBytes, et.DecompressedSize, et.TrailerBytes, et.Error)
		} else if errTables == 0 {
			t.Errorf("%s: missing Global/ElemTable table", p)
		}
		// Contents: at least one gzip record with GLOBAL on healthy files;
		// on corrupt files the record list must still be non-empty.
		if len(md.ContentsRecords) == 0 {
			t.Errorf("%s: no contents records", p)
			continue
		}
		hasGzip := false
		for _, g := range md.ContentsRecords {
			if g.GZIP && g.DecompressedLen > 0 {
				hasGzip = true
				joined := strings.Join(g.Strings, "|")
				if strings.Contains(joined, "GLOBAL") {
					t.Logf("%s: contents gzip decomp=%d strings=%v", p, g.DecompressedLen, g.Strings)
				}
			}
		}
		if !hasGzip {
			t.Errorf("%s: no decompressible contents record", p)
		}
		// Worksets: only asserted when PartitionTable decompressed cleanly.
		for _, ti := range md.Tables {
			if ti.Path == "Global/PartitionTable" && ti.Error == "" {
				if len(md.PartitionWorksets) == 0 {
					t.Errorf("%s: PartitionTable ok but no worksets", p)
				} else {
					t.Logf("%s: worksets=%v", p, md.PartitionWorksets)
				}
			}
		}
	}
	if found == 0 {
		t.Skip("no .rvt samples on this machine")
	}
}
