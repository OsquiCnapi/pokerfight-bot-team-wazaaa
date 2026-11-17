// Package estrategia decide qué jugar. Junta las tres piezas del bot:
//
//   - partida: qué pasó en la mano hasta ahora.
//   - rivales: cómo suele jugar cada uno.
//   - equity: cuánto vale la mano propia contra lo que pueden tener.
//
// El recorrido de cada decisión es siempre el mismo: reconstruir la mano,
// estimar el rango de cada rival a partir de sus acciones (lectura.go),
// simular la equidad contra esos rangos y elegir la jugada de mayor valor
// esperado (preflop.go y postflop.go).
package estrategia

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/partida"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/rivales"
)

// presupuesto es el tiempo máximo que una decisión dedica a simular. Está muy
// por debajo del plazo de la mesa (5 s): lo normal es decidir en pocos
// milisegundos, y esto es solo el techo si la máquina está cargada.
const presupuesto = 700 * time.Millisecond

// Cerebro guarda la memoria del bot entre jugadas y toma las decisiones.
type Cerebro struct {
	mu     sync.Mutex
	modelo *rivales.Modelo
	sim    *equity.Simulador

	miID   string
	inicio *botsdk.EstadoPublico // Foto del mano_inicio de la mano en curso.
	ultimo *botsdk.EstadoPublico // Foto más reciente de la mano en curso.

	// Para reconocer la mesa final (ver notarMesa).
	enMesa      map[string]bool // Jugadores vistos en la mesa actual.
	manosEnMesa int
	mesas       int // Mesas anteriores a la actual.
	final       bool

	// fuerzas guarda la tabla de fuerzas de cada calle de la mano en curso,
	// indexada por cantidad de comunitarias. Se calcula una vez por calle.
	fuerzas   map[int]*equity.FuerzaMesa
	manoTabla string

	// P son los números ajustables de la estrategia.
	P Parametros

	// Registro, si no es nil, recibe una línea por decisión.
	Registro *log.Logger
}

// Nuevo crea un cerebro sin memoria de rivales.
func Nuevo(semilla uint64) *Cerebro {
	return &Cerebro{
		modelo: rivales.NuevoModelo(),
		sim:    equity.NuevoSimulador(semilla),
		P:      PorDefecto(),
	}
}

// ManoInicio guarda la foto inicial de la mano: es el punto de partida para
// reconstruirla después.
func (c *Cerebro) ManoInicio(estado botsdk.EstadoPublico, _ botsdk.Mano) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inicio = &estado
	c.ultimo = &estado
	c.notarMesa(&estado)
	if c.Registro != nil {
		for _, j := range estado.Jugadores {
			if j.ID == c.miID {
				c.Registro.Printf("[inicio ] %s stack %d", estado.IDMano, j.Saldo+j.ApuestaRonda)
			}
		}
	}
}

// manosPorMesa es el máximo de manos de una mesa de la clasificatoria.
const manosPorMesa = 50

// notarMesa lleva la cuenta de en qué mesa está el bot y decide si es la
// final. La arena no avisa en qué fase se está, pero la final se reconoce de
// dos maneras:
//
//   - Todos empiezan con las mismas fichas después de haber jugado otras
//     mesas. En la clasificatoria cada uno arrastra sus fichas, así que tras
//     la primera ronda los stacks ya no coinciden.
//   - La mesa pasa de las 50 manos, que es el tope de la clasificatoria.
func (c *Cerebro) notarMesa(estado *botsdk.EstadoPublico) {
	nueva := c.enMesa == nil
	for _, j := range estado.Jugadores {
		if j.EnMano && !c.enMesa[j.ID] {
			nueva = true
		}
	}
	if nueva {
		if c.enMesa != nil {
			c.mesas++
		}
		c.enMesa = map[string]bool{}
		for _, j := range estado.Jugadores {
			c.enMesa[j.ID] = true
		}
		c.manosEnMesa = 0

		iguales, primero := true, int64(-1)
		for _, j := range estado.Jugadores {
			if !j.EnMano {
				continue
			}
			if stack := j.Saldo + j.ApuestaRonda; primero < 0 {
				primero = stack
			} else if stack != primero {
				iguales = false
			}
		}
		c.final = c.mesas >= 1 && iguales
	}
	c.manosEnMesa++
	if c.manosEnMesa > manosPorMesa+5 {
		c.final = true
	}
}

// EstadoActualizado guarda la foto más completa de la mano en curso.
func (c *Cerebro) EstadoActualizado(estado botsdk.EstadoPublico) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordar(&estado)
}

