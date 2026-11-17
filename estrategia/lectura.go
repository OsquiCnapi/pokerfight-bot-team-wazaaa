package estrategia

import (
	"math"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/partida"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/rivales"
)

// Lectura de rangos.
//
// Cada rival empieza la mano pudiendo tener cualquier cosa. Cada acción suya
// dice algo: quien sube preflop rara vez tiene 72, quien apuesta el pozo en el
// river suele tener una mano hecha. Aquí cada acción multiplica el peso de
// cada mano posible por la probabilidad de que el rival jugara así teniéndola
// (una actualización bayesiana). Cuánto se le cree depende de su perfil: la
// apuesta de alguien que apuesta siempre pesa poco.
//
// Ninguna probabilidad llega a cero: siempre queda un piso, porque los bots
// hacen cosas raras y un rango vacío dejaría al bot sin referencia.

// leerRango estima el rango actual de un rival a partir de todas sus acciones
// en la mano.
func (c *Cerebro) leerRango(m *partida.Mano, id string, mesa []equity.Carta) *equity.Rango {
	r := equity.RangoCompleto()
	perfil := c.modelo.De(id)
	cg := float64(m.CiegaGrande)

	// agresor: si ya apostó o subió en la calle de la jugada que se está leyendo.
	calleAgresiva := partida.Calle(-1)
	for _, j := range m.Jugadas {
		if j.Jugador != id {
			continue
		}
		agresor := calleAgresiva == j.Calle
		if j.Calle == partida.Preflop {
			leerPreflop(r, j, perfil, cg, agresor)
		} else if f := c.fuerza(j.Calle, mesa); f != nil {
			leerPostflop(r, j, perfil, cg, f, agresor, c.P.MezclaPago)
		}
		if j.Tipo.Agresiva() {
			calleAgresiva = j.Calle
		}
	}
	return r
}

// dentro es la probabilidad de que una mano de ese puesto (0 la mejor) esté
// entre "el tope% mejor", con un borde suave: los bots no ordenan las manos
// exactamente igual que nosotros.
func dentro(puesto, tope float64) float64 {
	return sigmoide((tope - puesto) / (0.10*tope + 0.012))
}

func leerPreflop(r *equity.Rango, j partida.Jugada, p *rivales.Perfil, cg float64, agresor bool) {
	var tope, quitar float64
	piso := 0.03

	switch {
	case j.Tipo.Agresiva():
		switch {
		case j.AllIn || float64(j.Total) >= 15*cg:
			tope = p.RangoAllInPre()
			// Con pocas ciegas, ir all-in es lo normal y no dice tanto.
			if ciegas := float64(j.Saldo) / cg; ciegas <= 20 {
				tope = math.Max(tope, acotar(0.60-0.03*ciegas, 0.05, 0.5))
			}
		case j.Subidas == 0:
			tope = p.RangoAbrir()
		default:
			tope = p.RangoResubir()
			if j.Subidas >= 2 {
				tope *= 0.6
			}
		}

	case j.Tipo == partida.Call && j.Subidas == 0:
		// Entró pagando la ciega: tiene algo jugable, pero con lo mejor
		// probablemente habría subido.
		tope = p.RangoEntrar()
		quitar = 0.6 * p.RangoAbrir()
		piso = 0.06

	case j.Tipo == partida.Call:
		// Pagó una subida: se quedó con la parte de arriba de lo que ya tenía.
		// Qué parte la da su perfil: con qué frecuencia se retira ante ese
		// tamaño.
		ciegas := float64(j.PorIgualar) / cg
		seVa := p.ProbRetiroPre(j.Fraccion(), ciegas, agresor)
		limite := corte(r, func(i int) float64 { return 1 - equity.PuestoDeCombo(i) }, seVa)
		lento := 0.0
		if !j.AllIn && ciegas < 12 {
			lento = 0.45 // Con lo mejor suele resubir en vez de pagar.
		}
		tope := 0.8 * p.RangoResubir()
		for i := range r {
			if r[i] == 0 {
				continue
			}
			puesto := equity.PuestoDeCombo(i)
			creible := 0.03 + 0.97*sigmoide(((1-puesto)-limite)/0.03)
			creible *= 1 - lento*dentro(puesto, tope)
			r[i] *= float32(creible)
		}
		return

	default:
		return // Pasar o retirarse preflop no cambia lo que se cree.
	}

	for i := range r {
		if r[i] == 0 {
			continue
		}
		puesto := equity.PuestoDeCombo(i)
		creible := piso + (1-piso)*dentro(puesto, tope)
		if quitar > 0 {
			creible *= 1 - 0.55*dentro(puesto, quitar)
		}
		r[i] *= float32(creible)
	}
}

