package serveur

import (
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/fiches"
)

// Un PJ n'a qu'un nombre borné de propositions en attente ; celles des autres PJ n'y comptent pas, et
// une proposition retirée libère sa place.
func TestPotPropositionsPlafonnees(t *testing.T) {
	p := &fiches.Pot{Pieces: fiches.Pieces{Po: 5}}
	un := func(pj int64) fiches.Proposition { return fiches.Proposition{Pj: pj, Pieces: fiches.Pieces{Po: 1}} }
	for i := 0; i < fiches.PropositionsMaxParPJ; i++ {
		if _, err := p.Proposer(un(1)); err != nil {
			t.Fatalf("proposition %d : %v", i, err)
		}
	}
	if _, err := p.Proposer(un(1)); err == nil {
		t.Fatal("une de trop : refus attendu")
	}
	if _, err := p.Proposer(un(2)); err != nil {
		t.Fatalf("un autre PJ : %v", err)
	}
	p.Oublier(p.Propositions[0].ID)
	if _, err := p.Proposer(un(1)); err != nil {
		t.Fatalf("place libérée : %v", err)
	}
}

// Une rafale passe ; au-delà, 429, et le débit revient avec le temps. Chacun a le sien.
func TestDebitDesJets(t *testing.T) {
	var d debitJets
	h := time.Unix(1000, 0)
	for i := 0; i < jetsRafale; i++ {
		if !d.permis("a", h) {
			t.Fatalf("jet %d de la rafale refusé", i)
		}
	}
	if d.permis("a", h) {
		t.Fatal("au-delà de la rafale : refus attendu")
	}
	if !d.permis("b", h) {
		t.Fatal("un autre lanceur n'y est pour rien")
	}
	if !d.permis("a", h.Add(time.Second)) {
		t.Fatal("le débit revient")
	}
	for i := 0; i < seauxMax+10; i++ {
		d.permis(string(rune(i+0x4000)), h)
	}
	d.permis("z", h.Add(time.Hour))
	if len(d.seaux) > seauxMax {
		t.Fatalf("mémoire bornée : %d", len(d.seaux))
	}
}

func TestDesLimitesParCompte(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	mj := connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	refus := 0
	for i := 0; i < jetsRafale+5; i++ {
		if c, _ := mj.faire("POST", "/api/des", map[string]any{"formule": "1d6"}, enTeteUnivers, id); c == 429 {
			refus++
		}
	}
	if refus == 0 {
		t.Fatal("aucun refus au-delà de la rafale")
	}
}
