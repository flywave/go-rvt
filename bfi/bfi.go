// Package bfi parses the Revit BasicFileInfo stream (CFB root stream).
//
// Layout observed on Revit 2022/2023 samples (stream revision 14) and
// cross-checked against ODA BimRv BmBasicFileInfo.h getters:
//
//	u32 revision; 10B reserved
//	u32 n + UTF-16LE SavedInVersion / Build / LastSavePath
//	u32 tag; u8 worksharedFlag
//	u32 n + UTF-16LE CentralModelIdentity GUID
//	u32 n + UTF-16LE Language; u8 marker(0)
//	u32 CMVerNum
//	u32 n + UTF-16LE CentralModelEpisodeGUID
//	u32 n + UTF-16LE UniqueDocumentGUID
//	u32 n + UTF-16LE UniqueDocumentIncrements (decimal string)
//	u32 n + UTF-16LE ModelIdentity GUID; u8 marker(0)
//	u32 n + UTF-16LE Author / ClientAppName
//	summary text block (ANSI CRLF + UTF-16LE, mixed-encoding tail)
package bfi

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// FileInfo is the structured content of a BasicFileInfo stream.
type FileInfo struct {
	Revision                uint32 `json:"revision"`
	SavedInVersion          string `json:"saved_in_version"`
	Build                   string `json:"build"`
	LastSavePath            string `json:"last_save_path"`
	Tag                     uint32 `json:"tag"`
	WorksharedFlag          uint8  `json:"workshared_flag"`
	CentralModelIdentity    string `json:"central_model_identity"`
	Language                string `json:"language"`
	CMVerNum                uint32 `json:"cm_ver_num"`
	CentralModelEpisodeGUID string `json:"central_model_episode_guid"`
	UniqueDocumentGUID      string `json:"unique_document_guid"`
	Increments              string `json:"unique_document_increments"`
	ModelIdentity           string `json:"model_identity"`
	Author                  string `json:"author"`
	ClientAppName           string `json:"client_app_name"`
	Summary                 string `json:"summary,omitempty"`

	IsWorkshared bool `json:"is_workshared"`
}

// Err indicates a malformed BasicFileInfo stream.
var Err = fmt.Errorf("bfi: malformed stream")

// RevisionAccept maps Revit version → the set of BasicFileInfo stream
// revisions that version writes (parallel-REA accept table — findings R58;
// both bundled samples are rev 14 / 2023 and 2022). Parse reports a
// RevisionMismatch when a stream's revision falls outside its version's set;
// parsing still succeeds (the rev-14 layout is attempted for all).
var RevisionAccept = map[string][]uint32{
	"2011": {3}, "2012": {3},
	"2013": {6}, "2014": {6},
	"2015": {9},
	"2016": {9, 10},
	"2017": {10}, "2018": {10}, "2019": {10},
	"2020": {13}, "2021": {13},
	"2022": {13, 14},
	"2023": {14}, "2024": {14}, "2025": {14}, "2026": {14},
}

// ErrRevisionMismatch reports a revision outside the version's accept set.
var ErrRevisionMismatch = fmt.Errorf("bfi: revision not in version accept set")

// CheckRevision validates fi.Revision against fi.SavedInVersion using the
// accept table. Unknown versions are accepted (table covers 2011–2026).
func CheckRevision(fi *FileInfo) error {
	accept, ok := RevisionAccept[year(fi.SavedInVersion)]
	if !ok {
		return nil
	}
	for _, r := range accept {
		if fi.Revision == r {
			return nil
		}
	}
	return fmt.Errorf("%w: rev %d for %s (accept %v)", ErrRevisionMismatch, fi.Revision, fi.SavedInVersion, accept)
}

func year(version string) string {
	for i := 0; i+4 <= len(version); i++ {
		if version[i] >= '0' && version[i] <= '9' {
			y := version[i : i+4]
			digits := true
			for k := 0; k < 4; k++ {
				if y[k] < '0' || y[k] > '9' {
					digits = false
					break
				}
			}
			if digits && y >= "2011" && y <= "2026" {
				return y
			}
		}
	}
	return version
}

