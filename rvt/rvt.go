// Package rvt opens Revit .rvt files (OLE/CFB containers) and extracts
// identification metadata: stream inventory, BasicFileInfo and TransmissionData.
package rvt

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"sort"
	"unicode/utf16"

	"github.com/flywave/go-rvt/bfi"
	"github.com/flywave/go-rvt/internal/cfb"
	"github.com/flywave/go-rvt/rvt/classindex"
)

// StreamInfo describes one CFB stream.
type StreamInfo struct {
	Path string `json:"path"`
	Size uint64 `json:"size"`
}

// TransmissionInfo is the decoded TransmissionData stream (UTF-16 XML).
type TransmissionInfo struct {
	Version string `json:"version,omitempty"`
	XML     string `json:"xml"`
}

// Metadata is the M1 identification result for one .rvt file.
type Metadata struct {
	Format   string `json:"format"` // "rvt"
	FileSize int64  `json:"file_size"`

	// RevitVersion is BasicFileInfo.SavedInVersion (e.g. "2023"); empty when
	// BasicFileInfo is missing or unparsable.
	RevitVersion string `json:"revit_version,omitempty"`
	Build        string `json:"build,omitempty"`
	Workshared   bool   `json:"workshared"`
	Language     string `json:"language,omitempty"`

	BasicFileInfo *bfi.FileInfo     `json:"basic_file_info,omitempty"`
	Transmission  *TransmissionInfo `json:"transmission,omitempty"`

	Streams []StreamInfo `json:"streams"`

	// M2: compressed table streams (Global/*, Formats/*) and the Contents
	// frame index.
	Tables            []TableInfo      `json:"tables,omitempty"`
	ContentsRecords   []ContentsRecord `json:"contents_records,omitempty"`
	PartitionWorksets []string         `json:"partition_worksets,omitempty"`
	ElemTable         *ElemTableInfo   `json:"elem_table,omitempty"`
	ProjectEntries    []string         `json:"project_entries,omitempty"` // ProjectInformation ZIP
	Model             *ModelSummary    `json:"model,omitempty"`

	// M3: partition containers (Partitions/<n>).
	Partitions []PartitionInfo `json:"partitions,omitempty"`

	BFIError string `json:"bfi_error,omitempty"`
}

// OpenCFB exposes the underlying container for callers that need raw stream
// access (tests, advanced tooling).
func OpenCFB(data []byte) (*cfb.File, error) {
	return cfb.Open(data)
}

