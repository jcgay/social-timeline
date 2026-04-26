package timeline

import "sort"

// MergeSort flattens one or more Post slices and returns them sorted
// chronologically (ascending). Ties are broken by platform name (alphabetical)
// then URL (alphabetical) for deterministic output.
func MergeSort(slices ...[]Post) []Post {
	var all []Post
	for _, s := range slices {
		all = append(all, s...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.Platform != b.Platform {
			return a.Platform < b.Platform
		}
		return a.URL < b.URL
	})
	return all
}
