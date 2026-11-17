package estrategia

import (
	"math"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/partida"
)

// Preflop.
//
// Con dos cartas la decisión es sobre todo de rangos: con qué fracción de las
// manos conviene hacer cada cosa según la posición, y qué rango representa lo
// que hicieron los rivales. `puesto` es qué fracción de las manos iniciales es
// mejor que la propia (0 es AA).

// rangoAbrir es con qué fracción de las manos se abre subiendo según cuántos
// rivales faltan hablar. Con pocos detrás (botón, ciega chica) se abre mucho
// más: hay menos manos que puedan ser mejores y se roban las ciegas.
var rangoAbrir = [...]float64{0.12, 0.48, 0.42, 0.30, 0.23, 0.18}

// rangoEmpujar es lo mismo para ir all-in directo con unas 10 ciegas.
var rangoEmpujar = [...]float64{0.25, 0.60, 0.45, 0.33, 0.26, 0.21}

const (
	// stackCorto es el stack efectivo, en ciegas, por debajo del cual se juega
	// a todo o nada: con tan poco, una subida normal ya compromete el stack.
	stackCorto = 12.0
	// compromiso es la parte del stack a partir de la cual pagar se trata como
	// una decisión por todas las fichas.
	compromiso = 0.35
)

func (c *Cerebro) preflop(s *situacion) decision {
	puesto := equity.PuestoPreflop(s.mano[0], s.mano[1])
	cg := float64(s.cg)
	miTotal := float64(s.saldo + s.ronda)

	// Stack efectivo: lo máximo que de verdad se puede ganar o perder.
	var mayorRival float64
	for _, rv := range s.rivales {
		mayorRival = math.Max(mayorRival, float64(rv.asiento.Saldo+rv.asiento.Ronda))
	}
	efectivo := math.Min(miTotal, mayorRival) / cg
	detras := s.m.PendientesTras(s.yo)

	if s.m.Subidas == 0 {
		return c.sinSubida(s, puesto, efectivo, detras)
	}
	return c.anteSubida(s, puesto, efectivo, detras)
}

// sinSubida decide cuando nadie subió todavía: abrir, entrar barato o irse.
func (c *Cerebro) sinSubida(s *situacion, puesto, efectivo float64, detras int) decision {
	cg := float64(s.cg)
	miTotal := float64(s.saldo + s.ronda)

	limpers := 0
	for _, j := range s.m.Jugadas {
		if j.Calle == partida.Preflop && j.Tipo == partida.Call {
			limpers++
		}
	}
	indice := min(detras, len(rangoAbrir)-1)

	if efectivo <= stackCorto {
		tope := rangoEmpujar[indice] * math.Pow(10/math.Max(efectivo, 3), 0.7)
		if limpers > 0 {
			tope *= 0.6
		}
		if puesto <= tope {
			return decision{tipo: todoAdentro, motivo: motivo("stack corto (%.0f ciegas): empujo el %.0f%% mejor", efectivo, 100*tope)}
		}
		return decision{tipo: retirarse, motivo: "stack corto: no alcanza para empujar"}
	}

	// Subir a tres ciegas, más una por cada jugador que ya entró.
	objetivo := float64(3+limpers) * cg

	// Probabilidad de que todos los que faltan (y los que entraron pagando) se
	// retiren ante esa subida, según sus perfiles.
	seRetiranTodos := 1.0
	for _, rv := range s.rivales {
		cuesta := objetivo - float64(rv.asiento.Ronda)
		if cuesta <= 0 {
			continue
		}
		pozoTrasSubir := float64(s.pozo) + objetivo - float64(s.ronda)
		fraccion := cuesta / math.Max(pozoTrasSubir-cuesta, 1)
		seRetiranTodos *= rv.perfil.ProbRetiroPre(fraccion, cuesta/cg, false)
	}

	tope := rangoAbrir[indice] * s.p.Abrir
	if limpers > 0 {
		tope *= 0.75
	}
	// Si se van a retirar casi siempre, la subida gana las ciegas sola y
	// conviene hacerla con muchas más manos.
	if seRetiranTodos >= 0.6 {
		tope = math.Max(tope, math.Min(0.95, 0.45+2.5*(seRetiranTodos-0.6)))
	}

	if puesto <= tope {
		// Con lo mejor y rivales que pagan, que paguen más caro.
		if puesto <= 0.06 && seRetiranTodos < 0.3 {
			objetivo += cg
		}
		if objetivo-float64(s.ronda) >= compromiso*miTotal {
			return decision{tipo: todoAdentro, motivo: "abrir ya compromete el stack: all-in"}
		}
		return decision{tipo: apostar, total: int64(objetivo),
			motivo: motivo("abro: puesto %.0f%% dentro del %.0f%% (se retiran todos %.0f%%)", 100*puesto, 100*tope, 100*seRetiranTodos)}
	}

	// Entrar barato con manos jugables: detrás de otros que ya entraron, o
	// completando desde la ciega chica.
	if s.porIgualar > 0 && float64(s.porIgualar) <= cg {
		completa := float64(s.porIgualar) < cg
		switch {
		case limpers > 0 && puesto <= 0.42:
			return decision{tipo: pagar, motivo: "entro barato detrás de otros"}
		case completa && puesto <= 0.60:
			return decision{tipo: pagar, motivo: "completo la ciega chica"}
		}
	}
	return decision{tipo: retirarse, motivo: motivo("puesto %.0f%% fuera del %.0f%%", 100*puesto, 100*tope)}
}