func (c *Cerebro) recordar(estado *botsdk.EstadoPublico) {
	if c.ultimo == nil || c.ultimo.IDMano != estado.IDMano ||
		len(estado.HistorialAcciones) >= len(c.ultimo.HistorialAcciones) {
		c.ultimo = estado
	}
}

// ManoFin le pasa la mano terminada al modelo de rivales.
func (c *Cerebro) ManoFin(resultado botsdk.ResultadoMano) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ultimo == nil || c.ultimo.IDMano != resultado.IDMano {
		return
	}
	mano := partida.Reconstruir(c.inicio, c.ultimo)

	mostradas := make(map[string][2]equity.Carta, len(resultado.Mostradas))
	for id, cartas := range resultado.Mostradas {
		if equity.Valida(cartas[0]) && equity.Valida(cartas[1]) && cartas[0] != cartas[1] {
			mostradas[id] = [2]equity.Carta{equity.DeSDK(cartas[0]), equity.DeSDK(cartas[1])}
		}
	}
	c.modelo.Observar(mano, c.miID, mostradas, convertir(resultado.Comunitarias))
	if c.Registro != nil {
		texto := ""
		for id, cartas := range resultado.Mostradas {
			texto += motivo(" %s=%s%s(%s)", id, cartas[0], cartas[1], resultado.Descripcion[id])
		}
		for _, r := range resultado.Repartos {
			texto += motivo(" gana:%s+%d", r.IDJugador, r.Monto)
		}
		c.Registro.Printf("[fin    ] mesa %s |%s", textoCartas(resultado.Comunitarias), texto)
		// Cada tanto, lo aprendido de cada rival de la mesa.
		if c.manosEnMesa%25 == 0 {
			for _, a := range mano.Asientos {
				if a.ID != c.miID {
					c.Registro.Printf("[rival  ] %-14s %s", a.ID, c.modelo.De(a.ID).Resumen())
				}
			}
		}
	}
	c.inicio, c.ultimo = nil, nil
}

func convertir(cartas []botsdk.Carta) []equity.Carta {
	salida := make([]equity.Carta, 0, len(cartas))
	for _, carta := range cartas {
		if equity.Valida(carta) {
			salida = append(salida, equity.DeSDK(carta))
		}
	}
	return salida
}

// rival es un jugador que sigue en la mano, con lo que se sabe de él.
type rival struct {
	id      string
	perfil  *rivales.Perfil
	asiento *partida.Asiento
	rango   *equity.Rango
}

// situacion es todo lo que una decisión necesita, ya calculado.
type situacion struct {
	t    botsdk.Turno
	m    *partida.Mano
	yo   string
	mano [2]equity.Carta
	mesa []equity.Carta

	cg         int64 // Ciega grande.
	pozo       int64
	porIgualar int64 // Lo que cuesta pagar, recortado al saldo propio.
	saldo      int64
	ronda      int64 // Lo ya apostado en la ronda.

	rivales []rival
	limite  time.Time
	p       *Parametros
	reserva float64 // Fichas de reserva para valor(): ver Parametros.ReservaCiegas.
}

// tipoDecision es la intención de la estrategia, antes de traducirla a una
// acción válida del protocolo.
type tipoDecision int

const (
	retirarse tipoDecision = iota // Fold, o check si pasar es gratis.
	pagar                         // Call, o check si no hay nada que pagar.
	apostar                       // Bet o raise hasta `total`.
	todoAdentro
)

type decision struct {
	tipo   tipoDecision
	total  int64 // Para apostar: total de la ronda, como el monto del protocolo.
	motivo string
}

// Decidir elige la jugada del turno.
func (c *Cerebro) Decidir(_ context.Context, t botsdk.Turno) botsdk.Accion {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.miID = t.IDJugador
	estado := t.Estado
	c.recordar(&estado)

	if !equity.Valida(t.Mano[0]) || !equity.Valida(t.Mano[1]) || t.Mano[0] == t.Mano[1] {
		// Sin cartas (una reconexión a mitad de mano) no hay nada que estimar.
		return Segura(t)
	}

	s := c.situar(t)
	var d decision
	if len(s.mesa) < 3 {
		d = c.preflop(s)
	} else {
		d = c.postflop(s)
	}
	accion := traducir(d, t)

	if c.Registro != nil {
		quienes := ""
		for _, rv := range s.rivales {
			quienes += " " + rv.id
		}
		c.Registro.Printf("[%-7s] %s%s | mesa %-14s | pozo %5d pagar %5d saldo %6d | %-5s %-6d | %s | vs%s",
			t.Estado.Etapa, t.Mano[0], t.Mano[1], textoCartas(t.Estado.Comunitarias),
			s.pozo, s.porIgualar, s.saldo, accion.Tipo, accion.Monto, d.motivo, quienes)
	}
	return accion
}

