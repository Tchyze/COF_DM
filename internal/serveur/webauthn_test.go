package serveur

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

const (
	origineEssai = "https://jdr.exemple.org"
	domaineEssai = "jdr.exemple.org"
	mdpCleEssai  = "une-phrase-bien-longue"
)

// cleLogicielle : un authentificateur FIDO2 en mémoire. Elle répond à ce qu'on lui demande, y compris de
// mauvaises origines : c'est au serveur de refuser.
type cleLogicielle struct {
	t            *testing.T
	priv         *ecdsa.PrivateKey
	id           []byte
	compteur     uint32
	origine      string
	domaine      string
	sansToucher  bool
	sansCompteur bool
}

func nouvelleCle(t *testing.T) *cleLogicielle {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &cleLogicielle{t: t, priv: priv, id: id, compteur: 10, origine: origineEssai, domaine: domaineEssai}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (k *cleLogicielle) donneesClient(genre string, options map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"type": genre, "challenge": options["challenge"], "origin": k.origine, "crossOrigin": false})
	return b
}

// Un encodage CBOR juste assez long pour ce qu'une clé envoie.
func cborTete(majeur byte, n int) []byte {
	switch {
	case n < 24:
		return []byte{majeur<<5 | byte(n)}
	case n < 256:
		return []byte{majeur<<5 | 24, byte(n)}
	default:
		return []byte{majeur<<5 | 25, byte(n >> 8), byte(n)}
	}
}

func cborTexte(s string) []byte  { return append(cborTete(3, len(s)), s...) }
func cborOctets(b []byte) []byte { return append(cborTete(2, len(b)), b...) }

func (k *cleLogicielle) cleCOSE() []byte {
	x, y := make([]byte, 32), make([]byte, 32)
	k.priv.X.FillBytes(x)
	k.priv.Y.FillBytes(y)
	var b bytes.Buffer
	b.Write([]byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21})
	b.Write(cborOctets(x))
	b.WriteByte(0x22)
	b.Write(cborOctets(y))
	return b.Bytes()
}

func (k *cleLogicielle) donneesAuth(drapeaux byte) []byte {
	h := sha256.Sum256([]byte(k.domaine))
	b := append([]byte{}, h[:]...)
	if !k.sansToucher {
		drapeaux |= 0x01
	}
	b = append(b, drapeaux)
	return binary.BigEndian.AppendUint32(b, k.compteur)
}

// creer : la réponse à navigator.credentials.create, attestation « none ».
func (k *cleLogicielle) creer(options map[string]any) map[string]any {
	auth := k.donneesAuth(0x40)
	auth = append(auth, make([]byte, 16)...) // AAGUID
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(k.id)))
	auth = append(auth, k.id...)
	auth = append(auth, k.cleCOSE()...)
	var att bytes.Buffer
	att.WriteByte(0xa3)
	att.Write(cborTexte("fmt"))
	att.Write(cborTexte("none"))
	att.Write(cborTexte("attStmt"))
	att.WriteByte(0xa0)
	att.Write(cborTexte("authData"))
	att.Write(cborOctets(auth))
	return map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"clientDataJSON": b64(k.donneesClient("webauthn.create", options)), "attestationObject": b64(att.Bytes()), "transports": []string{"usb"}}}
}

// prouver : la réponse à navigator.credentials.get.
func (k *cleLogicielle) prouver(options map[string]any) map[string]any {
	if !k.sansCompteur {
		k.compteur++
	}
	auth := k.donneesAuth(0)
	client := k.donneesClient("webauthn.get", options)
	hc := sha256.Sum256(client)
	h := sha256.Sum256(append(append([]byte{}, auth...), hc[:]...))
	r, s, err := ecdsa.Sign(rand.Reader, k.priv, h[:])
	if err != nil {
		k.t.Fatal(err)
	}
	sig, _ := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	return map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"clientDataJSON": b64(client), "authenticatorData": b64(auth), "signature": b64(sig)}}
}

// bancCles : un serveur qui connaît son adresse, un administrateur « chef » sans facteur, connecté en enrôlement.
func bancCles(t *testing.T) (*Application, *comptes.Registre, *navigateur) {
	t.Helper()
	a, reg, _ := serveurMulti(t)
	cfg, err := LireConfigCles(origineEssai, "")
	if err != nil {
		t.Fatal(err)
	}
	a.opt.acces.cles = cfg
	if _, err := reg.Creer("chef", mdpEssai("chef"), true); err != nil {
		t.Fatal(err)
	}
	n := anonymeDe(a, t, reg)
	if c, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")}); c != 200 || v["etape"] != "enrolement" {
		t.Fatalf("connexion : %d %v", c, v)
	}
	return a, reg, n
}

