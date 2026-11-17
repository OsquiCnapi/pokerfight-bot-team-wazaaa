// Package partida reconstruye el desarrollo de una mano a partir de lo que la
// mesa informa.
//
// El historial de acciones del protocolo no trae montos en los call ni en los
// all-in, ni dice cuánto había en el pozo cuando se jugó cada acción. Para leer
// a un rival hace falta justamente eso ("apostó medio pozo", "pagó una subida
// de 8 ciegas"), así que este paquete vuelve a jugar la mano desde la foto
// inicial (la del mano_inicio, con las ciegas ya puestas) aplicando cada
// acción, y anota el contexto de cada una.
package partida

import (
	"sort"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
)

// Calle es la ronda de apuestas: 0 preflop, 1 flop, 2 turn, 3 river.
type Calle int

const (
	Preflop Calle = iota
	Flop
	Turn
	River
)

// CalleDe traduce la etapa del protocolo.
func CalleDe(e botsdk.Etapa) Calle {
	switch e {
	case botsdk.Flop:
		return Flop
	case botsdk.Turn:
		return Turn
	case botsdk.River, botsdk.Showdown:
		return River
	}
	return Preflop
}

// Tipo es una acción ya interpretada. El all-in del protocolo se traduce a lo
// que significa en la mesa: una apuesta, una subida o un pago.
type Tipo int

const (
	Fold Tipo = iota
	Check
	Call
	Bet
	Raise
)

// Agresiva indica si la acción mete presión (apuesta o sube).
func (t Tipo) Agresiva() bool { return t == Bet || t == Raise }

// Jugada es una acción con todo su contexto.
type Jugada struct {
	Jugador string
	Calle   Calle
	Tipo    Tipo
	AllIn   bool // El jugador se quedó sin fichas con esta acción.

	Total int64 // Apuesta del jugador en la ronda después de actuar.
	Pone  int64 // Fichas que agregó con esta acción.

	// Todo lo que sigue es la situación ANTES de la acción.
	Pozo       int64 // Pozo completo, incluida la ronda en curso.
	PorIgualar int64 // Lo que le faltaba para igualar.
	Apuesta    int64 // Apuesta más alta de la ronda.
	Saldo      int64 // Fichas que le quedaban al jugador.
	Subidas    int   // Apuestas y subidas previas en la calle (las ciegas no cuentan).
	EnMano     int   // Jugadores que seguían en la mano, contándolo.
}

// Fraccion es el tamaño de lo que el jugador enfrentaba, como fracción del
// pozo que había antes de esa apuesta. Para quien paga una apuesta de medio
// pozo, 0.5.
func (j Jugada) Fraccion() float64 {
	base := j.Pozo - j.PorIgualar
	if base <= 0 || j.PorIgualar <= 0 {
		return 0
	}
	return float64(j.PorIgualar) / float64(base)
}

// FraccionPuesta es el tamaño de una apuesta o subida propia: lo que agrega
// por encima de igualar, sobre el pozo que quedaría tras igualar.
func (j Jugada) FraccionPuesta() float64 {
	base := j.Pozo + j.PorIgualar
	extra := j.Pone - j.PorIgualar
	if base <= 0 || extra <= 0 {
		return 0
	}
	return float64(extra) / float64(base)
}

// Asiento es el estado de un jugador dentro de la mano.
type Asiento struct {
	ID    string
	Silla int

	Saldo  int64 // Fichas que le quedan.
	Ronda  int64 // Apostado en la ronda en curso.
	Activo bool  // Sigue en la mano (no se retiró).
	AllIn  bool

	// Pendiente es true si todavía le toca actuar en la ronda en curso: no
	// habló, o alguien subió después de que hablara.
	Pendiente bool

	SaldoInicial int64 // Fichas al empezar la mano, antes de las ciegas.
}

