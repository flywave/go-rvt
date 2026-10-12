package rvt

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXObjRef(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	b, _ := f.Stream("Partitions/13")
	p, _ := ReadPartition("13", b)
	var s102 *SubStream
	for _, ss := range p.SubStreams {
		if ss.Class == 102 {
			s102 = ss
		}
	}
	d := s102.Data
	lookup := func(cls int16) string {
		n, _ := classindex.Lookup("2023", cls)
		return n
	}
	gp, _ := classindex.Index("2023", "OdBmGPoint")

	// dump 3 GPoint[id=0] payloads fully with nested class annotation
	shown := 0
	for off := 0; off+6+64 <= len(d) && shown < 3; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gp) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		payload := d[off+6 : off+6+64]
		t.Logf("GPoint[0] @%d payload: %x", off, payload)
		line := ""
		for k := 0; k+2 <= 64; k += 2 {
			v := int16(binary.LittleEndian.Uint16(payload[k:]))
			if n := lookup(v); n != "" && v > 16 {
				line += " @" + itoa6(k) + "=" + n
			}
		}
		t.Logf("   nested:%s", line)
		shown++
	}
}

func itoa6(v int) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}