// enregistrer : le début et l'activation d'une clé. Renvoie le corps de l'activation.
func enregistrer(t *testing.T, n *navigateur, k *cleLogicielle, conf map[string]any) (int, map[string]any) {
	t.Helper()
	c, o := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé d'essai", "confirmation": conf})
	if c != 200 {
		t.Fatalf("début : %d %v", c, o)
	}
	return n.faire("POST", prefixeSecondFacteur+"cles/activer", map[string]any{"reponse": k.creer(o)})
}

// connecterParCle : le mot de passe, le défi, la réponse. Renvoie le navigateur et le code de la dernière étape.
func connecterParCle(t *testing.T, a *Application, reg *comptes.Registre, k *cleLogicielle) (*navigateur, int, map[string]any) {
	t.Helper()
	n := anonymeDe(a, t, reg)
	c, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	if c != 200 || v["etape"] != "second_facteur" {
		t.Fatalf("mot de passe : %d %v", c, v)
	}
	c, o := n.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	if c != 200 {
		t.Fatalf("défi : %d %v", c, o)
	}
	c, v = n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": k.prouver(o)})
	return n, c, v
}

func TestConfigCles(t *testing.T) {
	for _, x := range []struct {
		origine, rp string
		ok          bool
		rpAttendu   string
	}{
		{"https://jdr.exemple.org", "", true, "jdr.exemple.org"},
		{"https://JDR.exemple.org/", "", true, "jdr.exemple.org"},
		{"https://jdr.exemple.org:8443", "", true, "jdr.exemple.org"},
		{"https://jdr.exemple.org", "exemple.org", true, "exemple.org"},
		{"http://localhost:8177", "", true, "localhost"},
		{"http://jdr.exemple.org", "", false, ""},
		{"https://jdr.exemple.org/chemin", "", false, ""},
		{"https://jdr.exemple.org?x=1", "", false, ""},
		{"https://192.0.2.1", "", false, ""},
		{"jdr.exemple.org", "", false, ""},
		{"https://jdr.exemple.org", "autre.org", false, ""},
		{"https://jdr.exemple.org", "emple.org", false, ""},
		{"https://jdr.exemple.org", "org", false, ""},
		{"", "", false, ""},
	} {
		c, err := LireConfigCles(x.origine, x.rp)
		if (err == nil) != x.ok {
			t.Errorf("%q %q : %v", x.origine, x.rp, err)
		} else if x.ok && c.RPID != x.rpAttendu {
			t.Errorf("%q %q : RP ID %q", x.origine, x.rp, c.RPID)
		}
	}
}

