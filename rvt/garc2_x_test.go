package rvt

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGArc2(t *testing.T) {
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
	ga, _ := classindex.Index("2023", "OdBmGArc")
	shown := 0
	for off := 0; off+6+96 <= len(d) && shown < 5; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		payload := d[off+6 : off+6+96]
		t.Logf("GArc[0] @%d payload 96B: %x", off, payload)
		// annotate plausible class i16s
		line := ""
		for k := 0; k+2 <= 96; k += 2 {
			v := int16(binary.LittleEndian.Uint16(payload[k:]))
			if n := lookup(v); n != "" && v > 16 {
				line += " @" + itoa5(k) + "=" + n
			}
		}
		t.Logf("   classes:%s", line)
		shown++
	}
}

func itoa5(v int) string {
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
