package zg

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gzipBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecompressOneWithTrailer(t *testing.T) {
	payload := bytes.Repeat([]byte("RVT-table-payload."), 100)
	src := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, gzipBytes(t, payload)...)
	src = append(src, []byte("TRAILER-DATA-42")...)
	m, err := DecompressOne(src, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(m.Data, payload) {
		t.Fatalf("data mismatch: got %d bytes", len(m.Data))
	}
	if string(m.Trailer) != "TRAILER-DATA-42" {
		t.Fatalf("trailer: %q", m.Trailer)
	}
	if m.Start != 8 || m.End != len(src)-15 {
		t.Fatalf("extents: start=%d end=%d src=%d", m.Start, m.End, len(src))
	}
}

func TestDecompressOneRejectsNonGzip(t *testing.T) {
	if _, err := DecompressOne([]byte("plain text bytes here"), 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecompressOneDetectsCorruption(t *testing.T) {
	// Large highly-compressed payload: flipping a byte inside the huffman
	// bitstream must break decoding (stored-block payloads can absorb flips).
	src := gzipBytes(t, bytes.Repeat([]byte("aaaaaaaa"), 1024))
	src[20] ^= 0xFF
	if _, err := DecompressOne(src, 0); err == nil {
		t.Fatal("expected deflate error")
	}
}

// Revit writes non-standard gzip footers on some streams; a CRC/ISIZE
// mismatch must be recorded, not rejected.
func TestDecompressOneToleratesBadFooter(t *testing.T) {
	payload := []byte("footer will be wrong")
	src := gzipBytes(t, payload)
	src[len(src)-8] ^= 0xFF // corrupt stored CRC only
	m, err := DecompressOne(src, 0)
	if err != nil {
		t.Fatalf("expected tolerance, got %v", err)
	}
	if !bytes.Equal(m.Data, payload) {
		t.Fatal("data mismatch")
	}
	if m.FooterCRCOK {
		t.Fatal("FooterCRCOK should be false")
	}
}