// Du mot de passe à la session complète, par une clé ; la session d'enrôlement ne survit pas.
func TestEnrolementEtConnexionParCle(t *testing.T) {
	buf := journalDe(t)
	a, reg, n := bancCles(t)
	k := nouvelleCle(t)
	// le mot de passe est demandé, le nom aussi
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé"}); c != 403 {
		t.Fatalf("sans confirmation : %d", c)
	}
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé", "confirmation": map[string]any{"mdp": "pas-le-bon-mot-de-passe"}}); c != 401 {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": " ", "confirmation": map[string]any{"mdp": mdpEssai("chef")}}); c != 422 {
		t.Fatalf("sans nom : %d", c)
	}
	c, o := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé du trousseau", "confirmation": map[string]any{"mdp": mdpEssai("chef")}})
	if c != 200 {
		t.Fatalf("début : %d %v", c, o)
	}
	rp := o["rp"].(map[string]any)
	if rp["id"] != domaineEssai || o["attestation"] != "none" {
		t.Fatalf("options : %v", o)
	}
	if strings.Contains(toJSON(o["user"]), `"id":"chef"`) {
		t.Fatalf("identifiant de l'utilisateur : %v", o["user"])
	}
	avant := n.cookies[cookieMJ].Value
	c, v := n.faire("POST", prefixeSecondFacteur+"cles/activer", map[string]any{"reponse": k.creer(o)})
	if l, _ := v["secours"].([]any); c != 200 || len(l) != 10 {
		t.Fatalf("activation : %d %v", c, v)
	}
	if c, _ := n.faire("GET", "/api/admin/comptes", nil); c != 200 {
		t.Fatalf("après l'activation : %d", c)
	}
	vieux := anonymeDe(a, t, reg)
	vieux.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: avant}
	if c, _ := vieux.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("ancienne session d'enrôlement : %d", c)
	}
	_, compte := n.faire("GET", "/api/compte", nil)
	f := compte["second_facteur"].(map[string]any)
	if f["requis"] != false || f["totp"] != false || toJSON(f["cles"]) == "[]" || strings.Contains(toJSON(compte), "cle_publique") || strings.Contains(toJSON(compte), "identifiant") {
		t.Fatalf("compte : %v", compte)
	}
	// connexion : la clé, après le mot de passe, et une session de 12 heures
	m, code, r := connecterParCle(t, a, reg, k)
	if code != 200 || r["compte"] == nil {
		t.Fatalf("connexion par la clé : %d %v", code, r)
	}
	if d := time.Until(m.cookies[cookieMJ].Expires); d < dureeSessionAdmin-time.Minute {
		t.Fatalf("durée : %v", d)
	}
	if c, _ := m.faire("GET", "/api/admin/comptes", nil); c != 200 {
		t.Fatalf("administration : %d", c)
	}
	if x, _ := reg.Lire("chef"); x.SecondFacteur.Cles[0].Compteur != k.compteur || x.SecondFacteur.Cles[0].Utilise == 0 {
		t.Fatalf("compteur non noté : %+v", x.SecondFacteur.Cles[0])
	}
	l := lignesSecurite(buf)
	for _, attendu := range []string{`securite evenement=second_facteur_active facteur=cle nom="chef" ip=127.0.0.1`, `securite evenement=connexion_ok facteur=cle nom="chef" ip=127.0.0.1`} {
		if !contient(l, attendu) {
			t.Errorf("manque %s dans\n%s", attendu, strings.Join(l, "\n"))
		}
	}
	if strings.Contains(buf.String(), o["challenge"].(string)) {
		t.Error("un défi au journal")
	}
}

// Une clé ne s'enregistre pas sur le mauvais site, ni deux fois.
func TestEnregistrementRefuse(t *testing.T) {
	_, reg, n := bancCles(t)
	conf := map[string]any{"mdp": mdpEssai("chef")}
	k := nouvelleCle(t)
	k.origine = "https://piege.exemple.org"
	if c, v := enregistrer(t, n, k, conf); c != 422 {
		t.Fatalf("mauvaise origine : %d %v", c, v)
	}
	k = nouvelleCle(t)
	k.domaine = "exemple.org"
	if c, v := enregistrer(t, n, k, conf); c != 422 {
		t.Fatalf("mauvais domaine : %d %v", c, v)
	}
	// sans défi en attente, ou un défi déjà servi
	k = nouvelleCle(t)
	_, o := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé", "confirmation": conf})
	rep := k.creer(o)
	rep2 := map[string]any{"reponse": rep}
	k.origine = origineEssai
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/activer", rep2); c != 200 {
		t.Fatalf("activation : %d", c)
	}
	if x, _ := reg.Lire("chef"); x.NombreDeCles() != 1 {
		t.Fatalf("%d clés", x.NombreDeCles())
	}
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/activer", rep2); c != 409 {
		t.Fatalf("même défi rejoué : %d", c)
	}
}