// anteSubida decide cuando ya hubo al menos una subida: resubir, pagar o irse.
// Aquí sí se simula: la mano propia contra el rango de quienes pusieron fichas.
func (c *Cerebro) anteSubida(s *situacion, puesto, efectivo float64, detras int) decision {
	cg := float64(s.cg)
	miTotal := float64(s.saldo + s.ronda)

	// Los que ya igualaron la subida (o subieron) son contra quienes se juega.
	// Los que todavía tienen que responder casi siempre se retiran.
	var comprometidos []rival
	for _, rv := range s.rivales {
		if rv.asiento.Ronda >= s.m.Apuesta || rv.asiento.AllIn {
			comprometidos = append(comprometidos, rv)
		}
	}
	if len(comprometidos) == 0 {
		comprometidos = s.rivales
	}
	pendientes := len(s.rivales) - len(comprometidos)

	rangos := make([]*equity.Rango, len(comprometidos))
	hayAllIn := false
	for i, rv := range comprometidos {
		rangos[i] = rv.rango
		hayAllIn = hayAllIn || rv.asiento.AllIn
	}
	eq, _ := c.sim.Equidad(s.mano, nil, rangos, 15000, s.limite)

	n := float64(len(comprometidos))
	parteJusta := 1 / (n + 1)
	ventaja := (eq - parteJusta) / (1 - parteJusta)

	costo := float64(s.porIgualar)
	precio := costo / (float64(s.pozo) + costo)

	// valorTodo es el valor de jugarse todo el stack efectivo contra quienes
	// ya están comprometidos, sin contar con que alguno se retire.
	valorTodo := func() float64 {
		var mayor float64
		for _, rv := range comprometidos {
			mayor = math.Max(mayor, float64(rv.asiento.Saldo+rv.asiento.Ronda))
		}
		arriesgo := math.Max(math.Min(float64(s.saldo), mayor-float64(s.ronda)), costo)
		return s.valor(
			desenlace{eq, float64(s.saldo) + float64(s.pozo) + arriesgo - costo},
			desenlace{1 - eq, float64(s.saldo) - arriesgo},
		)
	}
	parteDelStack := costo / math.Max(float64(s.saldo), 1)
	puedeSubir := s.t.Puede(botsdk.Raise) || (s.t.Puede(botsdk.AllIn) && s.saldo > s.porIgualar)

	// Decisión por todas las fichas (o casi): solo cuenta la equidad contra el
	// precio, con un margen que crece con lo que se arriesga.
	if parteDelStack >= compromiso || (hayAllIn && costo >= 10*cg) || efectivo <= stackCorto {
		// valorPagar ya descuenta el riesgo de quedar eliminado; el margen
		// cubre a los rivales que todavía pueden entrar.
		margen := (0.02 + 0.02*float64(pendientes)) * (float64(s.pozo) + costo)
		if s.valorPagar(eq, costo) < margen {
			return decision{tipo: retirarse, motivo: motivo("equidad %.0f%% no alcanza para jugarse %.0f%% del stack (precio %.0f%%)", 100*eq, 100*parteDelStack, 100*precio)}
		}
		if ventaja >= 0.25 && puedeSubir && valorTodo() >= s.valorPagar(eq, costo) {
			return decision{tipo: todoAdentro, motivo: motivo("equidad %.0f%% contra su rango: all-in", 100*eq)}
		}
		return decision{tipo: pagar, motivo: motivo("equidad %.0f%% alcanza para jugarse %.0f%% del stack (precio %.0f%%)", 100*eq, 100*parteDelStack, 100*precio)}
	}

	enPosicion := s.m.HablaUltimo(s.yo)

	// Resubir por valor: clara ventaja contra su rango y una mano de primera.
	if puedeSubir && puesto <= 0.12 && ventaja >= 0.20+0.04*float64(s.m.Subidas-1) {
		multiplo := 3.0
		if !enPosicion {
			multiplo = 3.5
		}
		objetivo := multiplo*float64(s.m.Apuesta) + float64(len(comprometidos)-1)*float64(s.m.Apuesta)
		if objetivo-float64(s.ronda) < compromiso*miTotal {
			return decision{tipo: apostar, total: int64(objetivo), motivo: motivo("resubo por valor: equidad %.0f%%", 100*eq)}
		}
		// Resubir ya compromete el stack: solo si jugárselo entero vale la pena.
		if todo := valorTodo(); todo > 0 && todo >= s.valorPagar(eq, costo) {
			return decision{tipo: todoAdentro, motivo: motivo("resubir compromete el stack, equidad %.0f%%: all-in", 100*eq)}
		}
	}

	margen := 0.025 * float64(pendientes)
	if !enPosicion {
		margen += 0.03
	}
	if s.m.Subidas >= 2 {
		margen += 0.05
	}
	if eq >= precio+margen {
		return decision{tipo: pagar, motivo: motivo("equidad %.0f%% >= precio %.0f%% + margen %.0f%%", 100*eq, 100*precio, 100*margen)}
	}

	// Un par chico rara vez es favorito, pero cuando liga trío gana un pozo
	// grande: vale pagar barato si hay fichas detrás para cobrarlo.
	esPar := equity.Valor(s.mano[0]) == equity.Valor(s.mano[1])
	if esPar && costo <= 0.06*efectivo*cg && eq >= precio-0.07 {
		return decision{tipo: pagar, motivo: "par: pago barato buscando el trío"}
	}

	return decision{tipo: retirarse, motivo: motivo("equidad %.0f%% < precio %.0f%% + margen %.0f%%", 100*eq, 100*precio, 100*margen)}
}
