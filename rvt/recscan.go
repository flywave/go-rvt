package rvt

// High-level assembly for the partition element-record scan: schema +
// ElemTable join + per-partition scan.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flywave/go-rvt/internal/cfb"
	"github.com/flywave/go-rvt/rvt/classindex"
)


// RecordsReport carries the element-record scan of one file.
type RecordsReport struct {
	Version int             `json:"version"`
	Schema  *Schema         `json:"-"`
	Records []ElementRecord `json:"records,omitempty"`
	// Declared is the Global/ElemTable id set.
	Declared map[uint32]bool `json:"-"`
	// Profiles maps sketched element id → plan profile reconstructed
	// from its OST_SKETCH_LINES records (floors, roofs, ceilings, pads).
	Profiles map[uint32]*PlanProfile `json:"-"`
}

// RecordsFromStream scans the element records of an in-memory RVT file.
// Empty (nil) on releases whose record shape is not proven (fail closed).
func RecordsFromStream(data []byte) (*RecordsReport, error) {
	f, err := OpenCFB(data)
	if err != nil {
		return nil, fmt.Errorf("rvt: cfb: %w", err)
	}
	version := ""
	if md, err := Open(data); err == nil {
		version = md.RevitVersion
	}
	return RecordsFromContainer(f, version)
}

// RecordsFromContainer scans the element records of an open container
// (as returned by OpenCFB). version is the BasicFileInfo saved-in-version
// string (year extracted here).
func RecordsFromContainer(f *cfb.File, version string) (*RecordsReport, error) {
	v := 0
	if y := strings.TrimSpace(version); len(y) >= 4 {
		if n, err := strconv.Atoi(y[:4]); err == nil {
			v = n
		}
	}
	schema, err := SchemaFromStream(f)
	if err != nil {
		return nil, err
	}
	declared := map[uint32]bool{}
	if stored, err := f.Stream("Global/ElemTable"); err == nil {
		if _, inflated, terr := decompressTable("Global/ElemTable", stored); terr == nil && inflated != nil {
			if et, perr := ParseElemTable(inflated); perr == nil {
				for _, e := range et.Entries {
					declared[e.ID] = true
				}
			}
		}
	}
	rep := &RecordsReport{Version: v, Schema: schema, Declared: declared}
	for _, sp := range f.StreamPaths() {
		if !strings.HasPrefix(sp, "Partitions/") {
			continue
		}
		stored, err := f.Stream(sp)
		if err != nil || len(stored) == 0 {
			continue
		}
		p, err := ReadPartition(strings.TrimPrefix(sp, "Partitions/"), stored)
		if err != nil {
			continue
		}
		vy := strconv.Itoa(v)
		lookup := func(cls int16) string {
			n, _ := classindex.Lookup(vy, cls)
			return n
		}
		rep.Records = append(rep.Records, ScanElementRecordsVersioned(p, sp, schema, declared, v, lookup)...)
	}
	rep.Profiles = PlanProfilesFromRecords(rep.Records)
	return rep, nil
}
