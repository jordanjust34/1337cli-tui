package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

// parseSelection turns --get specs into 1-based result numbers, in the order
// given with duplicates dropped. Each spec may hold comma-separated items:
// a number ("3"), an inclusive range ("2-4") or "all". pflag already splits
// "-g 1,3" into ["1","3"], but unsplit input is accepted too.
func parseSelection(specs []string, n int) ([]int, error) {
	var out []int
	seen := map[int]bool{}
	add := func(i int) error {
		if i < 1 || i > n {
			return fmt.Errorf("--get %d: no such result (have 1-%d)", i, n)
		}
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
		return nil
	}
	for _, spec := range specs {
		for _, item := range strings.Split(spec, ",") {
			item = strings.TrimSpace(item)
			if strings.EqualFold(item, "all") {
				for i := 1; i <= n; i++ {
					add(i) // always in range
				}
				continue
			}
			lo, hi, isRange := strings.Cut(item, "-")
			from, err := parseResultNumber(item, lo)
			if err != nil {
				return nil, err
			}
			to := from
			if isRange {
				if to, err = parseResultNumber(item, hi); err != nil {
					return nil, err
				}
				if to < from {
					return nil, fmt.Errorf("--get %s: range is backwards, did you mean %d-%d?", item, to, from)
				}
			}
			for i := from; i <= to; i++ {
				if err := add(i); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

// parseResultNumber parses one end of a --get item; item is only used in the error.
func parseResultNumber(item, s string) (int, error) {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || i < 1 {
		return 0, fmt.Errorf("--get %q: expected a result number (1 or greater), a range like 2-5, or all", item)
	}
	return i, nil
}
