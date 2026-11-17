package equity

import "sort"

// FuerzaMesa dice, para una mesa concreta (flop, turn o river), qué tan fuerte
// es cada una de las 1326 manos iniciales. Es la base para leer rangos: cuando
// un rival apuesta, se le creen más las manos con Puesto alto.
type FuerzaMesa struct {
	// Puesto es la fracción de manos posibles que quedan por debajo: 0 es la
	// peor mano sobre esta mesa y 1 la mejor. Cuenta solo la jugada ya armada.
	Puesto [NumCombos]float32
	// Efectivo es Puesto corregido por proyectos: antes del river, una mano
	// con un proyecto fuerte se juega (se apuesta, se paga) como una mano
	// media o buena aunque todavía no tenga nada.
	Efectivo [NumCombos]float32
	// Posible es false para las manos que usan una carta de la mesa.
	Posible [NumCombos]bool
}

// CalcularFuerza arma la tabla de fuerzas de una mesa de 3 a 5 cartas.
func CalcularFuerza(mesa []Carta) *FuerzaMesa {
	f := new(FuerzaMesa)
	if len(mesa) < 3 || len(mesa) > 5 {
		return f
	}

	var enMesa uint64
	var rangosMesa uint16
	var paloMesa [4]int
	for _, c := range mesa {
		enMesa |= Mascara(c)
		rangosMesa |= 1 << (c >> 2)
		paloMesa[c&3]++
	}

	var cartas [7]Carta
	copy(cartas[2:], mesa)
	n := 2 + len(mesa)

	valores := make([]uint32, NumCombos)
	orden := make([]int, 0, NumCombos)
	for i := 0; i < NumCombos; i++ {
		if mascaraCombo[i]&enMesa != 0 {
			continue
		}
		f.Posible[i] = true
		cartas[0], cartas[1] = comboA[i], comboB[i]
		valores[i] = Evaluar(cartas[:n])
		orden = append(orden, i)
	}
	sort.Slice(orden, func(a, b int) bool { return valores[orden[a]] < valores[orden[b]] })

	total := float32(len(orden))
	for inicio := 0; inicio < len(orden); {
		fin := inicio
		for fin < len(orden) && valores[orden[fin]] == valores[orden[inicio]] {
			fin++
		}
		// Las manos empatadas comparten el punto medio de su franja.
		puesto := (float32(inicio) + float32(fin-inicio)/2) / total
		for _, i := range orden[inicio:fin] {
			f.Puesto[i] = puesto
			f.Efectivo[i] = puesto
		}
		inicio = fin
	}

	if len(mesa) == 5 {
		return f
	}

	// Proyectos. Cada "out" es una carta que completa color o escalera. En el
	// flop quedan dos cartas por salir y un proyecto vale más que en el turn.
	base, porOut := float32(0.42), float32(0.027)
	if len(mesa) == 4 {
		base, porOut = 0.34, 0.02
	}
	for _, i := range orden {
		a, b := comboA[i], comboB[i]
		outs := 0

		// Color: cuatro cartas del mismo palo usando al menos una propia.
		for _, c := range [2]Carta{a, b} {
			cuantas := paloMesa[c&3] + 1
			if a&3 == b&3 {
				cuantas = paloMesa[c&3] + 2
			}
			if cuantas == 4 {
				outs = 9
				break
			}
		}

		// Escalera: rangos que, si salen, arman una escalera que hoy no está.
		rangos := rangosMesa | 1<<(a>>2) | 1<<(b>>2)
		if escaleraAlta[rangos] < 0 {
			completan := 0
			for r := 0; r < 13; r++ {
				if rangos&(1<<r) == 0 && escaleraAlta[rangos|1<<r] >= 0 {
					completan++
				}
			}
			// Si la mesa sola ya tiene el mismo proyecto, no es mérito de la
			// mano: lo tienen todos.
			propias := completan
			if escaleraAlta[rangosMesa] < 0 {
				for r := 0; r < 13; r++ {
					if rangosMesa&(1<<r) == 0 && escaleraAlta[rangosMesa|1<<r] >= 0 {
						propias--
					}
				}
			}
			if propias > 0 {
				outs += 4 * min(completan, 2)
			}
		}

		if outs > 0 {
			if conProyecto := base + porOut*float32(min(outs, 15)); conProyecto > f.Efectivo[i] {
				f.Efectivo[i] = conProyecto
			}
		}
	}
	return f
}
