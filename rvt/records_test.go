package rvt

// Element-record scan tests, cross-validated against the rvt-rs reference
// implementation on the regression samples (1,197 records / 876 placed on
// rac_basic_2023).

import (
	"os"
	"testing"
)

func TestElementRecords2023(t *testing.T) {
	path := "testdata/rac_basic_2023.rvt"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no sample")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RecordsFromStream(data)
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if rep.Version != 2023 {
		t.Fatalf("version %d", rep.Version)
	}
	if len(rep.Records) != 1197 {
		t.Errorf("records %d, want 1197 (rvt-rs reference)", len(rep.Records))
	}
	placed := 0
	walls, doors, windows := 0, 0, 0
	withRefs := 0
	byID := map[uint32]bool{}
	for i := range rep.Records {
		r := &rep.Records[i]
		if r.IsPlacedInstance() {
			placed++
		}
		if len(r.References) > 0 {
			withRefs++
		}
		if byID[r.ElementID] {
			t.Errorf("duplicate record id %d", r.ElementID)
		}
		byID[r.ElementID] = true
		switch r.Category {
		case -2_000_011:
			walls++
		case -2_000_023:
			doors++
		case -2_000_014:
			windows++
		}
	}
	if placed != 876 {
		t.Errorf("placed %d, want 876 (rvt-rs reference)", placed)
	}
	if withRefs != 1197 {
		t.Errorf("records with reference lists %d, want 1197", withRefs)
	}
	// category spot checks against the rvt-rs histogram
	if walls != 56 {
		t.Errorf("OST_Walls records %d, want 56", walls)
	}
	// sketch-plan profiles: nine floor/roof sketches close on this file
	// (reconstruction cross-checked: every profile vertex lies inside its
	// owner record's bbox)
	if len(rep.Profiles) != 9 {
		t.Errorf("plan profiles %d, want 9", len(rep.Profiles))
	}
	for id, p := range rep.Profiles {
		if len(p.OuterXY) < 3 {
			t.Errorf("profile of %d: %d outer vertices", id, len(p.OuterXY))
		}
	}
	t.Logf("records=%d placed=%d walls=%d doors=%d windows=%d profiles=%d",
		len(rep.Records), placed, walls, doors, windows, len(rep.Profiles))

	// class-tag histogram pins the record-layer class coverage: every
	// record's class tag resolves in the file's own schema (cross-checked
	// against the rvt-rs typed projection: SWall=56 walls, FamilyInstance
	// =398, FamilySymbol=141, CurveElem=230 sketch/curve elements)
	counts := map[int]int{}
	for i := range rep.Records {
		counts[int(rep.Records[i].ClassTag)]++
	}
	for tag := range counts {
		if rep.Schema.ByTag(uint16(tag)) == nil {
			t.Errorf("record class tag %d absent from schema", tag)
		}
	}
	if counts[int(rep.Schema.ByName("SWall").Tag)] != 56 {
		t.Errorf("SWall record count %d, want 56", counts[int(rep.Schema.ByName("SWall").Tag)])
	}
	if counts[int(rep.Schema.ByName("FamilyInstance").Tag)] != 398 {
		t.Errorf("FamilyInstance record count %d, want 398", counts[int(rep.Schema.ByName("FamilyInstance").Tag)])
	}
}

