package estrategia

import (
	"math"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/partida"
)

// Después del flop.
//
// Cada jugada posible se convierte en un valor en fichas, medido contra
// retirarse (que vale 0), y se elige la mayor. El valor descuenta el riesgo de
// quedar eliminado (ver valor):
//
//   - Pasar o pagar: equidad contra los rangos × lo que hay para ganar, menos
//     lo que cuesta.
//   - Apostar o subir: con cierta probabilidad todos se retiran y el pozo es
//     nuestro; si no, se juega un pozo más grande contra el rango con el que
//     pagan, que es más fuerte que el de antes.
//
// La misma cuenta cubre la apuesta por valor (pagan manos peores), el farol
// (se retiran manos mejores) y el caso del medio, donde apostar solo logra que
// paguen las manos que ganan: ahí el número de pasar sale más alto.

var (
	// Tamaños de apuesta que se comparan, como fracción del pozo.
	fraccionesApuesta = []float64{0.33, 0.66, 1.0, 1.5}
	// Tamaños de subida: lo que se agrega sobre igualar, como fracción del
	// pozo que quedaría tras igualar.
	fraccionesSubida = []float64{0.6, 1.0}
)

// desenlace es un final posible de una jugada: con qué probabilidad ocurre y
// con cuántas fichas deja al bot.
type desenlace struct {
	prob, fichas float64
}

// valor convierte una jugada con varios finales posibles en una sola cifra:
// cuántas fichas seguras equivale a jugarla, medidas contra retirarse ahora.
//
// No es el promedio simple. En este torneo quedarse sin fichas es quedar
// eliminado, así que perder el stack cuesta mucho más que lo que vale ganar
// otro igual. Para reflejarlo se promedia el logaritmo de las fichas (el
// criterio de Kelly): las apuestas chicas se valoran casi por su promedio,
// pero una moneda al aire por todo el stack sale negativa. Con 100 ciegas y
// la reserva de la clasificatoria, pagar un all-in exige cerca de 75% de
// equidad en vez de 50%.
//
// La reserva (Parametros.ReservaCiegas, o ReservaFinal en la mesa final) evita
// que quebrar valga menos infinito y regula la aversión: más reserva, más
// parecido al promedio simple.
func (s *situacion) valor(desenlaces ...desenlace) float64 {
	reserva := s.reserva
	esperado := 0.0
	for _, d := range desenlaces {
		esperado += d.prob * math.Log(math.Max(d.fichas, 0)+reserva)
	}
	return math.Exp(esperado) - reserva - float64(s.saldo)
}

// valorPagar es el valor de igualar `costo` con esa equidad.
func (s *situacion) valorPagar(eq, costo float64) float64 {
	saldo, pozo := float64(s.saldo), float64(s.pozo)
	return s.valor(desenlace{eq, saldo + pozo}, desenlace{1 - eq, saldo - costo})
}

type opcion struct {
	d  decision
	ev float64
}

