// Package zg decompresses the single-gzip-member framing used by RVT
// Global/* and Formats/* streams: an optional fixed header, one complete
// gzip member (header+deflate+CRC32+ISIZE), then an opaque trailer.
package zg

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// Member holds one decompressed gzip member and its byte extent in the
// source buffer.
type Member struct {
	Data      []byte // decompressed payload
	Start     int    // offset of the gzip header in the source
	End       int    // offset just past the gzip footer (CRC32+ISIZE)
	Trailer   []byte // bytes after the member, up to len(src)
	TrailerAt int

	// FooterCRCOK / FooterSizeOK report whether the standard gzip footer
	// matches the inflated payload. Revit writes non-standard footers on
	// some large streams (observed on Latest/Formats/ContentDocuments for
	// both 2022 and 2023 samples), so mismatches are recorded rather than
	// treated as fatal.
	FooterCRCOK  bool
	FooterSizeOK bool
}

// ErrNotGzip indicates the source does not hold a gzip member at the offset.
var ErrNotGzip = fmt.Errorf("zg: not a gzip member")

// DecompressOne parses a single gzip member starting at src[start].
// Deflate integrity (valid huffman stream with a final block) is enforced;
// the CRC32/ISIZE footer is checked but a mismatch is not an error (see
// Member.FooterCRCOK/FooterSizeOK).
func DecompressOne(src []byte, start int) (*Member, error) {
	off, err := headerLen(src, start)
	if err != nil {
		return nil, err
	}
	cr := &countingByteReader{r: bytes.NewReader(src[off:])}
	fr := flate.NewReader(cr)
	data, err := io.ReadAll(fr)
	if err != nil {
		return nil, fmt.Errorf("zg: deflate at %d: %w", start, err)
	}
	if err := fr.Close(); err != nil {
		return nil, fmt.Errorf("zg: deflate close at %d: %w", start, err)
	}
	deflateEnd := off + int(cr.n)
	if deflateEnd+8 > len(src) {
		return nil, fmt.Errorf("zg: truncated gzip footer at %d", deflateEnd)
	}
	wantCRC := binary.LittleEndian.Uint32(src[deflateEnd:])
	wantSize := binary.LittleEndian.Uint32(src[deflateEnd+4:])
	crcOK := crc32.ChecksumIEEE(data) == wantCRC
	sizeOK := uint32(len(data)) == wantSize
	end := deflateEnd + 8
	return &Member{
		Data:         data,
		Start:        start,
		End:          end,
		Trailer:      src[end:],
		TrailerAt:    end,
		FooterCRCOK:  crcOK,
		FooterSizeOK: sizeOK,
	}, nil
}

// headerLen returns the offset of the raw deflate stream given the gzip
// header at start.
func headerLen(src []byte, start int) (int, error) {
	if start < 0 || start+10 > len(src) || src[start] != 0x1f || src[start+1] != 0x8b || src[start+2] != 0x08 {
		return 0, fmt.Errorf("%w %d", ErrNotGzip, start)
	}
	flg := src[start+3]
	off := start + 10
	if flg&4 != 0 { // FEXTRA
		if off+2 > len(src) {
			return 0, fmt.Errorf("zg: truncated FEXTRA")
		}
		xlen := int(src[off]) | int(src[off+1])<<8
		off += 2 + xlen
	}
	if flg&8 != 0 { // FNAME
		i := bytes.IndexByte(src[off:], 0)
		if i < 0 {
			return 0, fmt.Errorf("zg: unterminated FNAME")
		}
		off += i + 1
	}
	if flg&16 != 0 { // FCOMMENT
		i := bytes.IndexByte(src[off:], 0)
		if i < 0 {
			return 0, fmt.Errorf("zg: unterminated FCOMMENT")
		}
		off += i + 1
	}
	if flg&2 != 0 { // FHCRC
		off += 2
	}
	if off > len(src) {
		return 0, fmt.Errorf("zg: truncated gzip header")
	}
	return off, nil
}

// countingByteReader implements io.ByteReader so compress/flate drives it
// directly instead of wrapping it in a read-ahead bufio.Reader; this keeps
// the consumed count exact at the deflate-stream boundary.
type countingByteReader struct {
	r *bytes.Reader
	n int64
}

func (c *countingByteReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *countingByteReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}