// OpenFile reads path and returns identification metadata.
func OpenFile(path string) (*Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	md, err := Open(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return md, nil
}

// Open identifies data as an RVT (CFB) container and extracts metadata.
// A non-CFB input is rejected; a CFB without BasicFileInfo is reported with
// BFIError set rather than failing (workshared/sync files may differ).
func Open(data []byte) (*Metadata, error) {
	f, err := cfb.Open(data)
	if err != nil {
		return nil, fmt.Errorf("not an RVT/CFB container: %w", err)
	}
	md := &Metadata{
		Format:   "rvt",
		FileSize: int64(len(data)),
	}
	for _, p := range f.StreamPaths() {
		sz, _ := f.Size(p)
		md.Streams = append(md.Streams, StreamInfo{Path: p, Size: sz})
	}
	sort.Slice(md.Streams, func(i, j int) bool { return md.Streams[i].Path < md.Streams[j].Path })

	if b, err := f.Stream("BasicFileInfo"); err == nil && len(b) > 0 {
		fi, perr := bfi.Parse(b)
		if perr != nil {
			md.BFIError = perr.Error()
		} else {
			md.BasicFileInfo = fi
			md.RevitVersion = fi.SavedInVersion
			md.Build = fi.Build
			md.Workshared = fi.IsWorkshared
			md.Language = fi.Language
		}
	} else if err != nil {
		md.BFIError = "BasicFileInfo stream absent"
	}

	if b, err := f.Stream("TransmissionData"); err == nil && len(b) > 0 {
		md.Transmission = parseTransmission(b)
	}

	// M2: decompress table streams and parse the Contents frame.
	for _, sp := range f.StreamPaths() {
		if !isTableStream(sp) {
			continue
		}
		b, err := f.Stream(sp)
		if err != nil || len(b) == 0 {
			continue
		}
		info, data, derr := decompressTable(sp, b)
		if info != nil {
			md.Tables = append(md.Tables, *info)
		}
		if derr != nil {
			continue // per-stream Error field already recorded
		}
		if sp == "Global/PartitionTable" && data != nil {
			md.PartitionWorksets = uniqueStrings(parsePartitionNames(data))
		}
		if sp == "Global/ElemTable" && data != nil {
			if et, eerr := ParseElemTable(data); eerr == nil {
				info := et.Info()
				md.ElemTable = &info
			} else {
				md.ElemTable = &ElemTableInfo{Err: eerr.Error()}
			}
		}
	}
	sort.Slice(md.Tables, func(i, j int) bool { return md.Tables[i].Path < md.Tables[j].Path })

	if b, err := f.Stream("Contents"); err == nil && len(b) > 0 {
		recs, _ := parseContents(b) // partial records kept even on error
		md.ContentsRecords = recs
	}

	if b, err := f.Stream("ProjectInformation"); err == nil && len(b) > 0 {
		md.ProjectEntries = zipEntryNames(b) // R44: Atom project-parameter XML
	}

	// M3: partition containers, with version-aware class-index lookup for
	// the object-record walk (class indexes are renumbered per Revit year).
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup(md.RevitVersion, cls)
		return name
	}
	var biggest *Partition
	for _, sp := range f.StreamPaths() {
		if !bytes.HasPrefix([]byte(sp), []byte("Partitions/")) {
			continue
		}
		b, err := f.Stream(sp)
		if err != nil || len(b) == 0 {
			continue
		}
		name := sp[len("Partitions/"):]
		if part, perr := ReadPartitionWithLookup(name, b, lookup); part != nil {
			md.Partitions = append(md.Partitions, part.Info)
			if biggest == nil || part.Info.LogicalBytes > biggest.Info.LogicalBytes {
				biggest = part
			}
			_ = perr
		}
	}
	if biggest != nil {
		m := biggest.Model(md.RevitVersion)
		withBBox := 0
		for _, e := range m.Elements {
			if e.BBox != nil {
				withBBox++
			}
		}
		md.Model = &ModelSummary{
			Elements: len(m.Elements),
			Views:    len(m.Views),
			BReps:    len(m.BReps),
			GTrees:   len(m.Trees),
			WithBBox: withBBox,
			Curves:      len(m.Curves),
			Arcs:        len(m.Arcs),
			BoundCurves: len(m.BoundCurves),
			BoundArcs:   len(m.BoundArcs),
		}
	}
	sort.Slice(md.Partitions, func(i, j int) bool { return md.Partitions[i].Name < md.Partitions[j].Name })
	return md, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

var transmissionVersionRe = regexp.MustCompile(`<TransmissionData\b[^>]*\bversion="([^"]+)"`)

func parseTransmission(b []byte) *TransmissionInfo {
	if len(b) < 4 {
		return nil
	}
	n := binary.LittleEndian.Uint32(b[:4])
	if uint64(4)+uint64(n)*2 > uint64(len(b)) {
		// tolerate: decode what is there
		n = uint32((len(b) - 4) / 2)
	}
	units := make([]uint16, 0, n)
	raw := b[4:]
	for i := 0; i+1 < len(raw) && uint32(i/2) < n; i += 2 {
		units = append(units, binary.LittleEndian.Uint16(raw[i:]))
	}
	xml := string(utf16.Decode(units))
	ti := &TransmissionInfo{XML: xml}
	if m := transmissionVersionRe.FindStringSubmatch(xml); m != nil {
		ti.Version = m[1]
	}
	return ti
}

// zipEntryNames lists entry names of a standard ZIP stream
// (ProjectInformation = one Atom project-parameter XML — findings R44).
func zipEntryNames(b []byte) []string {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(zr.File))
	for _, e := range zr.File {
		out = append(out, e.Name)
	}
	return out
}

// ModelSummary is the JSON summary of the assembled object model
// (full model via Partition.Model / rvt/geom).
type ModelSummary struct {
	Elements int `json:"elements"`
	Views    int `json:"views,omitempty"`
	BReps    int `json:"breps,omitempty"`
	GTrees   int `json:"g_trees,omitempty"`
	WithBBox int `json:"with_bbox,omitempty"`
	Curves   int `json:"curves,omitempty"`
	Arcs     int `json:"arcs,omitempty"`
	BoundCurves int `json:"bound_curves,omitempty"`
	BoundArcs   int `json:"bound_arcs,omitempty"`
}