func (c *Cerebro) postflop(s *situacion) decision {
	if len(s.rivales) == 0 {
		return decision{tipo: pagar, motivo: "no queda nadie más en la mano"}
	}

	calle := partida.Calle(len(s.mesa) - 2)
	f := c.fuerza(calle, s.mesa)
	rangos := make([]*equity.Rango, len(s.rivales))
	for i, rv := range s.rivales {
		rangos[i] = rv.rango
	}
	eq, _ := c.sim.Equidad(s.mano, s.mesa, rangos, 12000, s.limite)

	pozo, costo, saldo := float64(s.pozo), float64(s.porIgualar), float64(s.saldo)
	river := len(s.mesa) == 5
	pendientes := s.m.PendientesTras(s.yo)
	puedeSubir := s.t.Puede(botsdk.Bet) || s.t.Puede(botsdk.Raise) ||
		(s.t.Puede(botsdk.AllIn) && s.saldo > s.porIgualar)

	// Poner más de lo que el rival con más fichas puede cubrir no sirve: el
	// exceso vuelve.
	var cubre float64
	for _, rv := range s.rivales {
		cubre = math.Max(cubre, float64(rv.asiento.Saldo+rv.asiento.Ronda-s.ronda))
	}
	maximo := math.Min(saldo, cubre)

	// Arma la decisión de poner `pone` fichas más en esta ronda.
	agresiva := func(pone float64, texto string) decision {
		if pone >= saldo-0.5 {
			return decision{tipo: todoAdentro, motivo: texto}
		}
		return decision{tipo: apostar, total: s.ronda + int64(pone), motivo: texto}
	}

	// Lista de tamaños distintos entre el mínimo legal y el máximo útil.
	candidatos := func(fracciones []float64, base, piso float64, conAllIn bool) []float64 {
		var lista []float64
		agregar := func(pone float64) {
			pone = acotar(pone, piso, maximo)
			// Poner casi la mitad del stack ya es jugárselo entero: las fichas
			// que quedan detrás van a entrar igual. Mejor decirlo de una vez y
			// que la cuenta vea todo el riesgo.
			if pone >= 0.45*maximo {
				pone = maximo
			}
			for _, ya := range lista {
				if math.Abs(ya-pone) <= 0.08*ya {
					return
				}
			}
			lista = append(lista, pone)
		}
		for _, fr := range fracciones {
			agregar(costo + fr*base)
		}
		if conAllIn {
			agregar(maximo)
		}
		return lista
	}
	minimo := float64(s.t.SubidaMinima() - s.ronda)

	if costo == 0 {
		// Nadie apostó: pasar o apostar. Pasar no gana el pozo entero aunque la
		// mano sea la mejor: fuera de posición y con manos débiles se realiza
		// menos equidad, porque después toca decidir sin información.
		realiza := 0.95
		if !s.m.HablaUltimo(s.yo) {
			realiza = 0.88
		}
		if river {
			realiza += 0.05
		}
		if eq < 0.35 {
			realiza -= 0.2 * (0.35 - eq) / 0.35
		}
		evPasar := s.valor(desenlace{eq * realiza, saldo + pozo}, desenlace{1 - eq*realiza, saldo})
		mejor := opcion{decision{tipo: pagar, motivo: motivo("paso: equidad %.0f%%", 100*eq)}, evPasar}

		if puedeSubir && maximo >= minimo {
			conAllIn := maximo <= 3*pozo || eq >= 0.85
			for _, pone := range candidatos(fraccionesApuesta, pozo, minimo, conAllIn) {
				ev, seRetiran, eqSiPagan := c.evAgresiva(s, f, pone)
				// Las apuestas grandes solo tienen sentido en los extremos:
				// con ventaja clara cuando pagan, o cuando casi siempre se
				// retiran. Con una mano media inflan un pozo que no se quiere
				// jugar grande.
				if tamano := pone / pozo; tamano > 0.8 {
					minimaSiPagan, minimoRetiro := 0.58, 0.50
					if tamano > 1.2 {
						minimaSiPagan, minimoRetiro = 0.66, 0.60
					}
					if eqSiPagan < minimaSiPagan && seRetiran < minimoRetiro {
						continue
					}
				}
				exige := s.p.ExigeApuesta * pozo
				if eqSiPagan < 0.30 {
					// Farol puro: solo contra pocos rivales y con margen, porque
					// depende entero de una estimación de cuánto se retiran.
					if len(s.rivales) > 2 {
						continue
					}
					exige = s.p.ExigeFarol * pozo
				}
				if ev-exige > mejor.ev {
					mejor = opcion{agresiva(pone, motivo("apuesto %.0f%% del pozo: equidad %.0f%%, se retiran %.0f%%, si pagan %.0f%%",
						100*pone/pozo, 100*eq, 100*seRetiran, 100*eqSiPagan)), ev - exige}
				}
			}
		}
		return mejor.d
	}

	// Hay una apuesta delante: retirarse, pagar o subir. El margen cubre lo
	// que la equidad no ve: que quedan calles por pagar y rivales por hablar.
	margen := s.p.MargenPagar
	if river {
		margen = s.p.MargenPagarRiver
	}
	margen += 0.025 * float64(pendientes)
	evPagar := s.valorPagar(eq, costo) - margen*(pozo+costo)

	// Si después de pagar quedan pocas fichas detrás en relación al pozo, pagar
	// es comprometerse: lo que queda va a entrar en la calle siguiente. Se
	// valora entonces como lo que es, jugarse todo el stack efectivo.
	if !river {
		var detras float64
		for _, rv := range s.rivales {
			if !rv.asiento.AllIn {
				detras = math.Max(detras, float64(rv.asiento.Saldo))
			}
		}
		if resto := math.Min(saldo-costo, detras); resto > 0 && resto < 1.2*(pozo+costo) {
			todo := s.valor(desenlace{eq, saldo + pozo + resto}, desenlace{1 - eq, saldo - costo - resto}) - margen*(pozo+costo)
			evPagar = math.Min(evPagar, todo)
		}
	}

	mejor := opcion{decision{tipo: retirarse, motivo: motivo("me retiro: equidad %.0f%%, pagar cuesta %.0f%% del pozo final", 100*eq, 100*costo/(pozo+costo))}, 0}
	if evPagar > 0 {
		mejor = opcion{decision{tipo: pagar, motivo: motivo("pago: equidad %.0f%% contra precio %.0f%%", 100*eq, 100*costo/(pozo+costo))}, evPagar}
	}

	if puedeSubir && maximo > costo {
		conAllIn := maximo <= 2.5*(pozo+costo) || eq >= 0.9
		for _, pone := range candidatos(fraccionesSubida, pozo+costo, math.Max(minimo, costo+1), conAllIn) {
			if pone <= costo {
				continue
			}
			ev, seRetiran, eqSiPagan := c.evAgresiva(s, f, pone)
			// Subir reabre la apuesta: si el rival resube hay que tirar la
			// mano o jugarse todo. Solo vale con ventaja clara cuando pagan
			// (subida por valor) o si casi siempre se retiran (farol).
			if eqSiPagan < s.p.SubirSiPagan && seRetiran < s.p.SubirSiSeRetiran {
				continue
			}
			exige := 0.04 * pozo
			if eqSiPagan < 0.45 {
				exige = 0.10 * pozo
			}
			if ev-exige > mejor.ev {
				mejor = opcion{agresiva(pone, motivo("subo: equidad %.0f%%, se retiran %.0f%%, si pagan %.0f%%",
					100*eq, 100*seRetiran, 100*eqSiPagan)), ev - exige}
			}
		}
	}
	return mejor.d
}