func leerPostflop(r *equity.Rango, j partida.Jugada, p *rivales.Perfil, cg float64, f *equity.FuerzaMesa, agresor bool, mezclaPago float64) {
	// Las manos que usan una carta de la mesa ya no son posibles.
	for i := range r {
		if !f.Posible[i] {
			r[i] = 0
		}
	}
	efectiva := func(i int) float64 { return float64(f.Efectivo[i]) }

	// conLoVisto mezcla el límite estimado con lo que el rival mostró en
	// showdowns haciendo lo mismo. Sin showdowns queda el estimado.
	conLoVisto := func(limite, umbral, peso float64) float64 {
		const pesoEstimado = 2.0
		return (limite*pesoEstimado + umbral*peso) / (pesoEstimado + peso)
	}

	// creible(fuerza efectiva, puesto) es la probabilidad de la acción con esa mano.
	var creible func(efectiva, puesto float64) float64

	switch j.Tipo {
	case partida.Bet:
		// Apuesta con la parte de arriba de lo que tiene. Cuánta parte depende
		// de qué tan seguido apuesta y del tamaño.
		tamano := j.FraccionPuesta()
		limite := corte(r, efectiva, 1-p.ParteQueApuesta(tamano, j.AllIn))
		umbral, peso := p.VistoAlApostar(tamano, j.AllIn)
		limite = conLoVisto(limite, umbral, peso)
		piso := p.Farol()
		if j.AllIn {
			piso *= 0.5
		}
		creible = func(efectiva, _ float64) float64 {
			return piso + (1-piso)*sigmoide((efectiva-limite)/anchoCorte)
		}

	case partida.Raise:
		tamano := j.FraccionPuesta()
		limite := corte(r, efectiva, 1-p.ParteQueSube(j.AllIn, j.Subidas >= 2))
		umbral, peso := p.VistoAlSubir(tamano, j.AllIn)
		limite = conLoVisto(limite, umbral, peso)
		creible = func(efectiva, _ float64) float64 {
			return 0.03 + 0.97*sigmoide((efectiva-limite)/anchoCorte)
		}

	case partida.Call:
		// Pagó: siguió con la parte de arriba de su rango. Su perfil dice qué
		// fracción de las veces se retira ante una apuesta así.
		fraccion, ciegas := j.Fraccion(), float64(j.PorIgualar)/cg
		relativo := corte(r, efectiva, p.ProbRetiro(fraccion, ciegas, agresor))
		absoluto := p.ProbRetiro(fraccion, ciegas, false)
		limite := mezclaPago*relativo + (1-mezclaPago)*absoluto
		lento := 0.3 // Con lo mejor suele subir en vez de pagar.
		if j.AllIn {
			lento = 0
		}
		creible = func(efectiva, puesto float64) float64 {
			return (0.04 + 0.96*sigmoide((efectiva-limite)/anchoCorte)) * (1 - lento*sigmoide((puesto-0.95)/0.02))
		}

	case partida.Check:
		if j.PorIgualar > 0 {
			return
		}
		// Pasó pudiendo apostar: es menos probable que tenga una mano con la
		// que suele apostar. Cuánto menos depende de qué tan seguido apuesta.
		limite := corte(r, efectiva, 1-p.ParteQueApuesta(0.6, false))
		descuento := acotar(1.5*p.FrecApuesta(), 0.25, 0.8)
		creible = func(efectiva, _ float64) float64 {
			return 1 - descuento*sigmoide((efectiva-limite)/anchoCorte)
		}

	default:
		return
	}

	for i := range r {
		if r[i] != 0 {
			r[i] *= float32(creible(float64(f.Efectivo[i]), float64(f.Puesto[i])))
		}
	}
}

// anchoCorte es qué tan borroso es el límite entre las manos con que un rival
// sigue y las que tira, en unidades de fuerza.
const anchoCorte = 0.05

// corte devuelve la fuerza por debajo de la cual queda la fracción `seVa` del
// peso del rango. Es la forma de decir "se retira con el X% peor de lo que
// tiene": el límite depende de qué tan fuerte es ya su rango, no de una escala
// fija.
func corte(r *equity.Rango, fuerza func(i int) float64, seVa float64) float64 {
	const cubos = 200
	var histograma [cubos + 1]float64
	total := 0.0
	for i, w := range r {
		if w == 0 {
			continue
		}
		histograma[int(acotar(fuerza(i), 0, 1)*cubos)] += float64(w)
		total += float64(w)
	}
	if total <= 0 {
		return seVa
	}
	objetivo, acumulado := seVa*total, 0.0
	for cubo, peso := range histograma {
		if peso > 0 && acumulado+peso >= objetivo {
			return (float64(cubo) + (objetivo-acumulado)/peso) / cubos
		}
		acumulado += peso
	}
	return 1
}

// respuesta estima cómo reacciona un rival a una apuesta nuestra que es
// `fraccion` del pozo y le cuesta `ciegas`: con qué probabilidad se retira y
// con qué rango sigue si no lo hace.
//
// Su perfil dice con qué frecuencia se retira ante ese tamaño. Hay dos formas
// de convertir eso en "qué manos tira", y `mezcla` elige entre ellas:
//
//   - Relativa (mezcla 1): tira lo peor de su rango actual. Contra quien ya
//     mostró fuerza, el rango con que paga es todavía más fuerte.
//   - Absoluta (mezcla 0): tira las manos por debajo de cierta fuerza, sin
//     importar su rango. Quien ya mostró fuerza casi no tiene manos así, y
//     entonces casi nunca se retira.
func respuesta(rv *rival, f *equity.FuerzaMesa, fraccion, ciegas float64, agresor bool, muertas uint64, mezcla float64) (float64, *equity.Rango) {
	posible := new(equity.Rango)
	for i, w := range rv.rango {
		if w > 0 && equity.MascaraDeCombo(i)&muertas == 0 && f.Posible[i] {
			posible[i] = w
		}
	}
	relativo := corte(posible, func(i int) float64 { return float64(f.Efectivo[i]) }, rv.perfil.ProbRetiro(fraccion, ciegas, agresor))
	absoluto := rv.perfil.ProbRetiro(fraccion, ciegas, false)
	limite := mezcla*relativo + (1-mezcla)*absoluto

	sigue := new(equity.Rango)
	var total, queda float64
	for i, w := range posible {
		if w == 0 {
			continue
		}
		paga := sigmoide((float64(f.Efectivo[i]) - limite) / anchoCorte)
		sigue[i] = w * float32(paga)
		total += float64(w)
		queda += float64(w) * paga
	}
	if total <= 0 {
		return absoluto, rv.rango
	}
	return 1 - queda/total, sigue
}
