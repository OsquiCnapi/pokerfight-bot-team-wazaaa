// Bot del equipo Wazaaa para PokerFight

// Versión base
package main

import (
	"context"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

type botWazaaa struct{}

func (botWazaaa) Nombre() string { return "team-wazaaa" }

func (botWazaaa) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	if t.Puede(botsdk.Call) {
		return t.Pagar()
	}
	return t.Fold()
}

func main() { botsdk.CorrerDesdeFlags(botWazaaa{}) }
