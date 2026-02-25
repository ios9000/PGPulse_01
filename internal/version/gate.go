package version

// VersionRange defines a minimum and maximum PG version for a SQL variant.
type VersionRange struct {
	MinMajor int // inclusive
	MinMinor int // inclusive
	MaxMajor int // inclusive, 0 means no upper bound
	MaxMinor int // inclusive
}

// Contains returns true if the given PGVersion falls within this range.
func (r VersionRange) Contains(v PGVersion) bool {
	if !v.AtLeast(r.MinMajor, r.MinMinor) {
		return false
	}
	if r.MaxMajor == 0 {
		return true // no upper bound
	}
	if v.Major > r.MaxMajor {
		return false
	}
	if v.Major == r.MaxMajor && v.Minor > r.MaxMinor {
		return false
	}
	return true
}

// SQLVariant pairs a version range with a SQL query string.
type SQLVariant struct {
	Range VersionRange
	SQL   string
}

// Gate holds version-gated SQL variants for a single metric query.
// Variants are checked in order; the first matching range wins.
type Gate struct {
	Name     string
	Variants []SQLVariant
}

// Select returns the SQL query appropriate for the given PG version.
// Returns empty string and false if no variant matches.
func (g Gate) Select(v PGVersion) (string, bool) {
	for _, variant := range g.Variants {
		if variant.Range.Contains(v) {
			return variant.SQL, true
		}
	}
	return "", false
}
