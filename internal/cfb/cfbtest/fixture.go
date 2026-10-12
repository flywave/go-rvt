package cfbtest

import (
	"encoding/binary"
	"sort"
	"unicode/utf16"
)

const (
	sectFree = 0xFFFFFFFF
	sectEnd  = 0xFFFFFFFE
	sectFat  = 0xFFFFFFFD

	sectorSize   = 512
	miniSize     = 64
	miniCutoff   = 4096
	entriesPerS  = sectorSize / 128
	fatEntPerSec = sectorSize / 4
)

type planned struct {
	name  string
	data  []byte
	start uint32
	mini  bool
}

func Write(streams map[string][]byte) []byte {
	names := make([]string, 0, len(streams))
	for n := range streams {
		names = append(names, n)
	}
	sort.Strings(names)

	items := make([]planned, 0, len(names))
	for _, n := range names {
		items = append(items, planned{name: n, data: streams[n], mini: len(streams[n]) < miniCutoff})
	}

	nEntries := 1 + len(items)
	nDir := uint32((nEntries + entriesPerS - 1) / entriesPerS)
	dirStart := uint32(1)
	next := uint32(1) + nDir

	var miniBytes []byte
	miniStarts := map[string]uint32{}
	for i := range items {
		if !items[i].mini {
			continue
		}
		miniStarts[items[i].name] = uint32(len(miniBytes)) / uint32(miniSize)
		miniBytes = append(miniBytes, items[i].data...)
		if pad := (miniSize - len(items[i].data)%miniSize) % miniSize; pad > 0 {
			miniBytes = append(miniBytes, make([]byte, pad)...)
		}
	}
	miniSectCount := (len(miniBytes) + miniSize - 1) / miniSize

	for i := range items {
		if items[i].mini {
			items[i].start = miniStarts[items[i].name]
			continue
		}
		items[i].start = next
		next += uint32((len(items[i].data) + sectorSize - 1) / sectorSize)
	}

	nMiniFatSect := uint32(0)
	miniFatStart := uint32(sectEnd)
	if miniSectCount > 0 {
		nMiniFatSect = uint32((miniSectCount + fatEntPerSec - 1) / fatEntPerSec)
		miniFatStart = next
		next += nMiniFatSect
	}
	miniContStart := uint32(sectEnd)
	if miniSectCount > 0 {
		miniContStart = next
		next += uint32((len(miniBytes) + sectorSize - 1) / sectorSize)
	}
	total := next
	if total > fatEntPerSec {
		panic("cfbtest: fixture exceeds one FAT sector")
	}

	fat := make([]uint32, total)
	for i := range fat {
		fat[i] = sectFree
	}
	fat[0] = sectFat
	for s := uint32(0); s < nDir; s++ {
		fat[dirStart+s] = sectEnd
		if s+1 < nDir {
			fat[dirStart+s] = dirStart + s + 1
		}
	}
	for i := range items {
		if items[i].mini {
			continue
		}
		n := uint32((len(items[i].data) + sectorSize - 1) / sectorSize)
		for s := uint32(0); s < n; s++ {
			fat[items[i].start+s] = sectEnd
			if s+1 < n {
				fat[items[i].start+s] = items[i].start + s + 1
			}
		}
	}
	for s := uint32(0); s < nMiniFatSect; s++ {
		fat[int(miniFatStart+s)] = sectEnd
		if s+1 < nMiniFatSect {
			fat[int(miniFatStart+s)] = miniFatStart + s + 1
		}
	}
	if miniSectCount > 0 {
		n := uint32((len(miniBytes) + sectorSize - 1) / sectorSize)
		for s := uint32(0); s < n; s++ {
			fat[int(miniContStart+s)] = sectEnd
			if s+1 < n {
				fat[int(miniContStart+s)] = miniContStart + s + 1
			}
		}
	}

	miniFat := make([]uint32, miniSectCount)
	for i := range miniFat {
		miniFat[i] = sectFree
	}
	for i := range items {
		if !items[i].mini {
			continue
		}
		n := (len(items[i].data) + miniSize - 1) / miniSize
		for s := 0; s < n; s++ {
			id := int(items[i].start) + s
			miniFat[id] = sectEnd
			if s+1 < n {
				miniFat[id] = uint32(id + 1)
			}
		}
	}

	dir := make([]byte, nDir*sectorSize)
	rootChild := uint32(sectFree)
	if len(items) > 0 {
		rootChild = 1
	}
	writeDirEntry(dir[0:], "Root Entry", 5, sectFree, sectFree, rootChild, miniContStart, uint64(len(miniBytes)))
	for i := range items {
		off := (1 + i) * 128
		right := uint32(sectFree)
		if i+1 < len(items) {
			right = uint32(1 + i + 1)
		}
		writeDirEntry(dir[off:], items[i].name, 2, sectFree, right, sectFree, items[i].start, uint64(len(items[i].data)))
	}

	header := make([]byte, sectorSize)
	copy(header[0:], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	binary.LittleEndian.PutUint16(header[24:], 0x003E)
	binary.LittleEndian.PutUint16(header[26:], 3)
	binary.LittleEndian.PutUint16(header[28:], 0xFFFE)
	binary.LittleEndian.PutUint16(header[30:], 9)
	binary.LittleEndian.PutUint16(header[32:], 6)
	binary.LittleEndian.PutUint32(header[44:], 1)
	binary.LittleEndian.PutUint32(header[48:], dirStart)
	binary.LittleEndian.PutUint32(header[56:], miniCutoff)
	binary.LittleEndian.PutUint32(header[60:], miniFatStart)
	binary.LittleEndian.PutUint32(header[64:], nMiniFatSect)
	binary.LittleEndian.PutUint32(header[68:], sectEnd)
	for i := 0; i < 109; i++ {
		binary.LittleEndian.PutUint32(header[76+4*i:], sectFree)
	}
	binary.LittleEndian.PutUint32(header[76:], 0)

	out := make([]byte, 0, int(total+1)*sectorSize)
	out = append(out, header...)
	fatSec := make([]byte, sectorSize)
	for i, v := range fat {
		binary.LittleEndian.PutUint32(fatSec[4*i:], v)
	}
	out = append(out, fatSec...)
	out = append(out, dir...)
	for i := range items {
		if items[i].mini {
			continue
		}
		out = append(out, items[i].data...)
		if pad := (sectorSize - len(items[i].data)%sectorSize) % sectorSize; pad > 0 {
			out = append(out, make([]byte, pad)...)
		}
	}
	if nMiniFatSect > 0 {
		ms := make([]byte, nMiniFatSect*sectorSize)
		for i, v := range miniFat {
			binary.LittleEndian.PutUint32(ms[4*i:], v)
		}
		out = append(out, ms...)
	}
	if miniSectCount > 0 {
		out = append(out, miniBytes...)
		if pad := (sectorSize - len(miniBytes)%sectorSize) % sectorSize; pad > 0 {
			out = append(out, make([]byte, pad)...)
		}
	}
	return out
}

func writeDirEntry(b []byte, name string, typ byte, left, right, child, start uint32, size uint64) {
	u := utf16.Encode([]rune(name))
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[2*i:], r)
	}
	binary.LittleEndian.PutUint16(b[2*len(u):], 0)
	binary.LittleEndian.PutUint16(b[64:], uint16((len(u)+1)*2))
	b[66] = typ
	b[67] = 1
	binary.LittleEndian.PutUint32(b[68:], left)
	binary.LittleEndian.PutUint32(b[72:], right)
	binary.LittleEndian.PutUint32(b[76:], child)
	binary.LittleEndian.PutUint32(b[120:], start)
	binary.LittleEndian.PutUint32(b[124:], uint32(size))
}