// Mano es una mano reconstruida hasta el momento actual.
type Mano struct {
	ID          string
	CiegaGrande int64
	Boton       int

	// Asientos de quienes recibieron cartas, en orden de palabra postflop: el
	// primero es quien habla primero después del flop y el último es el botón
	// (o quien esté más cerca de él).
	Asientos []Asiento
	Jugadas  []Jugada

	Calle   Calle
	Pozo    int64
	Apuesta int64 // Apuesta más alta de la ronda en curso.
	Subidas int   // Apuestas y subidas en la calle en curso.

	// Completa es true si se reconstruyó desde el inicio de la mano. Si el bot
	// se conectó a mitad de mano no hay foto inicial y las jugadas faltan.
	Completa bool
}

// Asiento devuelve el asiento de un jugador, o nil si no está en la mano.
func (m *Mano) Asiento(id string) *Asiento {
	for i := range m.Asientos {
		if m.Asientos[i].ID == id {
			return &m.Asientos[i]
		}
	}
	return nil
}

// Rivales devuelve los IDs de quienes siguen en la mano además de `yo`.
func (m *Mano) Rivales(yo string) []string {
	var ids []string
	for _, a := range m.Asientos {
		if a.ID != yo && a.Activo {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// PendientesTras cuenta los rivales que todavía tienen que hablar en esta
// ronda después de `yo` (sin contar a los que están all-in).
func (m *Mano) PendientesTras(yo string) int {
	n := 0
	for _, a := range m.Asientos {
		if a.ID != yo && a.Activo && !a.AllIn && a.Pendiente {
			n++
		}
	}
	return n
}

// HablaUltimo indica si, después del flop, `yo` es el último en hablar entre
// los que siguen en la mano y todavía pueden apostar.
func (m *Mano) HablaUltimo(yo string) bool {
	for i := len(m.Asientos) - 1; i >= 0; i-- {
		a := m.Asientos[i]
		if a.Activo && !a.AllIn {
			return a.ID == yo
		}
	}
	return false
}

// Aposto indica si el jugador apostó o subió en la calle dada.
func (m *Mano) Aposto(id string, calle Calle) bool {
	for _, j := range m.Jugadas {
		if j.Jugador == id && j.Calle == calle && j.Tipo.Agresiva() {
			return true
		}
	}
	return false
}

// nuevaMano arma la mano con los asientos de la foto dada, sin jugadas.
func nuevaMano(foto *botsdk.EstadoPublico) *Mano {
	m := &Mano{
		ID:          foto.IDMano,
		CiegaGrande: max(foto.CiegaGrande, 1),
		Boton:       foto.PosicionBoton,
		Calle:       CalleDe(foto.Etapa),
		Pozo:        foto.Pozo,
		Apuesta:     foto.ApuestaActual,
	}

	maxSilla := 0
	for _, j := range foto.Jugadores {
		maxSilla = max(maxSilla, j.PosicionSilla)
	}
	sillas := maxSilla + 1

	for _, j := range foto.Jugadores {
		if !j.EnMano {
			continue
		}
		m.Asientos = append(m.Asientos, Asiento{
			ID:           j.ID,
			Silla:        j.PosicionSilla,
			Saldo:        j.Saldo,
			Ronda:        j.ApuestaRonda,
			Activo:       j.Activo,
			AllIn:        j.AllIn,
			Pendiente:    j.Activo && !j.AllIn,
			SaldoInicial: j.Saldo + j.ApuestaRonda,
		})
	}
	// Orden de palabra postflop: empieza el primero a la izquierda del botón.
	distancia := func(silla int) int { return ((silla-m.Boton-1)%sillas + sillas) % sillas }
	sort.SliceStable(m.Asientos, func(a, b int) bool {
		return distancia(m.Asientos[a].Silla) < distancia(m.Asientos[b].Silla)
	})
	return m
}

// Reconstruir vuelve a jugar la mano: parte de `inicio` (la foto del
// mano_inicio) y aplica el historial de `actual`. Si no hay foto inicial, o es
// de otra mano, devuelve la mano tal como se ve ahora, sin jugadas y con
// Completa en false.
func Reconstruir(inicio, actual *botsdk.EstadoPublico) *Mano {
	if inicio == nil || inicio.IDMano != actual.IDMano || len(inicio.HistorialAcciones) > 0 {
		return nuevaMano(actual)
	}

	m := nuevaMano(inicio)
	m.Completa = true
	m.Calle = Preflop

	for _, h := range actual.HistorialAcciones {
		m.aplicar(h)
	}

	// Si la foto actual ya está en una calle posterior a la última acción
	// (se repartieron cartas y todavía nadie habló), se abre esa calle.
	if calle := CalleDe(actual.Etapa); calle > m.Calle {
		m.abrirCalle(calle)
	}

	// La foto actual manda: si la reconstrucción se desvió en algo (una acción
	// que la mesa interpretó distinto), se corrigen saldos y pozo con lo que
	// la mesa dice. Las jugadas quedan como la mejor aproximación disponible.
	for _, j := range actual.Jugadores {
		if a := m.Asiento(j.ID); a != nil {
			a.Saldo, a.Ronda, a.Activo, a.AllIn = j.Saldo, j.ApuestaRonda, j.Activo, j.AllIn
			if !a.Activo || a.AllIn {
				a.Pendiente = false
			}
		}
	}
	m.Pozo, m.Apuesta = actual.Pozo, actual.ApuestaActual
	return m
}

func (m *Mano) abrirCalle(calle Calle) {
	m.Calle = calle
	m.Apuesta = 0
	m.Subidas = 0
	for i := range m.Asientos {
		a := &m.Asientos[i]
		a.Ronda = 0
		a.Pendiente = a.Activo && !a.AllIn
	}
}

func (m *Mano) enMano() int {
	n := 0
	for _, a := range m.Asientos {
		if a.Activo {
			n++
		}
	}
	return n
}

func (m *Mano) aplicar(h botsdk.AccionRegistrada) {
	if calle := CalleDe(h.Etapa); calle > m.Calle {
		m.abrirCalle(calle)
	}
	a := m.Asiento(h.IDJugador)
	if a == nil {
		return
	}

	j := Jugada{
		Jugador:    a.ID,
		Calle:      m.Calle,
		Pozo:       m.Pozo,
		PorIgualar: max(m.Apuesta-a.Ronda, 0),
		Apuesta:    m.Apuesta,
		Saldo:      a.Saldo,
		Subidas:    m.Subidas,
		EnMano:     m.enMano(),
	}

	total := a.Ronda
	switch h.Accion.Tipo {
	case botsdk.Fold:
		j.Tipo = Fold
		a.Activo = false
	case botsdk.Check:
		j.Tipo = Check
	case botsdk.Call:
		j.Tipo = Call
		total = a.Ronda + min(j.PorIgualar, a.Saldo)
	case botsdk.Bet, botsdk.Raise:
		total = min(max(h.Accion.Monto, a.Ronda), a.Ronda+a.Saldo)
		j.Tipo = Raise
		if m.Apuesta == 0 {
			j.Tipo = Bet
		}
	case botsdk.AllIn:
		total = a.Ronda + a.Saldo
		switch {
		case total <= m.Apuesta:
			j.Tipo = Call
		case m.Apuesta == 0:
			j.Tipo = Bet
		default:
			j.Tipo = Raise
		}
	default:
		j.Tipo = Check
	}

	j.Pone = total - a.Ronda
	j.Total = total
	a.Saldo -= j.Pone
	a.Ronda = total
	m.Pozo += j.Pone
	if a.Saldo == 0 && a.Activo {
		a.AllIn = true
		j.AllIn = true
	}
	a.Pendiente = false

	if total > m.Apuesta {
		m.Apuesta = total
		m.Subidas++
		for i := range m.Asientos {
			otro := &m.Asientos[i]
			if otro.ID != a.ID && otro.Activo && !otro.AllIn {
				otro.Pendiente = true
			}
		}
	}

	m.Jugadas = append(m.Jugadas, j)
}
