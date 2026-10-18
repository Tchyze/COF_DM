package main

import (
	"fmt"
	"net"
	"testing"
)

// Un autre programme tient 127.0.0.1 sur le port voulu (comme le relais de WSL sous Windows) :
// l'application ouverte au réseau prend le port suivant, pour que sa fenêtre ne l'ouvre pas.
func TestEcouterFenetreLibre(t *testing.T) {
	autre, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer autre.Close()
	pris := autre.Addr().(*net.TCPAddr).Port
	ln, port, err := ecouter("0.0.0.0", pris, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if port == pris {
		t.Fatalf("port %d pris sur 127.0.0.1, gardé quand même", pris)
	}
	// la fenêtre joint bien cette application
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}
