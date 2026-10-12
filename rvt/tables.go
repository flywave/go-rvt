package rvt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/flywave/go-rvt/internal/zg"
)

// TableInfo summarises one compressed table stream (Global/*, Formats/*).
type TableInfo struct {
	Path             string `json:"path"`
	HeaderHex        string `json:"header_hex"` // bytes before the gzip member
	CompressedBytes  int    `json:"compressed_bytes"`
	DecompressedSize int    `json:"decompressed_size"`
	TrailerBytes     int    `json:"trailer_bytes"`
	Error            string `json:"error,omitempty"`
}

// ContentsRecord is one framed record inside the Contents stream
// (magic 62 19 22 05, u32 field of unknown semantics, then usually a gzip
// member plus trailer bytes up to the next record).
type ContentsRecord struct {
	Offset          int      `json:"offset"`
	DeclaredLen     int      `json:"declared_len"` // u32 after magic; semantics [?]
	ActualLen       int      `json:"actual_len"`   // bytes consumed in-stream
	DecompressedLen int      `json:"decompressed_len,omitempty"`
	TrailerBytes    int      `json:"trailer_bytes,omitempty"`
	Strings         []string `json:"strings,omitempty"`
	GZIP            bool     `json:"gzip"`
}

var contentsMagic = []byte{0x62, 0x19, 0x22, 0x05}

// isTableStream reports whether path holds a gzip-compressed table
// (Global/* and Formats/*). Contents/Preview use the magic framing instead.
func isTableStream(path string) bool {
	return strings.HasPrefix(path, "Global/") || strings.HasPrefix(path, "Formats/")
}

// decompressTable splits a table stream into header + gzip member + trailer.
// On failure it still returns a TableInfo with Error populated so callers
// can report per-stream problems (e.g. a locally corrupted sample).
func decompressTable(path string, stream []byte) (*TableInfo, []byte, error) {
	idx := bytes.Index(stream, []byte{0x1f, 0x8b, 0x08})
	if idx < 0 {
		info := &TableInfo{Path: path, HeaderHex: fmt.Sprintf("%x", stream)}
		return info, nil, fmt.Errorf("rvt: %s: no gzip member", path)
	}
	m, err := zg.DecompressOne(stream, idx)
	info := &TableInfo{
		Path:            path,
		HeaderHex:       fmt.Sprintf("%x", stream[:idx]),
		CompressedBytes: 0,
	}
	if err != nil {
		info.Error = err.Error()
		return info, nil, fmt.Errorf("rvt: %s: %w", path, err)
	}
	info.CompressedBytes = m.End - m.Start
	info.DecompressedSize = len(m.Data)
	info.TrailerBytes = len(m.Trailer)
	return info, m.Data, nil
}

// parseContents walks the Contents magic framing. The u32 after the magic is
// NOT the payload length (observed larger than the remaining stream on
// Revit 2022 samples); after the field, a gzip member is expected. Record
// extent runs to the next magic or end of stream.
func parseContents(stream []byte) ([]ContentsRecord, error) {
	var recs []ContentsRecord
	pos := 0
	var firstErr error
	for pos < len(stream) {
		i := bytes.Index(stream[pos:], contentsMagic)
		if i < 0 {
			break
		}
		off := pos + i
		if off+8 > len(stream) {
			firstErr = fmt.Errorf("rvt: Contents: truncated record header at %d", off)
			break
		}
		declared := int(binary.LittleEndian.Uint32(stream[off+4:]))
		rec := ContentsRecord{Offset: off, DeclaredLen: declared}
		payload := stream[off+8:]
		if len(payload) >= 3 && bytes.Equal(payload[:3], []byte{0x1f, 0x8b, 0x08}) {
			m, err := zg.DecompressOne(payload, 0)
			if err != nil {
				// corrupt gzip inside Contents (e.g. damaged sample): keep the
				// record, note via strings empty, continue scanning after 8B
				rec.ActualLen = 8
				recs = append(recs, rec)
				pos = off + 8
				if firstErr == nil {
					firstErr = fmt.Errorf("rvt: Contents record @%d: %w", off, err)
				}
				continue
			}
			rec.GZIP = true
			rec.DecompressedLen = len(m.Data)
			rec.TrailerBytes = len(m.Trailer)
			rec.Strings = extractUTF16Strings(m.Data, 3)
			rec.ActualLen = 8 + (m.End - 0) + len(m.Trailer)
			recs = append(recs, rec)
			pos = off + rec.ActualLen
			continue
		}
		// non-gzip record: fall back to declared length, clamped
		n := declared
		if off+8+n > len(stream) {
			n = len(stream) - off - 8
		}
		rec.ActualLen = 8 + n
		recs = append(recs, rec)
		pos = off + rec.ActualLen
	}
	return recs, firstErr
}

// extractUTF16Strings scans for runs of printable UTF-16LE code units
// (little-endian pairs with a zero high byte), returning runs of at least
// minLen characters.
func extractUTF16Strings(b []byte, minLen int) []string {
	var out []string
	var cur []uint16
	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(utf16.Decode(cur)))
		}
		cur = cur[:0]
	}
	for i := 0; i+1 < len(b); i += 2 {
		u := binary.LittleEndian.Uint16(b[i:])
		if u >= 0x20 && u < 0x7f {
			cur = append(cur, u)
			continue
		}
		// allow digits/letters already covered; treat other ASCII printable via range above
		flush()
	}
	flush()
	return out
}

// parsePartitionNames extracts length-prefixed UTF-16 strings from a
// decompressed PartitionTable. Observed: workset names ("Workset1" on the
// 2023 sample, "Project Standards" on the 2022 sample).
func parsePartitionNames(b []byte) []string {
	var out []string
	for off := 0; off+4 <= len(b); {
		n := binary.LittleEndian.Uint32(b[off:])
		if n == 0 || n > 128 || off+4+int(n)*2 > len(b) {
			off++
			continue
		}
		units := make([]uint16, 0, n)
		ok := true
		for k := 0; k < int(n); k++ {
			u := binary.LittleEndian.Uint16(b[off+4+k*2:])
			if u < 0x20 || u >= 0x7f {
				ok = false
				break
			}
			units = append(units, u)
		}
		if ok && n >= 2 {
			out = append(out, string(utf16.Decode(units)))
			off += 4 + int(n)*2
			continue
		}
		off++
	}
	return out
}
