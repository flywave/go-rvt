package cfb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrNotCompoundFile = errors.New("cfb: not an OLE compound file")
	ErrCorrupt         = errors.New("cfb: corrupt structure")
	ErrStreamNotFound  = errors.New("cfb: stream not found")
)

const (
	sig0, sig1, sig2, sig3, sig4, sig5, sig6, sig7 = 0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1

	sectFree     = 0xFFFFFFFF
	sectEnd      = 0xFFFFFFFE
	sectFat      = 0xFFFFFFFD
	sectDifat    = 0xFFFFFFFC
	entryStorage = 1
	entryStream  = 2
	entryRoot    = 5

	maxFatEntries = 1 << 20
)

type entry struct {
	name        string
	typ         byte
	left, right int32
	child       int32
	start       uint32
	size        uint64
}

type File struct {
	data       []byte
	major      uint16
	sectorSize int
	miniSize   int
	cutoff     uint32
	fat        []uint32
	miniFat    []uint32
	entries    []entry
	nodes      []node
}

// node pairs a directory entry index with its full slash-separated path
// (e.g. "Global/ElemTable").
type node struct {
	idx  int
	path string
}

func Open(data []byte) (*File, error) {
	if len(data) < 512 {
		return nil, ErrNotCompoundFile
	}
	if data[0] != sig0 || data[1] != sig1 || data[2] != sig2 || data[3] != sig3 ||
		data[4] != sig4 || data[5] != sig5 || data[6] != sig6 || data[7] != sig7 {
		return nil, ErrNotCompoundFile
	}
	shift := binary.LittleEndian.Uint16(data[30:])
	if shift < 9 || shift > 12 {
		return nil, fmt.Errorf("%w: sector shift %d", ErrCorrupt, shift)
	}
	miniShift := binary.LittleEndian.Uint16(data[32:])
	if miniShift < 2 || miniShift > shift {
		return nil, fmt.Errorf("%w: mini shift %d", ErrCorrupt, miniShift)
	}
	f := &File{
		data:       data,
		major:      binary.LittleEndian.Uint16(data[26:]),
		sectorSize: 1 << shift,
		miniSize:   1 << miniShift,
		cutoff:     binary.LittleEndian.Uint32(data[56:]),
	}
	if len(data) < f.sectorSize {
		return nil, ErrNotCompoundFile
	}
	firstDir := binary.LittleEndian.Uint32(data[48:])
	firstMini := binary.LittleEndian.Uint32(data[60:])
	firstDifat := binary.LittleEndian.Uint32(data[68:])
	numDifat := binary.LittleEndian.Uint32(data[72:])

	difat := make([]uint32, 0, 109)
	for i := 0; i < 109; i++ {
		v := binary.LittleEndian.Uint32(data[76+4*i:])
		if v != sectFree {
			difat = append(difat, v)
		}
	}
	if firstDifat != sectEnd && numDifat > 0 {
		per := f.sectorSize/4 - 1
		cur := firstDifat
		for i := uint32(0); i < numDifat && cur != sectEnd && cur != sectFree; i++ {
			b, err := f.sector(cur)
			if err != nil {
				return nil, err
			}
			for j := 0; j < per; j++ {
				v := binary.LittleEndian.Uint32(b[4*j:])
				if v != sectFree {
					difat = append(difat, v)
				}
			}
			cur = binary.LittleEndian.Uint32(b[4*per:])
		}
	}
	if len(difat) > maxFatEntries {
		return nil, fmt.Errorf("%w: too many FAT sectors", ErrCorrupt)
	}
	f.fat = make([]uint32, 0, len(difat)*f.sectorSize/4)
	for _, id := range difat {
		b, err := f.sector(id)
		if err != nil {
			return nil, err
		}
		for j := 0; j < f.sectorSize/4; j++ {
			f.fat = append(f.fat, binary.LittleEndian.Uint32(b[4*j:]))
		}
	}
	if len(f.fat) == 0 {
		return nil, fmt.Errorf("%w: empty FAT", ErrCorrupt)
	}

	dirBytes, err := f.chain(firstDir)
	if err != nil {
		return nil, err
	}
	if len(dirBytes) < 128 {
		return nil, fmt.Errorf("%w: short directory", ErrCorrupt)
	}
	for off := 0; off+128 <= len(dirBytes); off += 128 {
		e, err := parseEntry(dirBytes[off:off+128], f.major)
		if err != nil {
			return nil, err
		}
		f.entries = append(f.entries, e)
	}

	if firstMini != sectEnd && firstMini != sectFree {
		mb, err := f.chain(firstMini)
		if err != nil {
			return nil, err
		}
		f.miniFat = make([]uint32, len(mb)/4)
		for i := range f.miniFat {
			f.miniFat[i] = binary.LittleEndian.Uint32(mb[4*i:])
		}
	}
	f.nodes = f.walk()
	return f, nil
}

func parseEntry(b []byte, major uint16) (entry, error) {
	var e entry
	nameLen := int(binary.LittleEndian.Uint16(b[64:]))
	if b[66] == 0 && nameLen == 0 {
		return e, nil
	}
	if nameLen < 2 || nameLen > 64 || nameLen%2 != 0 {
		return e, fmt.Errorf("%w: name length %d", ErrCorrupt, nameLen)
	}
	e.name = decodeUTF16(b[:nameLen-2])
	e.typ = b[66]
	idx := func(off int) int32 {
		v := binary.LittleEndian.Uint32(b[off:])
		if v == sectFree {
			return -1
		}
		return int32(v)
	}
	e.left = idx(68)
	e.right = idx(72)
	e.child = idx(76)
	if major >= 4 {
		e.start = binary.LittleEndian.Uint32(b[116:])
		e.size = binary.LittleEndian.Uint64(b[120:])
	} else {
		e.start = binary.LittleEndian.Uint32(b[120:])
		e.size = uint64(binary.LittleEndian.Uint32(b[124:]))
	}
	return e, nil
}

