package rvt

// Formats/Latest schema: the file's own embedded serialization schema —
// every class definition with its tag, base class, version and field
// count. The tag is the class's definition ordinal (first class = 12,
// one more per definition, top-level or inline); the element records
// name their class by it, and the record marker is derived from two
// class tags (Outline / ElementParents).
//
// Grammar (cross-validated against the rvt-rs reference implementation,
// #154 / Discussion #112, schemas 2014-2026): records are separated by a
// zero u16; a class is a u16-counted ASCII name, a u16 base reference
// (bit 15 set: the base class is defined inline right after a zero u16),
// a u32 version, a u32 field count, the fields (u32-counted name plus a
// type word), and a u32-counted list of 16-byte GUIDs. A type word is a
// u32 (kind byte, modifier byte): modifier high nibble 0x10 adds a u32
// array count; kind 0x0d adds the filler 01 00 00 00 20 and an inner
// type, and a class reference when that inner type has one; kind 0x0e
// with a by-value modifier adds a class reference. A class reference is
// a u16, bit 15 meaning the class is defined inline after a zero u16.
// The tag is not stored on the wire — it is the definition ordinal, and
// every inline reference repeats its tag in the low 15 bits, so a
// misread stops the parse instead of shifting every later tag.

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"

	"github.com/flywave/go-rvt/internal/cfb"
)

const firstSchemaTag = 12

// maxSchemaNesting bounds inline definitions (real schemas stay a few
// levels deep).
const maxSchemaNesting = 64

// SchemaClass is one class definition of the Formats/Latest schema.
type SchemaClass struct {
	Tag        uint16 // definition ordinal
	Name       string
	Base       uint16 // base-class tag, 0 when none
	Version    uint32
	FieldCount uint32
	// Fields are the declared fields in wire order (names plus the raw
	// type-word bytes and a coarse kind classification — the substrate
	// for field-level value decoding).
	Fields []SchemaField
}

