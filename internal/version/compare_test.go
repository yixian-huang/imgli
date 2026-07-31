package version

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.5.0", "v0.5.1", -1},
		{"0.5.1", "v0.5.1", 0},
		{"v0.6.0", "v0.5.9", 1},
		{"v1.0.0", "v0.9.9", 1},
	}
	for _, c := range cases {
		if got := CompareSemver(c.a, c.b); got != c.want {
			t.Errorf("CompareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
