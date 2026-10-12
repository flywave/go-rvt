// Package classindex maps Revit serialization class indexes (Index4Enum,
// i16 on disk — findings R30/R33) to OdBm* class names.
//
// Class indexes are renumbered per Revit version: the same class carries
// different numbers across years (OdBmElementHeader = 1398@2023 but
// 1340@2022), so lookups must go through the file's RevitVersion.
//
// Tables were extracted from the ODA BimRv per-class generated readers
// (TB_Format{Year}Readers: …Index4Enum<N>…OdSmartPtr<I<OdBmClassName>>…)
// — see analysis/06-class-index.md. 2023: 2845 classes, 2022: 2761.
package classindex

import (
	_ "embed"
	"strconv"
	"strings"
)

//go:embed data/2023.csv
var csv2023 string

//go:embed data/2022.csv
var csv2022 string

var tables = map[string]map[int16]string{}

func init() {
	tables["2023"] = parse(csv2023)
	tables["2022"] = parse(csv2022)
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

// HasVersion reports whether a table exists for the given Revit version.
func HasVersion(version string) bool {
	_, found := tables[year(version)]
	return found
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

// Index is the reverse lookup: class name → index for the given Revit
// version. ok is false when the version or class name is unknown.
func Index(version, className string) (index int16, ok bool) {
	t, found := tables[year(version)]
	if !found {
		return 0, false
	}
	// tables are small (≈3k entries); linear scan on demand
	for idx, name := range t {
		if name == className {
			return idx, true
		}
	}
	return 0, false
}
