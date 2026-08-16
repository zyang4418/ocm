package observation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// NormalizeSections validates a section list and returns it sorted and
// de-duplicated. Only positive integers are accepted, matching the legacy
// observation section semantics; a nil input normalizes to an empty list.
func NormalizeSections(sections []int) ([]int, error) {
	if sections == nil {
		return []int{}, nil
	}
	set := make(map[int]struct{}, len(sections))
	for _, v := range sections {
		if v <= 0 {
			return nil, fmt.Errorf("sections must contain positive integers")
		}
		set[v] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out, nil
}

// BuildSectionsKey returns the canonical sha256 key of a section list. It backs
// the (observer, course, observe_date, sections_key) uniqueness constraint for
// observations without an occurrence. The list is sorted and de-duplicated
// first (matching the legacy build_sections_key, which normalizes before
// hashing), and the canonical JSON form ([1,2,3], no whitespace) matches the
// legacy output byte-for-byte.
func BuildSectionsKey(sections []int) string {
	set := make(map[int]struct{}, len(sections))
	for _, v := range sections {
		set[v] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)

	canonical, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
