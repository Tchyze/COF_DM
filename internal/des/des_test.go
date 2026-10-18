package des

import (
	"strings"
	"testing"
)

// suite : les dés tombent dans cet ordre.
func suite(t *testing.T, v ...int) {
	t.Helper()
	ancien := De
	t.Cleanup(func() { De = ancien })
	i := 0
	De = func(f int) int {
		if i >= len(v) {
			t.Fatalf("dé de trop (d%d)", f)
		}
		i++
		return v[i-1]
	}
}

func TestLancer(t *testing.T) {
	cas := []struct {
		formule string
		des     []int
		total   int
		propre  string
		detail  string
		nat     int
	}{
		{"1d20+3", []int{14}, 17, "1d20 + 3", "1d20[14] + 3", 14},
		{" D20 ", []int{20}, 20, "1d20", "1d20[20]", 20},
		{"2d6-1", []int{3, 5}, 7, "2d6 - 1", "2d6[3,5] − 1", 0},
		{"2d20kh1+5", []int{4, 17}, 22, "2d20kh1 + 5", "2d20kh1[(4),17] + 5", 17},
		{"2d20k+5", []int{4, 17}, 22, "2d20kh1 + 5", "2d20kh1[(4),17] + 5", 17},
		{"2d20kl1", []int{4, 17}, 4, "2d20kl1", "2d20kl1[4,(17)]", 4},
		{"4d6kh3", []int{2, 6, 2, 5}, 13, "4d6kh3", "4d6kh3[2,6,(2),5]", 0},
		{"1d8+1d6-2", []int{8, 1}, 7, "1d8 + 1d6 - 2", "1d8[8] + 1d6[1] − 2", 8},
		{"3-1d4", []int{4}, -1, "3 - 1d4", "3 − 1d4[4]", 0},
	}
	for _, c := range cas {
		suite(t, c.des...)
		r, err := Lancer(c.formule)
		if err != nil {
			t.Fatalf("%s : %v", c.formule, err)
		}
		if r.Total != c.total || r.Formule != c.propre || r.Detail != c.detail || r.Nat != c.nat {
			t.Fatalf("%s : %+v", c.formule, r)
		}
	}
}

func TestFormulesRefusees(t *testing.T) {
	for _, f := range []string{"", "abc", "d", "1d1", "0d6", "1d20++3", "2d20kh3", "2d20kl0", "101d6", "1d1001",
		"5", "1d6+" + strings.Repeat("1+", 20) + "1", "1d20*2", "1d20 + x"} {
		if _, err := Lancer(f); err == nil {
			t.Fatalf("%q devrait être refusée", f)
		}
	}
	// au plus 100 dés en tout, sur toute la formule
	if _, err := Lancer("60d6+41d6"); err == nil {
		t.Fatal("101 dés en deux groupes devraient être refusés")
	}
}

func TestBornes(t *testing.T) {
	for range 500 {
		r, err := Lancer("3d6")
		if err != nil || r.Total < 3 || r.Total > 18 {
			t.Fatalf("3d6 : %+v %v", r, err)
		}
	}
}
