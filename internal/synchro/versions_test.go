package synchro

import "testing"

func TestVersions(t *testing.T) {
	for _, c := range []struct {
		version, protocole, modele string
		attendu                    Versions
	}{
		{"1.2.1", "", "", Versions{"1.2.1", 1, 0}},
		{"1.2.2-test.8", "", "", Versions{"1.2.2-test.8", 1, 0}},
		{"1.2.2", "", "", Versions{"1.2.2", 1, 1}},
		{"dev 1a2b3c4", "", "", Versions{"dev 1a2b3c4", 1, 1}},
		{"", "", "", Versions{"", 1, 1}},
		{"1.4.0", "2", "5", Versions{"1.4.0", 2, 5}},
		{"1.4.0", "x", "0", Versions{"1.4.0", 1, 0}},
	} {
		if v := Lire(c.version, c.protocole, c.modele); v != c.attendu {
			t.Errorf("Lire(%q, %q, %q) = %+v, attendu %+v", c.version, c.protocole, c.modele, v, c.attendu)
		}
	}
	ici := Versions{"1.3.0", 1, 2}
	for _, c := range []struct {
		srv     Versions
		attendu Ecart
	}{
		{Versions{"1.3.1", 1, 2}, Accord},
		{Versions{"1.2.9", 1, 2}, Accord},
		{Versions{"1.4.0", 1, 3}, ServeurRecent},
		{Versions{"1.2.2", 1, 1}, BureauRecent},
		{Versions{"2.0.0", 2, 1}, ServeurRecent}, // le protocole l'emporte
		{Versions{"1.0.0", 0, 9}, BureauRecent},
	} {
		if e := Comparer(ici, c.srv); e != c.attendu {
			t.Errorf("Comparer(%+v) = %d, attendu %d", c.srv, e, c.attendu)
		}
	}
	if (Versions{Version: "v1.2.2"}).Affichee() != "1.2.2" || (Versions{}).Affichee() != "version inconnue" {
		t.Error("version affichée")
	}
}
