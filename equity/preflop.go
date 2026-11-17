package equity

import "sort"

var (
	// puestoClase[c] es qué fracción de las 1326 manos iniciales es mejor que
	// la clase c: 0 para AA y cerca de 1 para la peor. Es la escala con la
	// que se arman todos los rangos preflop ("el 15% mejor").
	puestoClase [NumClases]float32
	// combosClase[c] es cuántas de las 1326 manos pertenecen a la clase c.
	combosClase [NumClases]int
)

// valorPreflop ordena las manos iniciales. Parte de la equidad contra una mano
// al azar (tabla_preflop.go) y le suma un poco a las manos que se juegan mejor
// de lo que ese número dice: las del mismo palo y las conectadas ganan pozos
// grandes cuando ligan, cosa que la equidad contra una mano al azar no ve.
func valorPreflop(clase int) float64 {
	fila, columna := clase/13, clase%13
	valor := float64(equidadVsAzar[clase])
	if fila == columna {
		return valor
	}
	alta, baja := fila, columna
	if alta < baja {
		alta, baja = baja, alta
		// Distinto palo: sin bono.
	} else {
		valor += 0.02
	}
	switch alta - baja {
	case 1:
		valor += 0.012
	case 2:
		valor += 0.006
	}
	return valor
}

func prepararPreflop() {
	for i := 0; i < NumCombos; i++ {
		combosClase[claseCombo[i]]++
	}
	orden := make([]int, NumClases)
	for i := range orden {
		orden[i] = i
	}
	sort.SliceStable(orden, func(a, b int) bool { return valorPreflop(orden[a]) > valorPreflop(orden[b]) })

	mejores := 0
	for _, clase := range orden {
		// El puesto de una clase es el punto medio de la franja que ocupa: así
		// "el 5% mejor" incluye a las clases cuya franja cae mayormente dentro.
		puestoClase[clase] = (float32(mejores) + float32(combosClase[clase])/2) / NumCombos
		mejores += combosClase[clase]
	}
}

// PuestoPreflop devuelve qué fracción de las manos iniciales es mejor que la
// dada: 0 es AA, 1 es la peor.
func PuestoPreflop(a, b Carta) float64 { return float64(puestoClase[Clase(a, b)]) }

// PuestoDeCombo es PuestoPreflop para un índice de combo.
func PuestoDeCombo(i int) float64 { return float64(puestoClase[claseCombo[i]]) }

// EquidadVsAzar devuelve la equidad preflop de la mano contra una sola mano al
// azar, leída de la tabla precalculada.
func EquidadVsAzar(a, b Carta) float64 { return float64(equidadVsAzar[Clase(a, b)]) }