// À la connexion : mauvaise origine, mauvais domaine, défi rejoué, défi d'une autre preuve, présence absente,
// compteur incohérent. Rien n'ouvre de session.
func TestConnexionParCleRefusee(t *testing.T) {
	a, reg, n := bancCles(t)
	k := nouvelleCle(t)
	if c, v := enregistrer(t, n, k, map[string]any{"mdp": mdpEssai("chef")}); c != 200 {
		t.Fatalf("enregistrement : %d %v", c, v)
	}
	refuser := func(nom string, k *cleLogicielle) {
		t.Helper()
		m, c, v := connecterParCle(t, a, reg, k)
		if c != 401 || m.cookies[cookieMJ] != nil {
			t.Fatalf("%s : %d %v", nom, c, v)
		}
		a.opt.acces.oublierEssais("2f-couple:chef|127.0.0.1")
	}
	mal := *k
	mal.origine = "https://piege.exemple.org"
	refuser("mauvaise origine", &mal)
	mal = *k
	mal.domaine = "exemple.org"
	refuser("mauvais domaine", &mal)
	mal = *k
	mal.sansToucher = true
	refuser("sans présence", &mal)
	autre := nouvelleCle(t)
	refuser("clé inconnue", autre)
	// même signature fausse : la bonne clé publique, une autre clé privée
	mal = *k
	mal.priv, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	refuser("mauvaise signature", &mal)

	// compteur : égal, puis en recul
	x, _ := reg.Lire("chef")
	base := x.SecondFacteur.Cles[0].Compteur
	mal = *k
	mal.sansCompteur, mal.compteur = true, base
	refuser("compteur égal", &mal)
	mal.compteur = base - 1
	refuser("compteur en recul", &mal)

	// défi rejoué : la même réponse, deux fois
	m := anonymeDe(a, t, reg)
	m.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	_, o := m.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	k.compteur = base
	rep := map[string]any{"facteur": "cle", "reponse": k.prouver(o)}
	if c, v := m.faire("POST", routeSecondFacteur, rep); c != 200 {
		t.Fatalf("première réponse : %d %v", c, v)
	}
	r := anonymeDe(a, t, reg)
	r.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	r.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	if c, _ := r.faire("POST", routeSecondFacteur, rep); c != 401 {
		t.Fatalf("réponse rejouée sur une nouvelle preuve : %d", c)
	}
	// sur la même preuve : un défi qui a reçu une réponse, même fausse, ne sert plus
	u := anonymeDe(a, t, reg)
	u.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	_, ou := u.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	faux := *k
	faux.origine = "https://piege.exemple.org"
	if c, _ := u.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": faux.prouver(ou)}); c != 401 {
		t.Fatalf("réponse fausse : %d", c)
	}
	if c, _ := u.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": k.prouver(ou)}); c != 401 || u.cookies[cookieMJ] != nil {
		t.Fatalf("bonne réponse à un défi déjà servi : %d", c)
	}
	acces0 := a.opt.acces
	acces0.oublierEssais("2f-couple:chef|127.0.0.1")
	// le défi d'une preuve ne vaut pas pour une autre
	p1, p2 := anonymeDe(a, t, reg), anonymeDe(a, t, reg)
	p1.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	p2.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	_, o1 := p1.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	if c, _ := p2.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": k.prouver(o1)}); c != 401 {
		t.Fatalf("défi d'une autre preuve : %d", c)
	}
	// sans preuve, pas de défi, et aucun compteur touché
	sans := anonymeDe(a, t, reg)
	if c, v := sans.faire("POST", routeSecondFacteur+"/defi", map[string]any{}); c != 401 || v["reprendre"] != true {
		t.Fatalf("défi sans preuve : %d %v", c, v)
	}
	// un défi expiré
	exp := anonymeDe(a, t, reg)
	exp.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	_, oe := exp.faire("POST", routeSecondFacteur+"/defi", map[string]any{})
	acces := a.opt.acces
	acces.mu.Lock()
	for id, d := range acces.defis {
		d.exp = time.Now().Add(-time.Second)
		acces.defis[id] = d
	}
	acces.mu.Unlock()
	if c, _ := exp.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": k.prouver(oe)}); c != 401 {
		t.Fatalf("défi expiré : %d", c)
	}
}

// Une clé sans compteur (toujours zéro) reste acceptée, et le compteur incohérent entre au journal.
func TestCompteurDeLaCleAuJournal(t *testing.T) {
	buf := journalDe(t)
	a, reg, n := bancCles(t)
	z := nouvelleCle(t)
	z.compteur, z.sansCompteur = 0, true
	if c, v := enregistrer(t, n, z, map[string]any{"mdp": mdpEssai("chef")}); c != 200 {
		t.Fatalf("enregistrement : %d %v", c, v)
	}
	for i := 0; i < 2; i++ {
		if _, c, v := connecterParCle(t, a, reg, z); c != 200 {
			t.Fatalf("clé sans compteur, fois %d : %d %v", i, c, v)
		}
	}
	// elle se met à compter, puis recule
	z.compteur = 8
	if _, c, v := connecterParCle(t, a, reg, z); c != 200 {
		t.Fatalf("compteur qui démarre : %d %v", c, v)
	}
	z.compteur = 3
	if _, c, _ := connecterParCle(t, a, reg, z); c != 401 {
		t.Fatalf("recul : %d", c)
	}
	if l := lignesSecurite(buf); !contient(l, `securite evenement=cle_compteur_incoherent nom="chef" ip=127.0.0.1`) {
		t.Errorf("pas au journal :\n%s", strings.Join(l, "\n"))
	}
}

