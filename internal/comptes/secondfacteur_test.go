package comptes

import (
	"bytes"
	"encoding/base32"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Les vecteurs de la RFC 6238, annexe B, pour SHA-1 (8 chiffres dans la RFC).
func TestTOTPVecteursRFC6238(t *testing.T) {
	cle := []byte("12345678901234567890")
	for _, v := range []struct {
		t      int64
		attend string
	}{
		{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"},
		{1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"},
	} {
		if c := hotp(cle, uint64(v.t/totpPeriode), 8); c != v.attend {
			t.Errorf("t=%d : %s, attendu %s", v.t, c, v.attend)
		}
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(cle)
		if c := CodeTOTP(secret, time.Unix(v.t, 0)); c != v.attend[2:] {
			t.Errorf("t=%d, 6 chiffres : %s, attendu %s", v.t, c, v.attend[2:])
		}
	}
}

// registreAdmin : un registre avec un administrateur sans facteur, et une horloge à la main.
func registreAdmin(t *testing.T) (*Registre, *time.Time) {
	t.Helper()
	r, err := Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("chef", "une-phrase-bien-longue", true); err != nil {
		t.Fatal(err)
	}
	h := time.Unix(1_800_000_000, 0)
	r.HorlogePourTests(func() time.Time { return h })
	return r, &h
}

func TestTOTPActivationSeulementApresUnCodeValide(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if n, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret); len(n) != 20 {
		t.Fatalf("secret de %d octets", len(n))
	}
	if _, err := r.ActiverTOTP("chef", secret, "000000x"); !errors.Is(err, ErrCode) {
		t.Fatalf("code illisible : %v", err)
	}
	faux := "000000"
	if CodeTOTP(secret, *h) == faux {
		faux = "000001"
	}
	if _, err := r.ActiverTOTP("chef", secret, faux); !errors.Is(err, ErrCode) {
		t.Fatalf("code faux : %v", err)
	}
	if c, _ := r.Lire("chef"); c.AFacteur() || c.SecondFacteur != nil {
		t.Fatalf("activé sans code valide : %+v", c.SecondFacteur)
	}
	codes, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h))
	if err != nil || len(codes) != secoursNombre {
		t.Fatalf("activation : %v, %d codes", err, len(codes))
	}
	if c, _ := r.Lire("chef"); !c.AFacteur() || c.SecoursRestants() != secoursNombre {
		t.Fatalf("après activation : %+v", c.SecondFacteur)
	}
}

// Un décalage d'horloge d'un pas passe ; au-delà, non. Aucun code ne sert deux fois, ni un code plus
// ancien que le dernier accepté.
func TestTOTPFenetreEtRejeu(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if _, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	// le code de l'activation ne sert pas à entrer
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, *h)); !errors.Is(err, ErrCode) {
		t.Fatalf("code de l'activation rejoué : %v", err)
	}
	*h = h.Add(10 * totpPeriode * time.Second)
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, h.Add(-2*totpPeriode*time.Second))); !errors.Is(err, ErrCode) {
		t.Fatalf("deux pas en retard : %v", err)
	}
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, h.Add(2*totpPeriode*time.Second))); !errors.Is(err, ErrCode) {
		t.Fatalf("deux pas en avance : %v", err)
	}
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, h.Add(-totpPeriode*time.Second))); err != nil {
		t.Fatalf("un pas en retard : %v", err)
	}
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, h.Add(-totpPeriode*time.Second))); !errors.Is(err, ErrCode) {
		t.Fatalf("rejeu : %v", err)
	}
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, h.Add(totpPeriode*time.Second))); err != nil {
		t.Fatalf("un pas en avance : %v", err)
	}
	// le code courant est plus ancien que le dernier accepté : refusé
	if err := r.VerifierTOTP("chef", CodeTOTP(secret, *h)); !errors.Is(err, ErrCode) {
		t.Fatalf("code d'un pas déjà dépassé : %v", err)
	}
	// le dernier pas survit à un redémarrage
	r2, err := Ouvrir(r.chemin)
	if err != nil {
		t.Fatal(err)
	}
	r2.HorlogePourTests(func() time.Time { return *h })
	if err := r2.VerifierTOTP("chef", CodeTOTP(secret, h.Add(totpPeriode*time.Second))); !errors.Is(err, ErrCode) {
		t.Fatalf("rejeu après un redémarrage : %v", err)
	}
}

