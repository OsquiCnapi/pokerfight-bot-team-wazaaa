package estrategia

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// turnoAlAzar inventa una situación cualquiera, coherente con lo que la mesa
// podría mandar: stacks, apuestas, cartas y acciones válidas al azar.
func turnoAlAzar(azar *rand.Rand) (inicio botsdk.EstadoPublico, t botsdk.Turno) {
	mazo := azar.Perm(52)
	carta := func(i int) botsdk.Carta {
		return botsdk.Carta{Rango: botsdk.Rango(mazo[i]/4 + 2), Palo: botsdk.Palo(mazo[i] % 4)}
	}

	jugadores := 2 + azar.IntN(5)
	const cg = 100
	inicio = botsdk.EstadoPublico{
		IDMano: "prueba", Etapa: botsdk.PreFlop, CiegaChica: cg / 2, CiegaGrande: cg,
		ApuestaActual: cg, SubidaMinima: cg, Pozo: cg + cg/2, PosicionBoton: azar.IntN(jugadores),
	}
	for i := 0; i < jugadores; i++ {
		inicio.Jugadores = append(inicio.Jugadores, botsdk.JugadorPublico{
			ID: string(rune('a' + i)), Saldo: int64(1 + azar.IntN(30000)),
			Activo: true, EnTorneo: true, EnMano: true, PosicionSilla: i,
		})
	}

	estado := inicio
	estado.Jugadores = append([]botsdk.JugadorPublico(nil), inicio.Jugadores...)
	etapas := []botsdk.Etapa{botsdk.PreFlop, botsdk.Flop, botsdk.Turn, botsdk.River}
	calle := azar.IntN(4)
	estado.Etapa = etapas[calle]
	if calle > 0 {
		for i := 0; i < 2+calle; i++ {
			estado.Comunitarias = append(estado.Comunitarias, carta(2+i))
		}
	}

	// Un historial corto con acciones cualesquiera de los rivales.
	tipos := []botsdk.TipoAccion{botsdk.Fold, botsdk.Check, botsdk.Call, botsdk.Bet, botsdk.Raise, botsdk.AllIn}
	for i := 1; i < jugadores; i++ {
		if azar.IntN(3) == 0 {
			continue
		}
		tipo := tipos[azar.IntN(len(tipos))]
		estado.HistorialAcciones = append(estado.HistorialAcciones, botsdk.AccionRegistrada{
			IDJugador: estado.Jugadores[i].ID, Etapa: etapas[azar.IntN(calle+1)],
			Accion: botsdk.Accion{Tipo: tipo, Monto: int64(azar.IntN(5000))},
		})
		if tipo == botsdk.Fold && i > 1 {
			estado.Jugadores[i].Activo = false
		}
		if tipo == botsdk.AllIn {
			estado.Jugadores[i].AllIn = true
		}
	}

	yo := estado.Jugadores[0]
	estado.Pozo = int64(150 + azar.IntN(20000))
	estado.ApuestaActual = int64(azar.IntN(3)) * int64(azar.IntN(8000))
	estado.SubidaMinima = cg + int64(azar.IntN(2000))
	yo.ApuestaRonda = 0
	if estado.ApuestaActual > 0 && azar.IntN(2) == 0 {
		yo.ApuestaRonda = int64(azar.IntN(int(estado.ApuestaActual)))
	}
	estado.Jugadores[0] = yo

	// Las mismas reglas con que la mesa arma las acciones válidas.
	validas := []botsdk.TipoAccion{botsdk.Fold}
	puedeSubir := azar.IntN(5) > 0
	pendiente := estado.ApuestaActual - yo.ApuestaRonda
	if pendiente <= 0 {
		pendiente = 0
		validas = append(validas, botsdk.Check)
		if puedeSubir {
			validas = append(validas, botsdk.Bet)
		}
	} else {
		validas = append(validas, botsdk.Call)
		if yo.Saldo > pendiente && puedeSubir {
			validas = append(validas, botsdk.Raise)
		}
	}
	if puedeSubir || yo.Saldo <= pendiente {
		validas = append(validas, botsdk.AllIn)
	}

	t = botsdk.Turno{
		IDJugador: yo.ID, Silla: 0, Yo: yo,
		Mano:    botsdk.Mano{carta(0), carta(1)},
		Estado:  estado,
		Validas: validas,
		Timeout: 5 * time.Second,
	}
	return inicio, t
}

// Pase lo que pase, el bot tiene que devolver una acción que la mesa acepte:
// una acción inválida cuesta lo mismo que no responder.
func TestSiempreDevuelveUnaAccionValida(t *testing.T) {
	azar := rand.New(rand.NewPCG(11, 22))
	cerebro := Nuevo(1)

	for vuelta := 0; vuelta < 3000; vuelta++ {
		inicio, turno := turnoAlAzar(azar)
		// A veces sin foto inicial, como tras una reconexión a mitad de mano.
		if azar.IntN(4) > 0 {
			cerebro.ManoInicio(inicio, turno.Mano)
		}

		accion := cerebro.Decidir(context.Background(), turno)

		if !turno.Puede(accion.Tipo) {
			t.Fatalf("vuelta %d: devolvió %q, válidas %v", vuelta, accion.Tipo, turno.Validas)
		}
		if accion.Tipo == botsdk.Bet || accion.Tipo == botsdk.Raise {
			if accion.Monto < turno.SubidaMinima() || accion.Monto > turno.Techo() {
				t.Fatalf("vuelta %d: %s de %d fuera de [%d, %d]", vuelta, accion.Tipo, accion.Monto, turno.SubidaMinima(), turno.Techo())
			}
		}
		if accion.Tipo == botsdk.Fold && turno.Puede(botsdk.Check) {
			t.Fatalf("vuelta %d: se retiró pudiendo pasar gratis", vuelta)
		}

		cerebro.ManoFin(botsdk.ResultadoMano{IDMano: turno.Estado.IDMano, Comunitarias: turno.Estado.Comunitarias})
	}
}

// Sin cartas (una reconexión en medio de una mano) juega lo seguro.
func TestSinCartasJuegaLoSeguro(t *testing.T) {
	azar := rand.New(rand.NewPCG(5, 6))
	_, turno := turnoAlAzar(azar)
	turno.Mano = botsdk.Mano{}

	accion := Nuevo(1).Decidir(context.Background(), turno)
	esperada := botsdk.Fold
	if turno.Puede(botsdk.Check) {
		esperada = botsdk.Check
	}
	if accion.Tipo != esperada {
		t.Errorf("sin cartas devolvió %q, se esperaba %q", accion.Tipo, esperada)
	}
}
