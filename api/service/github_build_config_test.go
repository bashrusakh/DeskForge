package service

import "testing"

// TestCompareSemver проверяет упорядочивание compareSemver для стабильных
// и pre-release версий: numeric > numeric-equal-non-numeric ("1.4.8" > "1.4.8-beta"),
// trailing-segments-равны только если non-zero, pre-release между равными numeric
// сегментами ("1.4.8-beta" < "1.4.8-beta.1").
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.8", "1.4.7", 1},
		{"1.4.7", "1.4.8", -1},
		{"1.4.8", "1.4.8", 0},
		{"1.4.8-beta", "1.4.8", -1},
		{"1.4.8", "1.4.8-beta", 1},
		{"1.4.8-beta", "1.4.8-beta.1", -1},
		{"1.4.8-beta.1", "1.4.8-beta", 1},
		{"1.4.8-beta", "1.4.8-beta", 0},
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		if got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