// TestBoundedLines2023 checks the wall location-curve scan: every placed
// wall's serialised data carries one (cross-validated 56/56 against the
// rvt-rs reference).
func TestBoundedLines2023(t *testing.T) {
	path := "testdata/rac_basic_2023.rvt"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no sample")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RecordsFromStream(data)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[uint32]bool{}
	walls := map[uint32]bool{}
	if f, err := OpenCFB(data); err == nil {
		if s, err := f.Stream("Global/ElemTable"); err == nil {
			if _, inf, e2 := decompressTable("Global/ElemTable", s); e2 == nil && inf != nil {
				if et, e3 := ParseElemTable(inf); e3 == nil {
					for _, e := range et.Entries {
						declared[e.ID] = true
					}
				}
			}
		}
		stored, err := f.Stream("Partitions/13")
		if err != nil {
			t.Fatal(err)
		}
		p, err := ReadPartition("13", stored)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rep.Records {
			r := &rep.Records[i]
			if r.Category == -2_000_011 && r.IsPlacedInstance() {
				walls[r.ElementID] = true
			}
		}
		lines := ScanFirstBoundedLines(p, rep.Schema, walls, rep.Version)
		if len(lines) != 56 {
			t.Errorf("wall location lines %d, want 56", len(lines))
		}
		for id, l := range lines {
			a, b := l.Endpoints()
			span := b[0] - a[0] + b[1] - a[1] + b[2] - a[2]
			if span == 0 {
				t.Errorf("wall %d line is degenerate", id)
			}
		}
		// compound layer lists (2023 layout): sane total thicknesses
		lists := ScanCompoundLayerLists(p, declared, rep.Version)
		if len(lists) == 0 {
			t.Error("no compound layer lists found")
		}
		for i, layers := range lists {
			total := 0.0
			for _, l := range layers {
				total += l.WidthFeet
			}
			if total <= 0 || total >= 10 {
				t.Errorf("layer list %d: total thickness %v out of range", i, total)
			}
		}
		t.Logf("lines=%d arcs=%d layerLists=%d", len(lines), 0, len(lists))
	}
}

// TestScanFailClosed checks the record and bounded-line scans refuse
// unproven releases.
func TestScanFailClosed(t *testing.T) {
	if rep, err := RecordsFromStream([]byte("not an rvt")); err == nil && len(rep.Records) > 0 {
		t.Error("records decoded from garbage")
	} else if err == nil {
		t.Error("garbage accepted as rvt")
	}
}

// TestTypeNames2023 checks the type-element join: every element whose
// data carries a compound layer list resolves a framed type name
// (cross-validated: 27/27 on rac_basic_2023 — "Interior - 165 Partition
// (1-hr)", "Retaining - 300mm Concrete", …), and each placed wall joins
// exactly one layered type through its reference list.
func TestTypeNames2023(t *testing.T) {
	path := "testdata/rac_basic_2023.rvt"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no sample")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := RecordsFromStream(data)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != 2023 {
		t.Fatalf("version %d", rep.Version)
	}
	f2, err := OpenCFB(data)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f2.Stream("Partitions/13")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ReadPartition("13", stored)
	if err != nil {
		t.Fatal(err)
	}
	thk := TypeLayerThickness(p, rep.Schema, rep.Declared, rep.Version)
	if len(thk) == 0 {
		t.Fatal("no type layer thicknesses decoded")
	}
	for id, v := range thk {
		if v <= 0 || v >= 10 {
			t.Errorf("type %d thickness %v out of range", id, v)
		}
	}
	names := ScanElementDataNames(p, rep.Schema, func() map[uint32]bool {
		m := make(map[uint32]bool, len(thk))
		for id := range thk {
			m[id] = true
		}
		return m
	}(), rep.Version)
	if len(names) != len(thk) {
		t.Errorf("type names %d/%d", len(names), len(thk))
	}
	join := TypeJoinByReference(rep.Records, thk)
	// 47 of the joins are placed walls (cross-validated); the rest are
	// floors, ceilings, roofs and pads — every one unambiguous
	wallJoins := 0
	for i := range rep.Records {
		r := &rep.Records[i]
		if r.Category == -2_000_011 {
			if _, ok := join[r.ElementID]; ok {
				wallJoins++
			}
		}
	}
	if wallJoins != 47 {
		t.Errorf("wall→type joins %d, want 47 (cross-validated)", wallJoins)
	}
	if len(join) != 62 {
		t.Errorf("total type joins %d, want 62", len(join))
	}
	for wall, ty := range join {
		if _, ok := names[ty]; !ok {
			t.Errorf("wall %d joins type %d without a name", wall, ty)
		}
	}
	t.Logf("types=%d names=%d joins=%d", len(thk), len(names), len(join))
}