// Parse walks the TLV layout. Only revision 14 is validated against samples;
// other revisions are attempted with the same layout and may fail.
func Parse(data []byte) (*FileInfo, error) {
	p := &parser{data: data}
	fi := &FileInfo{}
	var err error
	if fi.Revision, err = p.u32(); err != nil {
		return nil, fmt.Errorf("%w: %v", Err, err)
	}
	if err := p.skip(10); err != nil { // u64 + u16 reserved (observed zero)
		return nil, fmt.Errorf("%w: %v", Err, err)
	}
	if fi.Revision > 0 && fi.Revision <= 13 {
		return parseLegacy(p, fi)
	}
	if fi.SavedInVersion, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: SavedInVersion: %v", Err, err)
	}
	if fi.Build, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: Build: %v", Err, err)
	}
	if fi.LastSavePath, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: LastSavePath: %v", Err, err)
	}
	if fi.Tag, err = p.u32(); err != nil {
		return nil, fmt.Errorf("%w: tag: %v", Err, err)
	}
	if fi.WorksharedFlag, err = p.u8(); err != nil {
		return nil, fmt.Errorf("%w: workshared flag: %v", Err, err)
	}
	if fi.CentralModelIdentity, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: CentralModelIdentity: %v", Err, err)
	}
	if fi.Language, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: Language: %v", Err, err)
	}
	if _, err := p.u8(); err != nil { // marker (observed 0)
		return nil, fmt.Errorf("%w: marker1: %v", Err, err)
	}
	if fi.CMVerNum, err = p.u32(); err != nil {
		return nil, fmt.Errorf("%w: CMVerNum: %v", Err, err)
	}
	if fi.CentralModelEpisodeGUID, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: CMEpGUID: %v", Err, err)
	}
	if fi.UniqueDocumentGUID, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: UniqueDocumentGUID: %v", Err, err)
	}
	if fi.Increments, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: Increments: %v", Err, err)
	}
	if fi.ModelIdentity, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: ModelIdentity: %v", Err, err)
	}
	if _, err := p.u8(); err != nil { // marker (observed 0)
		return nil, fmt.Errorf("%w: marker2: %v", Err, err)
	}
	if fi.Author, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: Author: %v", Err, err)
	}
	if fi.ClientAppName, err = p.str(); err != nil {
		return nil, fmt.Errorf("%w: ClientAppName: %v", Err, err)
	}
	fi.Summary = decodeSummary(p.rest())
	fi.IsWorkshared = fi.WorksharedFlag != 0
	return fi, nil
}

type parser struct {
	data []byte
	off  int
}

func (p *parser) need(n int) error {
	if p.off+n > len(p.data) {
		return fmt.Errorf("truncated at %d (need %d, have %d)", p.off, n, len(p.data)-p.off)
	}
	return nil
}

func (p *parser) skip(n int) error {
	if err := p.need(n); err != nil {
		return err
	}
	p.off += n
	return nil
}

func (p *parser) u8() (uint8, error) {
	if err := p.need(1); err != nil {
		return 0, err
	}
	v := p.data[p.off]
	p.off++
	return v, nil
}

func (p *parser) u32() (uint32, error) {
	if err := p.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(p.data[p.off:])
	p.off += 4
	return v, nil
}

func (p *parser) str() (string, error) {
	n, err := p.u32()
	if err != nil {
		return "", err
	}
	if n > 1<<20 {
		return "", fmt.Errorf("string length %d too large", n)
	}
	if err := p.need(int(n) * 2); err != nil {
		return "", err
	}
	s := decodeUTF16(p.data[p.off : p.off+int(n)*2])
	p.off += int(n) * 2
	return s, nil
}

func (p *parser) rest() []byte {
	if p.off >= len(p.data) {
		return nil
	}
	return p.data[p.off:]
}

func decodeUTF16(b []byte) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		units = append(units, binary.LittleEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(units))
}

// decodeSummary best-effort decodes the Part2 text block: an ANSI CRLF prefix
// followed by UTF-16LE (with occasional embedded ANSI runs, e.g. "False").
func decodeSummary(b []byte) string {
	b = trimANSICRLF(b)
	if len(b) < 2 {
		return ""
	}
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	return strings.TrimSpace(decodeUTF16(b))
}

func trimANSICRLF(b []byte) []byte {
	for len(b) >= 2 && b[0] == 0x0d && b[1] == 0x0a {
		b = b[2:]
	}
	return b
}

// parseLegacy extracts the version string for rev <= 13 files (rev-10
// layout: strings [Version][Build][Path] then binary block — no workshared /
// GUID fields). SavedInVersion is the primary extraction target.
func parseLegacy(p *parser, fi *FileInfo) (*FileInfo, error) {
	var err error
	if fi.SavedInVersion, err = p.str(); err != nil {
		return fi, fmt.Errorf("%w: rev<=13 Version: %v", Err, err)
	}
	// best-effort: try one more string as path
	if fi.LastSavePath, err = p.str(); err != nil {
		fi.LastSavePath = ""
	}
	return fi, nil
}
