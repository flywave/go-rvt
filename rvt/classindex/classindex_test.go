package classindex

import "testing"

// Anchors from analysis/06-class-index.md (REA extraction).
func TestLookupAnchors(t *testing.T) {
	cases := []struct {
		version string
		index   int16
		want    string
	}{
		{"2023", 1398, "OdBmElementHeader"},
		{"2022", 1340, "OdBmElementHeader"}, // cross-version renumbering
		{"2023", 2073, "OdBmGElement"},
		{"2023", 2044, "OdBmGBrep"},
		{"2023", 1481, "OdBmEntityInfoData"},
		{"2023", 2081, "OdBmGFilter"},
	}
	for _, c := range cases {
		got, ok := Lookup(c.version, c.index)
		if !ok || got != c.want {
			t.Errorf("Lookup(%q, %d) = %q, %v; want %q", c.version, c.index, got, ok, c.want)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("2015", 100); ok {
		t.Error("unknown version should not resolve")
	}
	if _, ok := Lookup("2023", 30000); ok {
		t.Error("index outside table should not resolve")
	}
	if !HasVersion("2023") || !HasVersion("2022") || HasVersion("2014") {
		t.Error("HasVersion broken")
	}
	// version strings with prefixes/suffixes resolve via the year
	if _, ok := Lookup("Autodesk Revit 2023.1", 1398); !ok {
		t.Error("year extraction failed")
	}
}