// Plusieurs clés : une deuxième demande un second facteur, on les nomme et on les retire, jamais la dernière.
func TestPlusieursCles(t *testing.T) {
	a, reg, n := bancCles(t)
	k1, k2 := nouvelleCle(t), nouvelleCle(t)
	if c, v := enregistrer(t, n, k1, map[string]any{"mdp": mdpEssai("chef")}); c != 200 {
		t.Fatalf("première : %d %v", c, v)
	}
	// une session volée ne suffit pas : le mot de passe seul non plus, désormais
	if c, _ := n.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Deuxième", "confirmation": map[string]any{"mdp": mdpEssai("chef")}}); c != 422 {
		t.Fatalf("deuxième clé avec le mot de passe seul : %d", c)
	}
	// confirmer par la clé
	conf := func(k *cleLogicielle) map[string]any {
		t.Helper()
		c, o := n.faire("POST", prefixeSecondFacteur+"defi", map[string]any{})
		if c != 200 {
			t.Fatalf("défi de confirmation : %d %v", c, o)
		}
		return map[string]any{"mdp": mdpEssai("chef"), "facteur": "cle", "reponse": k.prouver(o)}
	}
	c, v := enregistrer(t, n, k2, conf(k1))
	if c != 200 || v["secours"] != nil {
		t.Fatalf("deuxième : %d %v", c, v)
	}
	x, _ := reg.Lire("chef")
	if x.NombreDeCles() != 2 {
		t.Fatalf("%d clés", x.NombreDeCles())
	}
	id1, id2 := x.SecondFacteur.Cles[0].ID, x.SecondFacteur.Cles[1].ID
	// la même clé deux fois : le navigateur l'empêche, le serveur aussi
	if c, _ := enregistrer(t, n, k2, conf(k1)); c != 409 {
		t.Fatalf("même clé : %d", c)
	}
	// les deux clés ouvrent le compte
	if _, c, v := connecterParCle(t, a, reg, k2); c != 200 {
		t.Fatalf("deuxième clé : %d %v", c, v)
	}
	if c, _ := n.faire("PATCH", prefixeSecondFacteur+"cles/"+id2, map[string]any{"nom": "Coffre"}); c != 200 {
		t.Fatalf("renommer : %d", c)
	}
	if c, _ := n.faire("PATCH", prefixeSecondFacteur+"cles/inconnue", map[string]any{"nom": "Coffre"}); c != 404 {
		t.Fatalf("renommer une clé inconnue : %d", c)
	}
	// retirer : confirmation, puis les autres sessions tombent et celle-ci continue
	autre, cc, _ := connecterParCle(t, a, reg, k1)
	if cc != 200 {
		t.Fatalf("autre session : %d", cc)
	}
	if c, _ := n.faire("DELETE", prefixeSecondFacteur+"cles/"+id1, map[string]any{}); c != 403 {
		t.Fatalf("retrait sans confirmation : %d", c)
	}
	if c, v := n.faire("DELETE", prefixeSecondFacteur+"cles/"+id1, map[string]any{"confirmation": conf(k1)}); c != 200 {
		t.Fatalf("retrait : %d %v", c, v)
	}
	if c, _ := autre.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("autre session après un retrait : %d", c)
	}
	if c, _ := n.faire("GET", "/api/compte", nil); c != 200 {
		t.Fatalf("cette session après un retrait : %d", c)
	}
	// la clé retirée ne sert plus : le serveur ne la connaît plus
	if _, c, _ := connecterParCle(t, a, reg, k1); c != 401 {
		t.Fatalf("clé retirée : %d", c)
	}
	a.opt.acces.oublierEssais("2f-couple:chef|127.0.0.1")
	// la dernière reste, sans rien consommer
	if c, _ := n.faire("DELETE", prefixeSecondFacteur+"cles/"+id2, map[string]any{"confirmation": conf(k2)}); c != 409 {
		t.Fatalf("dernière clé : %d", c)
	}
	// avec un TOTP en plus, la clé peut partir, et le TOTP aussi tant qu'une clé reste
	c, d := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": conf(k2)})
	if c != 200 {
		t.Fatalf("TOTP : %d %v", c, d)
	}
	code := comptes.CodeTOTP(d["secret"].(string), horlogeDe(reg).lire())
	if c, v := n.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": code}); c != 200 {
		t.Fatalf("activation du TOTP : %d %v", c, v)
	}
	if c, v := n.faire("DELETE", prefixeSecondFacteur+"totp", map[string]any{"confirmation": conf(k2)}); c != 200 {
		t.Fatalf("retrait du TOTP, la clé restant : %d %v", c, v)
	}
	if c, _ := n.faire("DELETE", prefixeSecondFacteur+"totp", map[string]any{"confirmation": conf(k2)}); c != 404 {
		t.Fatalf("plus de TOTP : %d", c)
	}
}

