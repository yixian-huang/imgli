package handler

import "strconv"

var AllowedThumbWidths = []int{120, 200, 240, 400, 480, 800, 960, 1600}

func ThumbWidthAllowed(n int) bool {
	for _, w := range AllowedThumbWidths {
		if w == n {
			return true
		}
	}
	return false
}

func ThumbWidthHint() string {
	if len(AllowedThumbWidths) == 0 {
		return ""
	}
	s := ""
	for i, w := range AllowedThumbWidths {
		switch {
		case i == 0:
			s = strconv.Itoa(w)
		case i == len(AllowedThumbWidths)-1:
			s += " 或 " + strconv.Itoa(w)
		default:
			s += "、" + strconv.Itoa(w)
		}
	}
	return s
}