func TestCodesDeSecours(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	codes, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h))
	if err != nil {
		t.Fatal(err)
	}
	vus := map[string]bool{}
	for _, c := range codes {
		if vus[c] || len(strings.ReplaceAll(c, "-", "")) != secoursLongueur {
			t.Fatalf("code %q", c)
		}
		vus[c] = true
	}
	b, _ := os.ReadFile(r.chemin)
	if bytes.Contains(b, []byte(codes[0])) || bytes.Contains(b, []byte(strings.ReplaceAll(codes[0], "-", ""))) {
		t.Fatal("un code de secours en clair dans le fichier")
	}
	if _, err := r.UtiliserSecours("chef", "0000-0000-0000-0000"); !errors.Is(err, ErrCode) {
		t.Fatalf("code inconnu : %v", err)
	}
	// saisi en minuscules, sans tirets : le même code
	n, err := r.UtiliserSecours("chef", strings.ToLower(strings.ReplaceAll(codes[0], "-", "")))
	if err != nil || n != secoursNombre-1 {
		t.Fatalf("premier usage : %d %v", n, err)
	}
	if _, err := r.UtiliserSecours("chef", codes[0]); !errors.Is(err, ErrCode) {
		t.Fatalf("second usage : %v", err)
	}
	neufs, err := r.RegenererSecours("chef")
	if err != nil || len(neufs) != secoursNombre {
		t.Fatalf("régénération : %d %v", len(neufs), err)
	}
	if _, err := r.UtiliserSecours("chef", codes[1]); !errors.Is(err, ErrCode) {
		t.Fatalf("ancien code après régénération : %v", err)
	}
	if n, err := r.UtiliserSecours("chef", neufs[9]); err != nil || n != secoursNombre-1 {
		t.Fatalf("nouveau code : %d %v", n, err)
	}
	// un compte sans facteur n'a rien à régénérer
	if _, err := r.Creer("autre", "une-phrase-bien-longue", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RegenererSecours("autre"); !errors.Is(err, ErrSansFacteur) {
		t.Fatalf("sans facteur : %v", err)
	}
}

// Le secret est chiffré, lié à son compte, et la clé n'est pas dans le fichier des comptes.
func TestSecretTOTPChiffre(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if _, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.chemin)
	if bytes.Contains(b, []byte(secret)) {
		t.Fatal("secret en clair dans comptes.json")
	}
	dossier := filepath.Dir(r.chemin)
	info, err := os.Stat(filepath.Join(dossier, fichierCle))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("fichier de la clé : %v %v", info, err)
	}
	cle, _ := os.ReadFile(filepath.Join(dossier, fichierCle))
	if bytes.Contains(b, bytes.TrimSpace(cle)) {
		t.Fatal("la clé dans comptes.json")
	}
	// recopié sur un autre compte, le secret ne se déchiffre pas
	if _, err := r.Creer("autre", "une-phrase-bien-longue", true); err != nil {
		t.Fatal(err)
	}
	c, _ := r.Lire("chef")
	if _, err := r.Modifier("autre", func(x *Compte) error { x.SecondFacteur = c.SecondFacteur; return nil }); err != nil {
		t.Fatal(err)
	}
	*h = h.Add(10 * totpPeriode * time.Second)
	if err := r.VerifierTOTP("autre", CodeTOTP(secret, *h)); !errors.Is(err, ErrSecretIllisible) {
		t.Fatalf("secret d'un autre compte : %v", err)
	}
	// clé perdue : aucune entrée, et pas de clé neuve qui ferait croire le contraire
	if err := os.Remove(filepath.Join(dossier, fichierCle)); err != nil {
		t.Fatal(err)
	}
	r2, err := Ouvrir(r.chemin)
	if err != nil {
		t.Fatal(err)
	}
	r2.HorlogePourTests(func() time.Time { return *h })
	if err := r2.VerifierTOTP("chef", CodeTOTP(secret, *h)); !errors.Is(err, ErrSecretIllisible) {
		t.Fatalf("clé perdue : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dossier, fichierCle)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("une lecture crée une clé : %v", err)
	}
}

// Un facteur ne se copie pas par erreur d'une copie du compte à l'original.
func TestCopieDuSecondFacteur(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if _, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	c, _ := r.Lire("chef")
	c.SecondFacteur.TOTP.DernierPas = 0
	c.SecondFacteur.Secours[0].Utilise = 1
	d, _ := r.Lire("chef")
	if d.SecondFacteur.TOTP.DernierPas == 0 || d.SecondFacteur.Secours[0].Utilise != 0 {
		t.Fatal("la copie partage le second facteur avec le registre")
	}
}

