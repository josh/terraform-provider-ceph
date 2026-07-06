// Package cephvalues compares and formats Ceph option value spellings.
//
// Cluster config options (mon) are parsed at set time by strict C++
// parsers in src/common and stored canonicalized, while mgr module
// options are stored as raw strings and coerced typed at read time by
// src/mgr/PyUtil.cc. The two backends accept different spellings for the
// same option types (suffixes, bool synonyms, durations), so their
// helpers are deliberately separate.
package cephvalues

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// ClusterEqual reports whether two spellings mean the same cluster config
// value for a given option type, mirroring the upstream parsers
// (strict_strtob, strict_si_cast, strict_iec_cast, parse_timespan, stoull)
// and the canonical forms produced by Option::to_str.
func ClusterEqual(optType, a, b string) bool {
	if a == b {
		return true
	}

	switch optType {
	case "bool":
		av, aok := parseBool(a)
		bv, bok := parseBool(b)
		return aok && bok && av == bv
	case "int", "uint":
		av, aok := parseSIInt(a)
		bv, bok := parseSIInt(b)
		return aok && bok && av == bv
	case "size":
		av, aok := parseIECInt(a)
		bv, bok := parseIECInt(b)
		return aok && bok && av == bv
	case "float":
		af, aerr := strconv.ParseFloat(strings.TrimSpace(a), 64)
		bf, berr := strconv.ParseFloat(strings.TrimSpace(b), 64)
		// Stored values are rounded to six decimals (std::fixed).
		return aerr == nil && berr == nil &&
			strconv.FormatFloat(af, 'f', 6, 64) == strconv.FormatFloat(bf, 'f', 6, 64)
	case "secs":
		av, aok := parseTimespan(a)
		bv, bok := parseTimespan(b)
		return aok && bok && av == bv
	case "millisecs":
		// Parsed with stoull, which reads the leading digits and ignores
		// any trailing suffix ("2s" means 2 milliseconds).
		av, aok := parseLeadingUint(a)
		bv, bok := parseLeadingUint(b)
		return aok && bok && av == bv
	case "uuid":
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	case "addr":
		return addrEqual(a, b) || addrEqual(b, a)
	default:
		return false
	}
}

func parseBool(s string) (bool, bool) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "true") {
		return true, true
	}
	if strings.EqualFold(s, "false") {
		return false, true
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v != 0, true
	}
	return false, false
}

var scaleExponents = map[byte]int{'K': 1, 'M': 2, 'G': 3, 'T': 4, 'P': 5, 'E': 6}

// int/uint options take a single trailing decimal suffix: K/M/G/T/P/E
// multiply by powers of 1000, B multiplies by one.
func parseSIInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}
	if s == "" {
		return 0, false
	}

	suffix := s[len(s)-1]
	num := s[:len(s)-1]
	if suffix == 'B' {
		v, err := strconv.ParseInt(num, 10, 64)
		return v, err == nil
	}
	exp, ok := scaleExponents[suffix]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, false
	}
	return scaleInt(v, 1000, exp)
}

// size options take binary suffixes: K/M/G/T/P/E and Ki/Mi/... all multiply
// by powers of 1024, with an optional trailing B ("1K" = "1KiB" = 1024,
// "100B" = 100).
func parseIECInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}

	i := strings.LastIndexAny(s, "0123456789")
	if i < 0 {
		return 0, false
	}
	num, unit := s[:i+1], s[i+1:]

	if len(unit) > 1 && unit[len(unit)-1] == 'B' {
		unit = unit[:len(unit)-1]
	}
	if unit == "B" {
		v, err := strconv.ParseInt(num, 10, 64)
		return v, err == nil
	}
	if len(unit) == 0 || len(unit) > 2 || (len(unit) == 2 && unit[1] != 'i') {
		return 0, false
	}
	exp, ok := scaleExponents[unit[0]]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, false
	}
	return scaleInt(v, 1024, exp)
}

func scaleInt(v int64, base int64, exp int) (int64, bool) {
	for range exp {
		if v > math.MaxInt64/base || v < math.MinInt64/base {
			return 0, false
		}
		v *= base
	}
	return v, true
}

var timespanUnits = map[string]int64{
	"s": 1, "sec": 1, "second": 1, "seconds": 1,
	"m": 60, "min": 60, "minute": 60, "minutes": 60,
	"h": 3600, "hr": 3600, "hour": 3600, "hours": 3600,
	"d": 86400, "day": 86400, "days": 86400,
	"w": 604800, "wk": 604800, "week": 604800, "weeks": 604800,
	"mo": 2592000, "month": 2592000, "months": 2592000,
	"y": 31536000, "yr": 31536000, "year": 31536000, "years": 31536000,
}

// secs options are parsed by summing value/unit pairs ("1h 30m" = 5400); a
// bare trailing number counts as seconds.
func parseTimespan(s string) (int64, bool) {
	var total int64
	parsedAny := false

	pos := 0
	for pos < len(s) {
		for pos < len(s) && asciiSpace(s[pos]) {
			pos++
		}
		if pos >= len(s) {
			break
		}

		start := pos
		for pos < len(s) && asciiDigit(s[pos]) {
			pos++
		}
		if start == pos {
			return 0, false
		}
		v, err := strconv.ParseInt(s[start:pos], 10, 64)
		if err != nil {
			return 0, false
		}

		for pos < len(s) && asciiSpace(s[pos]) {
			pos++
		}

		start = pos
		for pos < len(s) && asciiAlpha(s[pos]) {
			pos++
		}
		if start != pos {
			mult, ok := timespanUnits[s[start:pos]]
			if !ok {
				return 0, false
			}
			v *= mult
		} else if pos < len(s) {
			return 0, false
		}

		total += v
		parsedAny = true
	}

	return total, parsedAny
}

func parseLeadingUint(s string) (uint64, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "+")

	end := 0
	for end < len(s) && asciiDigit(s[end]) {
		end++
	}
	if end == 0 {
		return 0, false
	}

	v, err := strconv.ParseUint(s[:end], 10, 64)
	return v, err == nil
}

// addr options are canonicalized through entity_addr_t, which adds a msgr2
// type prefix, default port, and nonce ("1.2.3.4" is stored as
// "v2:1.2.3.4:0/0"). Only exact decorated forms of the other spelling are
// recognized; anything else stays unequal.
func addrEqual(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == b {
		return true
	}

	candidates := []string{
		"v2:" + a,
		a + "/0",
		"v2:" + a + "/0",
		a + ":0/0",
		"v2:" + a + ":0/0",
	}
	return slices.Contains(candidates, b)
}

func asciiSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func asciiDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func asciiAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
