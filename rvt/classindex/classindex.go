// Package classindex maps Revit serialization class indexes (Index4Enum,
// i16 on disk — findings R30/R33) to OdBm* class names.
//
// Class indexes are renumbered per Revit version: the same class carries
// different numbers across years (OdBmElementHeader = 1398@2023 but
// 1340@2022 and 1201@2015), so lookups must go through the file's
// RevitVersion.
//
// Tables were extracted from the ODA BimRv per-class generated readers
// (TB_Format{Year}Readers: …Index4Enum<N>…OdSmartPtr<I<len>OdBmClassName>…)
// for every year shipped in the SDK (2011–2026) — see
// analysis/06-class-index.md and R79/R80.
package classindex

import (
	"embed"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed data/*.csv
var dataFS embed.FS

var tables = map[string]map[int16]string{}

func init() {
	entries, err := dataFS.ReadDir("data")
	if err != nil {
		return
	}
	for _, e := range entries {
		year := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		b, err := dataFS.ReadFile("data/" + e.Name())
		if err != nil {
			continue
		}
		tables[year] = parse(string(b))
	}
}

func parse(csv string) map[int16]string {
	m := make(map[int16]string, 3000)
	for _, line := range strings.Split(csv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx, name, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(idx)
		if err != nil || n < -32768 || n > 32767 {
			continue
		}
		m[int16(n)] = name
	}
	return m
}

// Lookup resolves a class index for the given Revit version (e.g. "2023").
// version may carry suffixes; the leading 4-digit year is used. ok is false
// for unknown versions or indexes outside the extracted table.
func Lookup(version string, index int16) (name string, ok bool) {
	t, found := tables[year(version)]
	if !found {
		return "", false
	}
	name, ok = t[index]
	return name, ok
}

// Index is the reverse lookup: class name → index for the given Revit
// version. ok is false when the version or class name is unknown.
func Index(version, className string) (index int16, ok bool) {
	t, found := tables[year(version)]
	if !found {
		return 0, false
	}
	for idx, name := range t {
		if name == className {
			return idx, true
		}
	}
	return 0, false
}

// HasVersion reports whether a table exists for the given Revit version.
func HasVersion(version string) bool {
	_, found := tables[year(version)]
	return found
}

// Versions lists the covered years (sorted).
func Versions() []string {
	out := make([]string, 0, len(tables))
	for y := range tables {
		out = append(out, y)
	}
	sort.Strings(out)
	return out
}

func year(version string) string {
	for i := 0; i+4 <= len(version); i++ {
		if isDigits(version[i : i+4]) {
			y := version[i : i+4]
			if y >= "2011" && y <= "2026" {
				return y
			}
		}
	}
	return version
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
