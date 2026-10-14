package rvt

// Formats/Latest schema parser tests. Reference values cross-validated
// against the rvt-rs reference implementation (grammar parse #154) on the
// regression samples.

import (
	"os"
	"testing"
)

func TestSchema2023(t *testing.T) {
	path := "testdata/rac_basic_2023.rvt"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no sample")
	}
	s, err := OpenSchema(path)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	if s.Stopped != "" {
		t.Fatalf("parse stopped: %s (at %d)", s.Stopped, s.ParsedBytes)
	}
	if len(s.Classes) != 4418 {
		t.Errorf("classes %d, want 4418 (rvt-rs reference)", len(s.Classes))
	}
	for name, wantTag := range map[string]uint16{
		"Outline":        316,
		"ElementParents": 1410,
		"ElementHeader":  1398,
		"ElemTable":      1370,
	} {
		c := s.ByName(name)
		if c == nil {
			t.Errorf("class %s absent", name)
			continue
		}
		if c.Tag != wantTag {
			t.Errorf("class %s tag %d, want %d", name, c.Tag, wantTag)
		}
	}
	if s.ByTag(1398) == nil || s.ByTag(1398).Name != "ElementHeader" {
		t.Errorf("ByTag(1398) = %v", s.ByTag(1398))
	}
	// field-level decode substrate: ElementHeader declares 12 fields
	// (rvt-rs reference) with names and classified kinds
	eh := s.ByName("ElementHeader")
	if len(eh.Fields) != 12 {
		t.Errorf("ElementHeader fields %d, want 12", len(eh.Fields))
	}
	for _, f := range eh.Fields {
		if f.Name == "" || f.Kind == "" || f.Kind == "unknown" {
			t.Errorf("ElementHeader field %q kind %q", f.Name, f.Kind)
		}
	}
	if c := s.ByTag(12); c == nil {
		t.Errorf("first class (tag 12) absent")
	} else {
		t.Logf("first class: %s", c.Name)
	}
	t.Logf("schema: %d classes, parsed %d bytes", len(s.Classes), s.ParsedBytes)
}