func decodeUTF16(b []byte) string {
	s := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i:])
		if v == 0 {
			break
		}
		s = append(s, rune(v))
	}
	return string(s)
}

// walk traverses the directory red-black trees recursively, producing every
// storage and stream entry with a full path rooted at "".
func (f *File) walk() []node {
	root := -1
	for i, e := range f.entries {
		if e.typ == entryRoot {
			root = i
			break
		}
	}
	if root < 0 {
		return nil
	}
	var out []node
	var visitTree func(idx int32, prefix string)
	visitTree = func(idx int32, prefix string) {
		if idx < 0 || int(idx) >= len(f.entries) {
			return
		}
		e := f.entries[idx]
		if e.typ != entryStorage && e.typ != entryStream {
			return
		}
		visitTree(e.left, prefix)
		path := e.name
		if prefix != "" {
			path = prefix + "/" + e.name
		}
		out = append(out, node{idx: int(idx), path: path})
		if e.typ == entryStorage {
			visitTree(e.child, path)
		}
		visitTree(e.right, prefix)
	}
	visitTree(f.entries[root].child, "")
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func (f *File) sector(id uint32) ([]byte, error) {
	off := f.sectorSize + int(id)*f.sectorSize
	if off < f.sectorSize || off > len(f.data) {
		return nil, fmt.Errorf("%w: sector %d out of range", ErrCorrupt, id)
	}
	end := off + f.sectorSize
	if end > len(f.data) {
		return nil, fmt.Errorf("%w: sector %d truncated", ErrCorrupt, id)
	}
	return f.data[off:end], nil
}

func (f *File) chain(start uint32) ([]byte, error) {
	var out []byte
	seen := make(map[uint32]bool)
	for cur := start; cur != sectEnd && cur != sectFree; {
		if seen[cur] {
			return nil, fmt.Errorf("%w: FAT loop at %d", ErrCorrupt, cur)
		}
		seen[cur] = true
		b, err := f.sector(cur)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
		if int(cur) >= len(f.fat) {
			return nil, fmt.Errorf("%w: FAT index %d", ErrCorrupt, cur)
		}
		cur = f.fat[cur]
		if len(out) > len(f.fat)*f.sectorSize+f.sectorSize {
			return nil, fmt.Errorf("%w: chain too long", ErrCorrupt)
		}
	}
	return out, nil
}

func (f *File) miniChain(start uint32, size uint64) ([]byte, error) {
	root := -1
	for i, e := range f.entries {
		if e.typ == entryRoot {
			root = i
			break
		}
	}
	if root < 0 {
		return nil, fmt.Errorf("%w: no root", ErrCorrupt)
	}
	container, err := f.chain(f.entries[root].start)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, size)
	seen := make(map[uint32]bool)
	for cur := start; cur != sectEnd && cur != sectFree && uint64(len(out)) < size; {
		if seen[cur] {
			return nil, fmt.Errorf("%w: miniFAT loop at %d", ErrCorrupt, cur)
		}
		seen[cur] = true
		off := int(cur) * f.miniSize
		if off+f.miniSize > len(container) {
			return nil, fmt.Errorf("%w: mini sector %d out of range", ErrCorrupt, cur)
		}
		out = append(out, container[off:off+f.miniSize]...)
		if int(cur) >= len(f.miniFat) {
			return nil, fmt.Errorf("%w: miniFAT index %d", ErrCorrupt, cur)
		}
		cur = f.miniFat[cur]
	}
	if uint64(len(out)) > size {
		out = out[:size]
	}
	return out, nil
}

// Paths returns every storage and stream entry, sorted, with full
// slash-separated paths (root-level entries have no prefix).
func (f *File) Paths() []string {
	out := make([]string, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, n.path)
	}
	return out
}

// StreamPaths returns only stream entry paths, sorted.
func (f *File) StreamPaths() []string {
	out := make([]string, 0, len(f.nodes))
	for _, n := range f.nodes {
		if f.entries[n.idx].typ == entryStream {
			out = append(out, n.path)
		}
	}
	return out
}

func (f *File) lookup(path string) (int, bool) {
	for _, n := range f.nodes {
		if n.path == path {
			return n.idx, true
		}
	}
	return 0, false
}

func (f *File) Size(path string) (uint64, bool) {
	i, ok := f.lookup(path)
	if !ok {
		return 0, false
	}
	return f.entries[i].size, true
}

func (f *File) IsStorage(path string) bool {
	i, ok := f.lookup(path)
	if !ok {
		return false
	}
	return f.entries[i].typ == entryStorage
}

func (f *File) Stream(path string) ([]byte, error) {
	i, ok := f.lookup(path)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrStreamNotFound, path)
	}
	e := f.entries[i]
	if e.typ != entryStream {
		return nil, fmt.Errorf("%w: %q is a storage", ErrStreamNotFound, path)
	}
	if e.size == 0 {
		return []byte{}, nil
	}
	if e.size < uint64(f.cutoff) {
		return f.miniChain(e.start, e.size)
	}
	b, err := f.chain(e.start)
	if err != nil {
		return nil, err
	}
	if uint64(len(b)) < e.size {
		return nil, fmt.Errorf("%w: stream %q shorter than declared", ErrCorrupt, path)
	}
	return b[:e.size], nil
}