func TestRetirerEtReinitialiser(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if _, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	if err := r.RetirerTOTP("chef"); !errors.Is(err, ErrDernierFacteur) {
		t.Fatalf("dernier facteur d'un administrateur : %v", err)
	}
	// hors de l'obligation, il se retire
	if _, err := r.Creer("second", "une-phrase-bien-longue", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Modifier("chef", func(c *Compte) error { c.Admin = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Lire("chef"); !c.AFacteur() {
		t.Fatal("quitter l'administration efface le facteur : c'est une réinitialisation sans journal")
	}
	avant, _ := r.Lire("chef")
	if err := r.RetirerTOTP("chef"); err != nil {
		t.Fatalf("hors obligation : %v", err)
	}
	if c, _ := r.Lire("chef"); c.AFacteur() || c.Version != avant.Version+1 {
		t.Fatalf("après retrait : %+v", c)
	}
	// réinitialisation : tout part, les sessions tombent, et le mot de passe ne change pas
	if _, err := r.ActiverTOTP("second", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	s, _ := r.Lire("second")
	if err := r.ReinitialiserSecondFacteur("second"); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Lire("second"); c.SecondFacteur != nil || c.Version != s.Version+1 || c.Mdp != s.Mdp {
		t.Fatalf("après réinitialisation : %+v", c)
	}
	// changer le mot de passe ne retire aucun facteur
	if _, err := r.ActiverTOTP("second", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	if err := r.DefinirMdp("second", "une-autre-phrase-longue"); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Lire("second"); !c.AFacteur() || c.SecoursRestants() != secoursNombre {
		t.Fatalf("après un changement de mot de passe : %+v", c.SecondFacteur)
	}
}

// La commande locale laisse une note, que le serveur reprend une seule fois au démarrage suivant.
func TestReinitialisationLocale(t *testing.T) {
	r, h := registreAdmin(t)
	secret := NouveauSecretTOTP()
	if _, err := r.ActiverTOTP("chef", secret, CodeTOTP(secret, *h)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("meneur", "une-phrase-bien-longue", false); err != nil {
		t.Fatal(err)
	}
	if err := r.ReinitialiserLocalement("meneur"); !errors.Is(err, ErrPasAdmin) {
		t.Fatalf("un compte qui n'est pas administrateur : %v", err)
	}
	if err := r.ReinitialiserLocalement("inconnu"); !errors.Is(err, ErrInconnu) {
		t.Fatalf("compte inconnu : %v", err)
	}
	avant, _ := r.Lire("chef")
	if err := r.ReinitialiserLocalement("CHEF"); err != nil {
		t.Fatal(err)
	}
	r2, err := Ouvrir(r.chemin)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := r2.Lire("chef"); c.AFacteur() || c.Version != avant.Version+1 {
		t.Fatalf("après la commande : %+v", c)
	}
	if l := r2.PrendreReinitLocales(); len(l) != 1 || l[0] != "chef" {
		t.Fatalf("note : %v", l)
	}
	if l := r2.PrendreReinitLocales(); len(l) != 0 {
		t.Fatalf("note reprise deux fois : %v", l)
	}
	r3, _ := Ouvrir(r.chemin)
	if l := r3.PrendreReinitLocales(); len(l) != 0 {
		t.Fatalf("note retrouvée après un redémarrage : %v", l)
	}
}

// Un fichier d'avant cette version se relit sans perte et n'est pas réécrit pour autant ; un fichier
// migré garde tout d'une relecture à l'autre.
func TestFichierAncienRelu(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	ancien := `[{"nom":"chef","mdp":"pbkdf2-sha256$1000$00$00","admin":true,"version":3,"cree":1,"graine":"abc"},
{"nom":"meneur","mdp":"x:y","roles":{"u":"mj"},"jetons":[{"id":"1","nom":"p","empreinte":"e","cree":1}],"version":1,"cree":2,"graine":"def"}]`
	if err := os.WriteFile(chemin, []byte(ancien), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(chemin); string(b) != ancien {
		t.Fatal("relire un fichier ancien le réécrit")
	}
	c, _ := r.Lire("chef")
	m, _ := r.Lire("meneur")
	if !c.Admin || c.AFacteur() || c.SecondFacteur != nil || c.Version != 3 || m.Roles["u"] != "mj" || len(m.Jetons) != 1 {
		t.Fatalf("relu : %+v %+v", c, m)
	}
	// après une écriture, les comptes sans facteur n'ont aucun nouveau champ
	if _, err := r.Modifier("meneur", func(x *Compte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var l []map[string]any
	b, _ := os.ReadFile(chemin)
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	for _, x := range l {
		if _, ok := x["second_facteur"]; ok {
			t.Fatalf("champ écrit pour un compte sans facteur : %v", x)
		}
	}
}