// evAgresiva estima el valor esperado de poner `pone` fichas más en la ronda
// (incluye lo que haga falta para igualar). Devuelve también la probabilidad
// de que todos se retiren y la equidad contra los rangos con que pagan.
func (c *Cerebro) evAgresiva(s *situacion, f *equity.FuerzaMesa, pone float64) (ev, seRetiran, eqSiPagan float64) {
	pozo, cg := float64(s.pozo), float64(s.cg)
	total := float64(s.ronda) + pone

	muertas := equity.Mascara(s.mano[0]) | equity.Mascara(s.mano[1])
	for _, carta := range s.mesa {
		muertas |= equity.Mascara(carta)
	}

	seRetiran = 1
	siguen := make([]*equity.Rango, 0, len(s.rivales))
	var aportan float64 // Fichas que se espera que agreguen los que pagan.
	for i := range s.rivales {
		rv := &s.rivales[i]
		cuesta := math.Min(total-float64(rv.asiento.Ronda), float64(rv.asiento.Saldo))
		if rv.asiento.AllIn || cuesta <= 0 {
			// Ya no tiene nada que decidir: sigue hasta el showdown.
			seRetiran = 0
			siguen = append(siguen, rv.rango)
			continue
		}
		fraccion := cuesta / math.Max(pozo+pone-cuesta, 1)
		agresor := s.m.Aposto(rv.id, s.m.Calle)
		prob, sigue := respuesta(rv, f, fraccion, cuesta/cg, agresor, muertas, s.p.MezclaRetiro)
		seRetiran *= prob
		siguen = append(siguen, sigue)
		aportan += (1 - prob) * cuesta
	}

	eqSiPagan, _ = c.sim.Equidad(s.mano, s.mesa, siguen, 4000, s.limite)
	if seRetiran > 0.999 {
		return pozo, seRetiran, eqSiPagan
	}
	// aportan es un promedio sobre todos los casos; lo que importa es cuánto
	// agregan cuando al menos uno paga.
	aportan /= 1 - seRetiran
	saldo := float64(s.saldo)
	ev = s.valor(
		desenlace{seRetiran, saldo + pozo},
		desenlace{(1 - seRetiran) * eqSiPagan, saldo + pozo + aportan},
		desenlace{(1 - seRetiran) * (1 - eqSiPagan), saldo - pone},
	)
	return ev, seRetiran, eqSiPagan
}
