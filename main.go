// Bot del equipo Wazaaa para PokerFight

// Versión base
package main

import (
	// aqui se encuentran toda la parte de redes y protocolo, es como
	// la libreria que listas pilas y colas que daban en algoritmia para
	// que solo llames funciones y no hacer todo de 0
	"context"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// Estructura vacía del bot, se usará para guardar memoria entre jugadas
type botWazaaa struct{

}

// Identificador del bot
func (botWazaaa) Nombre() string { return "team-wazaaa" }

func (botWazaaa) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	// si el bot puede seguir gratis sigue
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	// si le cobran paga lo que corresponde
	if t.Puede(botsdk.Call) {
		return t.Pagar()
	}
	// si ya no puede pagar porque esta sin fichas, F
	return t.Fold()
}

func main() { botsdk.CorrerDesdeFlags(botWazaaa{}) }