func textoCartas(cartas []botsdk.Carta) string {
	if len(cartas) == 0 {
		return "-"
	}
	texto := ""
	for _, carta := range cartas {
		texto += carta.String()
	}
	return texto
}

// situar reconstruye la mano y lee el rango de cada rival que sigue en ella.
func (c *Cerebro) situar(t botsdk.Turno) *situacion {
	estado := t.Estado
	s := &situacion{
		t:          t,
		m:          partida.Reconstruir(c.inicio, &estado),
		yo:         t.IDJugador,
		mano:       [2]equity.Carta{equity.DeSDK(t.Mano[0]), equity.DeSDK(t.Mano[1])},
		mesa:       convertir(t.Estado.Comunitarias),
		cg:         max(t.Estado.CiegaGrande, 1),
		pozo:       t.Estado.Pozo,
		porIgualar: min(t.PorIgualar(), t.Yo.Saldo),
		saldo:      t.Yo.Saldo,
		ronda:      t.Yo.ApuestaRonda,
		limite:     time.Now().Add(presupuesto),
		p:          &c.P,
	}
	s.reserva = c.P.ReservaCiegas * float64(s.cg)
	if c.final {
		s.reserva = c.P.ReservaFinal * float64(s.cg)
	}
	if t.Timeout > 0 {
		if tope := time.Now().Add(t.Timeout / 3); tope.Before(s.limite) {
			s.limite = tope
		}
	}

	if c.manoTabla != estado.IDMano {
		c.manoTabla = estado.IDMano
		c.fuerzas = map[int]*equity.FuerzaMesa{}
	}

	for i := range s.m.Asientos {
		a := &s.m.Asientos[i]
		if a.ID == s.yo || !a.Activo {
			continue
		}
		s.rivales = append(s.rivales, rival{
			id:      a.ID,
			perfil:  c.modelo.De(a.ID),
			asiento: a,
			rango:   c.leerRango(s.m, a.ID, s.mesa),
		})
	}
	return s
}

// fuerza devuelve la tabla de fuerzas de la mesa en la calle dada.
func (c *Cerebro) fuerza(calle partida.Calle, mesa []equity.Carta) *equity.FuerzaMesa {
	cuantas := 2 + int(calle)
	if cuantas > len(mesa) || cuantas < 3 {
		return nil
	}
	if f, ok := c.fuerzas[cuantas]; ok {
		return f
	}
	f := equity.CalcularFuerza(mesa[:cuantas])
	c.fuerzas[cuantas] = f
	return f
}

// Segura es la jugada que haría la mesa por el bot: pasar si es gratis,
// retirarse si no. Es el último recurso cuando algo falla.
func Segura(t botsdk.Turno) botsdk.Accion {
	if t.Puede(botsdk.Check) {
		return t.Check()
	}
	return t.Fold()
}

// traducir convierte la decisión en una acción que la mesa acepta. Los montos
// los resuelve el SDK (t.Apostar), que recorta a la subida mínima y al all-in.
func traducir(d decision, t botsdk.Turno) botsdk.Accion {
	pagarOPasar := func() botsdk.Accion {
		switch {
		case t.Puede(botsdk.Check):
			return t.Check()
		case t.Puede(botsdk.Call):
			return t.Pagar()
		case t.Puede(botsdk.AllIn) && t.PorIgualar() >= t.Yo.Saldo:
			// Pagar cuesta todo lo que queda: el all-in es la forma de pagar.
			return t.AllIn()
		}
		return t.Fold()
	}

	switch d.tipo {
	case todoAdentro:
		if t.Puede(botsdk.AllIn) {
			return t.AllIn()
		}
		return pagarOPasar()
	case apostar:
		if !t.Puede(botsdk.Bet) && !t.Puede(botsdk.Raise) {
			if d.total >= t.Techo() && t.Puede(botsdk.AllIn) {
				return t.AllIn()
			}
			return pagarOPasar()
		}
		return t.Apostar(d.total)
	case pagar:
		return pagarOPasar()
	}
	return Segura(t)
}

func sigmoide(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

func acotar(x, minimo, maximo float64) float64 { return math.Max(minimo, math.Min(maximo, x)) }

func motivo(formato string, args ...any) string { return fmt.Sprintf(formato, args...) }