// Une session volée ne s'enregistre pas de clé ; une réponse de clé n'en confirme que pour la bonne session.
func TestCleEtSessionVolee(t *testing.T) {
	a, reg, chef := preparerAdmin(t)
	cfg, _ := LireConfigCles(origineEssai, "")
	a.opt.acces.cles = cfg
	k := nouvelleCle(t)
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé volée"}); c != 403 {
		t.Fatalf("sans confirmation : %d", c)
	}
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé volée", "confirmation": map[string]any{"mdp": "pas-le-bon-mot-de-passe", "facteur": "totp", "code": codeSuivant(reg)}}); c != 401 {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"cles/activer", map[string]any{"reponse": k.creer(map[string]any{"challenge": "x"})}); c != 409 {
		t.Fatalf("activation sans début : %d", c)
	}
	// un compte MJ n'a pas ces routes
	mj := connecterMJ(t, a, reg)
	for _, x := range [][2]string{{"POST", "defi"}, {"POST", "cles/debut"}, {"POST", "cles/activer"}, {"PATCH", "cles/x"}, {"DELETE", "cles/x"}} {
		if c, _ := mj.faire(x[0], prefixeSecondFacteur+x[1], map[string]any{}); c != 403 {
			t.Fatalf("%s %s par un MJ : %d", x[0], x[1], c)
		}
	}
	// le TOTP continue : une clé n'est pas obligatoire, et la limite du TOTP ne touche pas les clés
	c, o := chef.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé", "confirmation": confirmationDe(reg, "chef")})
	if c != 200 {
		t.Fatalf("début : %d %v", c, o)
	}
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"cles/activer", map[string]any{"reponse": k.creer(o)}); c != 200 {
		t.Fatalf("activation : %d", c)
	}
	for i := 0; i < essaisMaxTOTPCompte; i++ {
		a.opt.acces.reserverEssai("2f-compte:chef", essaisMaxTOTPCompte)
	}
	if _, c, v := connecterParCle(t, a, reg, k); c != 200 {
		t.Fatalf("clé malgré la limite du TOTP : %d %v", c, v)
	}
}

// Sans -origine : pas de clés, et le TOTP marche. Une clé déjà enregistrée n'est plus proposée.
func TestSansOrigine(t *testing.T) {
	a, reg, chef := preparerAdmin(t)
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"cles/debut", map[string]any{"nom": "Clé", "confirmation": confirmationDe(reg, "chef")}); c != 409 {
		t.Fatalf("sans configuration : %d", c)
	}
	if c, _ := chef.faire("POST", prefixeSecondFacteur+"defi", map[string]any{}); c != 409 {
		t.Fatalf("défi sans configuration : %d", c)
	}
	if c, v := chef.faire("GET", "/api/compte", nil); c != 200 || v["second_facteur"].(map[string]any)["cles_disponibles"] != false {
		t.Fatalf("compte : %d %v", c, v)
	}
	n := anonymeDe(a, t, reg)
	_, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	if toJSON(v["facteurs"]) != `["totp","secours"]` {
		t.Fatalf("facteurs : %v", v["facteurs"])
	}
	if c, _ := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "cle", "reponse": map[string]any{}}); c != 422 {
		t.Fatalf("clé sans configuration : %d", c)
	}
}

// WebAuthn n'est pas bloqué par les en-têtes : la permission reste au site lui-même.
func TestPermissionsCles(t *testing.T) {
	_, _, chef := preparerAdmin(t)
	p := chef.brut("GET", "/api/compte", nil).Header().Get("Permissions-Policy")
	for _, attendu := range []string{"publickey-credentials-get=(self)", "publickey-credentials-create=(self)", "camera=()"} {
		if !strings.Contains(p, attendu) {
			t.Errorf("Permissions-Policy %q : manque %s", p, attendu)
		}
	}
}
