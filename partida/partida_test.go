package partida

import (
	"testing"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// fotoInicial arma el estado de un mano_inicio: tres jugadores con 10 000
// fichas, botón en la silla 0, ciegas 50/100 ya puestas por las sillas 1 y 2.
func fotoInicial() botsdk.EstadoPublico {
	jugador := func(id string, silla int, ciega int64) botsdk.JugadorPublico {
		return botsdk.JugadorPublico{
			ID: id, Saldo: 10000 - ciega, ApuestaRonda: ciega,
			Activo: true, EnTorneo: true, EnMano: true, PosicionSilla: silla,
		}
	}
	return botsdk.EstadoPublico{
		IDMano:        "m-1",
		Etapa:         botsdk.PreFlop,
		Pozo:          150,
		ApuestaActual: 100,
		SubidaMinima:  100,
		CiegaChica:    50,
		CiegaGrande:   100,
		PosicionBoton: 0,
		Jugadores: []botsdk.JugadorPublico{
			jugador("boton", 0, 0), jugador("chica", 1, 50), jugador("grande", 2, 100),
		},
	}
}

func accion(id string, etapa botsdk.Etapa, tipo botsdk.TipoAccion, monto int64) botsdk.AccionRegistrada {
	return botsdk.AccionRegistrada{IDJugador: id, Etapa: etapa, Accion: botsdk.Accion{Tipo: tipo, Monto: monto}}
}

func TestReconstruirSigueLasFichas(t *testing.T) {
	inicio := fotoInicial()
	actual := fotoInicial()
	actual.Etapa = botsdk.Flop
	actual.HistorialAcciones = []botsdk.AccionRegistrada{
		accion("boton", botsdk.PreFlop, botsdk.Raise, 300),
		accion("chica", botsdk.PreFlop, botsdk.Fold, 0),
		accion("grande", botsdk.PreFlop, botsdk.Call, 0),
		accion("grande", botsdk.Flop, botsdk.Check, 0),
		accion("boton", botsdk.Flop, botsdk.Bet, 325),
	}
	// Lo que la mesa informaría en ese momento.
	actual.Pozo = 975
	actual.ApuestaActual = 325
	actual.Jugadores[0].Saldo, actual.Jugadores[0].ApuestaRonda = 9375, 325
	actual.Jugadores[1].Activo = false
	actual.Jugadores[2].Saldo, actual.Jugadores[2].ApuestaRonda = 9700, 0

	m := Reconstruir(&inicio, &actual)
	if !m.Completa {
		t.Fatal("la mano debería reconstruirse completa")
	}
	if len(m.Jugadas) != 5 {
		t.Fatalf("se esperaban 5 jugadas, hay %d", len(m.Jugadas))
	}

	abre := m.Jugadas[0]
	if abre.Tipo != Raise || abre.Pone != 300 || abre.Pozo != 150 || abre.PorIgualar != 100 || abre.Subidas != 0 {
		t.Errorf("subida preflop mal reconstruida: %+v", abre)
	}

	paga := m.Jugadas[2]
	if paga.Tipo != Call || paga.Pone != 200 || paga.PorIgualar != 200 || paga.Subidas != 1 {
		t.Errorf("pago de la ciega grande mal reconstruido: %+v", paga)
	}
	// Pagar 200 con 450 en el pozo: 200 sobre los 250 que había antes de la subida... 200/250.
	if f := paga.Fraccion(); f < 0.79 || f > 0.81 {
		t.Errorf("fracción enfrentada %.2f, se esperaba 0.80", f)
	}

	apuesta := m.Jugadas[4]
	if apuesta.Tipo != Bet || apuesta.Calle != Flop || apuesta.Pozo != 650 || apuesta.Pone != 325 {
		t.Errorf("apuesta del flop mal reconstruida: %+v", apuesta)
	}
	if f := apuesta.FraccionPuesta(); f < 0.49 || f > 0.51 {
		t.Errorf("tamaño de la apuesta %.2f, se esperaba 0.50", f)
	}

	if m.Pozo != 975 || m.Apuesta != 325 || m.Calle != Flop {
		t.Errorf("estado final: pozo %d, apuesta %d, calle %d", m.Pozo, m.Apuesta, m.Calle)
	}
	if got := m.Rivales("boton"); len(got) != 1 || got[0] != "grande" {
		t.Errorf("rivales del botón: %v", got)
	}
	// Tras la apuesta del botón, a la ciega grande le toca responder.
	if m.PendientesTras("boton") != 1 {
		t.Errorf("la ciega grande debería estar pendiente")
	}
	if !m.HablaUltimo("boton") {
		t.Errorf("el botón habla último después del flop")
	}
	if !m.Aposto("boton", Flop) || m.Aposto("grande", Flop) {
		t.Errorf("solo el botón apostó en el flop")
	}
}

func TestReconstruirAllIn(t *testing.T) {
	inicio := fotoInicial()
	inicio.Jugadores[0].Saldo = 800 // El botón tiene pocas fichas.
	actual := inicio
	actual.HistorialAcciones = []botsdk.AccionRegistrada{
		accion("boton", botsdk.PreFlop, botsdk.AllIn, 0),
		accion("chica", botsdk.PreFlop, botsdk.AllIn, 0),
	}
	actual.Jugadores = append([]botsdk.JugadorPublico(nil), inicio.Jugadores...)
	actual.Jugadores[0].Saldo, actual.Jugadores[0].ApuestaRonda, actual.Jugadores[0].AllIn = 0, 800, true
	actual.Jugadores[1].Saldo, actual.Jugadores[1].ApuestaRonda, actual.Jugadores[1].AllIn = 0, 10000, true
	actual.Pozo, actual.ApuestaActual = 10900, 10000

	m := Reconstruir(&inicio, &actual)
	primero, segundo := m.Jugadas[0], m.Jugadas[1]
	if primero.Tipo != Raise || !primero.AllIn || primero.Total != 800 {
		t.Errorf("all-in del botón: %+v", primero)
	}
	if segundo.Tipo != Raise || !segundo.AllIn || segundo.Total != 10000 || segundo.Subidas != 1 {
		t.Errorf("all-in de la ciega chica: %+v", segundo)
	}
	if m.PendientesTras("grande") != 0 {
		t.Errorf("los jugadores all-in no quedan pendientes")
	}
}

func TestSinFotoInicial(t *testing.T) {
	actual := fotoInicial()
	actual.HistorialAcciones = []botsdk.AccionRegistrada{accion("boton", botsdk.PreFlop, botsdk.Raise, 300)}

	m := Reconstruir(nil, &actual)
	if m.Completa || len(m.Jugadas) != 0 {
		t.Errorf("sin foto inicial la mano no se puede reconstruir: completa=%v jugadas=%d", m.Completa, len(m.Jugadas))
	}
	if len(m.Asientos) != 3 {
		t.Errorf("igual tiene que conocer a los jugadores: %d asientos", len(m.Asientos))
	}

	otra := fotoInicial()
	otra.IDMano = "m-0"
	if Reconstruir(&otra, &actual).Completa {
		t.Errorf("una foto inicial de otra mano no sirve")
	}
}

func TestOrdenDePalabra(t *testing.T) {
	inicio := fotoInicial()
	m := Reconstruir(&inicio, &inicio)
	orden := []string{m.Asientos[0].ID, m.Asientos[1].ID, m.Asientos[2].ID}
	if orden[0] != "chica" || orden[1] != "grande" || orden[2] != "boton" {
		t.Errorf("orden postflop %v, se esperaba chica, grande, boton", orden)
	}
	// Antes de que nadie hable, al botón le quedan dos rivales por hablar.
	if m.PendientesTras("boton") != 2 {
		t.Errorf("pendientes tras el botón: %d", m.PendientesTras("boton"))
	}
}
