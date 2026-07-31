package version

import (
	"strconv"
	"strings"
)

// NormalizeTag 去掉空白与可选 v/V 前缀。
func NormalizeTag(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	return s
}

// CompareSemver 比较 a 与 b（可带 v 前缀）。返回 -1/0/1；无法解析时按字符串比较。
func CompareSemver(a, b string) int {
	ap := parseParts(NormalizeTag(a))
	bp := parseParts(NormalizeTag(b))
	if ap == nil || bp == nil {
		return strings.Compare(NormalizeTag(a), NormalizeTag(b))
	}
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(ap) {
			ai = ap[i]
		}
		if i < len(bp) {
			bi = bp[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func parseParts(s string) []int {
	if s == "" || s == "dev" {
		return nil
	}
	// strip pre-release / build metadata for coarse compare
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	segs := strings.Split(s, ".")
	out := make([]int, 0, len(segs))
	for _, p := range segs {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}