// SchemaField is one declared field of a schema class.
type SchemaField struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`  // coarse classification
	TypeBytes []byte `json:"type_bytes,omitempty"`
}

// Schema is every class of a Formats/Latest stream, in definition order.
type Schema struct {
	Classes     []SchemaClass
	ParsedBytes int
	Stopped     string // why the parse ended early; empty = complete

	byName map[string]*SchemaClass
}

// ByName returns the class with the given name, or nil.
func (s *Schema) ByName(name string) *SchemaClass {
	if s.byName == nil {
		s.byName = make(map[string]*SchemaClass, len(s.Classes))
		for i := range s.Classes {
			s.byName[s.Classes[i].Name] = &s.Classes[i]
		}
	}
	return s.byName[name]
}

// ByTag returns the class with serialization tag `tag`, or nil. Tags are
// dense from the first class's tag, so the lookup is positional.
func (s *Schema) ByTag(tag uint16) *SchemaClass {
	if len(s.Classes) == 0 {
		return nil
	}
	first := s.Classes[0].Tag
	if tag < first {
		return nil
	}
	i := int(tag - first)
	if i >= len(s.Classes) || s.Classes[i].Tag != tag {
		return nil
	}
	return &s.Classes[i]
}

type schemaReader struct {
	data    []byte
	nextTag uint16
	classes []SchemaClass
}

func (r *schemaReader) u16(at int) (uint16, bool) {
	if at < 0 || at+2 > len(r.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(r.data[at:]), true
}

func (r *schemaReader) u32(at int) (uint32, bool) {
	if at < 0 || at+4 > len(r.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(r.data[at:]), true
}

// name reads a width-prefixed (2 or 4 byte) printable-ASCII name.
func (r *schemaReader) name(at, width int) (string, int, error) {
	var l int
	if width == 2 {
		v, ok := r.u16(at)
		if !ok {
			return "", 0, fmt.Errorf("name length past the end at %#x", at)
		}
		l = int(v)
	} else {
		v, ok := r.u32(at)
		if !ok {
			return "", 0, fmt.Errorf("name length past the end at %#x", at)
		}
		l = int(v)
	}
	start := at + width
	if l < 1 || l > 250 || start+l > len(r.data) {
		return "", 0, fmt.Errorf("bad name length %d at %#x", l, at)
	}
	raw := r.data[start : start+l]
	for _, b := range raw {
		if b < 0x20 || b >= 0x7f {
			return "", 0, fmt.Errorf("name is not ASCII at %#x", at)
		}
	}
	return string(raw), start + l, nil
}

func (r *schemaReader) inlineClass(at int, reference uint16, depth int) (uint16, int, error) {
	if v, ok := r.u16(at); !ok || v != 0 {
		return 0, 0, fmt.Errorf("inline pad is not 0 at %#x", at)
	}
	tag, next, err := r.class(at+2, depth+1)
	if err != nil {
		return 0, 0, err
	}
	if tag != reference&0x7fff {
		return 0, 0, fmt.Errorf("inline tag %d does not match reference %d at %#x", tag, reference&0x7fff, at)
	}
	return tag, next, nil
}

func (r *schemaReader) classRef(at, depth int) (bool, int, error) {
	reference, ok := r.u16(at)
	if !ok {
		return false, 0, fmt.Errorf("class reference past the end at %#x", at)
	}
	if reference&0x8000 == 0 {
		return true, at + 2, nil
	}
	_, next, err := r.inlineClass(at+2, reference, depth)
	if err != nil {
		return false, 0, err
	}
	return true, next, nil
}

// fieldType returns whether the type carries a class reference, the
// byte span it occupies and an error.
func (r *schemaReader) fieldType(at, depth int) (bool, int, int, error) {
	if depth > maxSchemaNesting {
		return false, at, 0, fmt.Errorf("types nest too deep at %#x", at)
	}
	word, ok := r.u32(at)
	if !ok {
		return false, at, 0, fmt.Errorf("type word past the end at %#x", at)
	}
	p := at + 4
	kind, modifier := byte(word&0xff), byte((word>>8)&0xff)
	switch modifier & 0xf0 {
	case 0x00, 0x10, 0x50, 0x60:
	default:
		return false, at, 0, fmt.Errorf("unknown modifier %#04x at %#x", modifier, at)
	}
	if modifier&0xf0 == 0x10 {
		count, ok := r.u32(p)
		if !ok {
			return false, at, 0, fmt.Errorf("array count past the end at %#x", p)
		}
		if count > 65536 {
			return false, at, 0, fmt.Errorf("array count %d at %#x", count, p)
		}
		p += 4
	}
	if kind == 0x0d {
		if p+5 > len(r.data) || r.data[p] != 1 || r.data[p+1] != 0 || r.data[p+2] != 0 || r.data[p+3] != 0 || r.data[p+4] != 0x20 {
			return false, at, 0, fmt.Errorf("array filler at %#x", p)
		}
		innerRef, _, next, err := r.fieldType(p+5, depth+1)
		if err != nil {
			return false, at, 0, err
		}
		if innerRef {
			ref, next2, err := r.classRef(next, depth)
			return ref, at, next2, err
		}
		return false, at, next, nil
	}
	if kind == 0x0e && modifier&0x0f == 0 {
		ref, next, err := r.classRef(p, depth)
		return ref, at, next, err
	}
	return false, at, p, nil
}

// fieldKind classifies a type span coarsely (kind byte + modifier).
func fieldKind(data []byte, start, end int) string {
	if end-start < 1 {
		return "unknown"
	}
	kind := data[start]
	modifier := data[start+1] & 0xf0
	switch {
	case kind == 0x08:
		return "string"
	case kind == 0x09:
		return "guid"
	case kind == 0x0e:
		if modifier == 0x50 || modifier == 0x60 || data[start+1]&0x0f != 0 {
			return "container"
		}
		return "element_id"
	case modifier == 0x10:
		return "vector"
	case modifier == 0x50 || modifier == 0x60:
		return "container"
	}
	switch kind {
	case 0x01:
		return "bool"
	case 0x02:
		return "u16"
	case 0x03, 0x04, 0x05:
		return "i32"
	case 0x06:
		return "f32"
	case 0x07:
		return "f64"
	case 0x0b:
		return "i64"
	case 0x0d:
		return "point"
	}
	return "unknown"
}

func (r *schemaReader) class(at, depth int) (uint16, int, error) {
	if depth > maxSchemaNesting {
		return 0, 0, fmt.Errorf("classes nest too deep at %#x", at)
	}
	name, p, err := r.name(at, 2)
	if err != nil {
		return 0, 0, err
	}
	tag := r.nextTag
	if r.nextTag == 0x7fff {
		return 0, 0, fmt.Errorf("too many classes at %#x", at)
	}
	r.nextTag = tag + 1
	reference, ok := r.u16(p)
	if !ok {
		return 0, 0, fmt.Errorf("base reference past the end at %#x", p)
	}
	p += 2
	var base uint16
	if reference&0x8000 != 0 {
		b, next, err := r.inlineClass(p, reference, depth)
		if err != nil {
			return 0, 0, err
		}
		base, p = b, next
	} else if reference != 0 {
		base = reference
	}
	version, ok := r.u32(p)
	if !ok {
		return 0, 0, fmt.Errorf("version past the end at %#x", p)
	}
	fieldCount, ok := r.u32(p + 4)
	if !ok {
		return 0, 0, fmt.Errorf("field count past the end at %#x", p+4)
	}
	p += 8
	if fieldCount > 2000 {
		return 0, 0, fmt.Errorf("field count %d at %#x", fieldCount, p-4)
	}
	fields := make([]SchemaField, 0, fieldCount)
	for i := uint32(0); i < fieldCount; i++ {
		fname, next, err := r.name(p, 4)
		if err != nil {
			return 0, 0, err
		}
		typeStart := next
		_, _, next, err = r.fieldType(next, depth)
		if err != nil {
			return 0, 0, err
		}
		fields = append(fields, SchemaField{
			Name:      fname,
			Kind:      fieldKind(r.data, typeStart, next),
			TypeBytes: append([]byte(nil), r.data[typeStart:next]...),
		})
		p = next
	}
	guids, ok := r.u32(p)
	if !ok {
		return 0, 0, fmt.Errorf("GUID count past the end at %#x", p)
	}
	p += 4
	if guids > 60000 || p+int(guids)*16 > len(r.data) {
		return 0, 0, fmt.Errorf("GUID count %d at %#x", guids, p-4)
	}
	p += int(guids) * 16
	r.classes = append(r.classes, SchemaClass{
		Tag: tag, Name: name, Base: base, Version: version, FieldCount: fieldCount,
		Fields: fields,
	})
	return tag, p, nil
}

// ParseSchema reads the class records of a page-stripped, decompressed
// Formats/Latest stream. The parse stops (with Stopped set) at the first
// record that breaks the grammar.
func ParseSchema(data []byte) *Schema {
	r := &schemaReader{data: data, nextTag: firstSchemaTag}
	at := 0
	var stopped string
	for {
		if at+2 > len(data) {
			break
		}
		if v, ok := r.u16(at); !ok || v != 0 {
			stopped = fmt.Sprintf("record separator is not 0 at %#x", at)
			break
		}
		at += 2
		if at >= len(data) || allZero(data[at:]) {
			at = len(data)
			break
		}
		tag, count := r.nextTag, len(r.classes)
		_, next, err := r.class(at, 0)
		if err != nil {
			r.classes = r.classes[:count]
			r.nextTag = tag
			stopped = err.Error()
			break
		}
		at = next
	}
	sort.SliceStable(r.classes, func(i, j int) bool { return r.classes[i].Tag < r.classes[j].Tag })
	return &Schema{Classes: r.classes, ParsedBytes: at, Stopped: stopped}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// OpenSchema parses the Formats/Latest schema of the RVT file at path.
func OpenSchema(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return SchemaFromCFB(data)
}

// SchemaFromCFB parses the Formats/Latest schema of an in-memory RVT file.
func SchemaFromCFB(data []byte) (*Schema, error) {
	f, err := OpenCFB(data)
	if err != nil {
		return nil, fmt.Errorf("rvt: cfb: %w", err)
	}
	return SchemaFromStream(f)
}

// SchemaFromStream parses the Formats/Latest schema of an open container.
func SchemaFromStream(f *cfb.File) (*Schema, error) {
	stored, err := f.Stream("Formats/Latest")
	if err != nil {
		return nil, fmt.Errorf("rvt: Formats/Latest: %w", err)
	}
	_, inflated, err := decompressTable("Formats/Latest", stored)
	if err != nil || inflated == nil {
		return nil, fmt.Errorf("rvt: Formats/Latest: %w", err)
	}
	return ParseSchema(inflated), nil
}
