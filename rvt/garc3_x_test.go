package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGArcDiff(t *testing.T) {
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
	ga, _ := classindex.Index("2023", "OdBmGArc")
	const PL = 96
	var payloads [][PL]byte
	for off := 0; off+6+PL <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		payloads = append(payloads, *(*[PL]byte)(d[off+6 : off+6+PL]))
		off += PL
	}
	t.Logf("GArc[0] payloads: %d", len(payloads))
	if len(payloads) < 4 {
		t.Fatal("too few")
	}
	// column stats: f64 columns at 8-byte alignment
	for c := 0; c < PL/8; c++ {
		distinct := map[uint64]int{}
		for _, pl := range payloads {
			v := binary.LittleEndian.Uint64(pl[c*8:])
			distinct[v]++
		}
		// top 2
		type kv struct {
			k uint64
			v int
		}
		var l []kv
		for k, v := range distinct {
			l = append(l, kv{k, v})
		}
		best := kv{0, 0}
		second := kv{0, 0}
		for _, e := range l {
			if e.v > best.v {
				second = best
				best = e
			} else if e.v > second.v {
				second = e
			}
		}
		fb := math.Float64frombits(best.k)
		fs := math.Float64frombits(second.k)
		t.Logf("f64 col %2d (off %3d): distinct %4d top=%.6g x%d (%.0f%%) second=%.6g x%d",
			c, c*8, len(distinct), fb, best.v, 100*float64(best.v)/float64(len(payloads)), fs, second.v)
	}
	// content-anchored scan: XDir(unit) YDir(unit, dot~0) Radius(0,1e4) Center(3xf64)
	found := 0
	for off := 0; off+8*10 <= len(d) && found < 6; off += 2 {
		var f [10]float64
		ok := true
		for k := 0; k < 10; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(d[off+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		xm := math.Sqrt(f[0]*f[0] + f[1]*f[1] + f[2]*f[2])
		ym := math.Sqrt(f[3]*f[3] + f[4]*f[4] + f[5]*f[5])
		dot := f[0]*f[3] + f[1]*f[4] + f[2]*f[5]
		if xm < 0.99 || xm > 1.01 || ym < 0.99 || ym > 1.01 || math.Abs(dot) > 0.01 {
			continue
		}
		if f[6] <= 1e-6 || f[6] > 1e4 {
			continue
		}
		found++
		if found <= 3 {
			t.Logf("arc-like @%d: X=(%.4f,%.4f,%.4f) Y=(%.4f,%.4f,%.4f) R=%.4f C=(%.4f,%.4f,%.4f)",
				off, f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9])
		}
		off += 80
	}
	t.Logf("content-anchored arcs: %d", found)
	if found > 0 {
		t.Logf("first arc bytes[off-24..off]: %x", func() []byte {
			for off := 0; off+80 <= len(d); off += 2 {
				var f [10]float64
				for k := 0; k < 10; k++ {
					f[k] = math.Float64frombits(binary.LittleEndian.Uint64(d[off+k*8:]))
				}
				xm := math.Sqrt(f[0]*f[0] + f[1]*f[1] + f[2]*f[2])
				ym := math.Sqrt(f[3]*f[3] + f[4]*f[4] + f[5]*f[5])
				dot := f[0]*f[3] + f[1]*f[4] + f[2]*f[5]
				if xm > 0.99 && xm < 1.01 && ym > 0.99 && ym < 1.01 && math.Abs(dot) < 0.01 && f[6] > 1e-6 && f[6] < 1e4 {
					lo := off - 24
					if lo < 0 {
						lo = 0
					}
					return d[lo : off+80]
				}
			}
			return nil
		}())
	}

	// duplicate payloads
	seen := map[string]int{}
	dups := 0
	for _, pl := range payloads {
		seen[string(pl[:])]++
	}
	for _, v := range seen {
		if v > 1 {
			dups += v
		}
	}
	t.Logf("duplicate-payload records: %d of %d", dups, len(payloads))
}
