package estrategia

// Parametros junta los números de la estrategia que se eligieron midiendo:
// cada uno se probó jugando miles de mesas contra los bots del kit
// (herramientas/medir) y se quedó el valor que más fichas dejó sin subir la
// frecuencia de quiebra.
//
// Están en un solo lugar para poder cambiarlos sin recompilar durante las
// pruebas (flag -ajustes de main.go, en JSON).
type Parametros struct {
	// ReservaCiegas regula la aversión al riesgo (ver situacion.valor). Con
	// menos reserva el bot evita más las apuestas que pueden eliminarlo.
	ReservaCiegas float64 `json:"reserva"`
	// ReservaFinal es la reserva en la mesa final. Ahí solo cuenta ganar, así
	// que conviene arriesgar más que en la clasificatoria, donde lo primero es
	// no quedar eliminado.
	ReservaFinal float64 `json:"reserva_final"`

	// MezclaRetiro decide cómo se estima con qué manos un rival tira o paga
	// una apuesta nuestra: 1 = "tira lo peor de lo que tiene" (relativo a su
	// rango), 0 = "tira las manos por debajo de cierta fuerza" (absoluto).
	MezclaRetiro float64 `json:"mezcla_retiro"`
	// MezclaPago es lo mismo al leer el rango de un rival que pagó.
	MezclaPago float64 `json:"mezcla_pago"`

	// SubirSiPagan es la equidad mínima contra el rango que paga para subir
	// por valor; SubirSiSeRetiran, la probabilidad mínima de que se retiren
	// para subir de farol.
	SubirSiPagan     float64 `json:"subir_si_pagan"`
	SubirSiSeRetiran float64 `json:"subir_si_se_retiran"`

	// ExigeFarol es cuánto mejor que pasar (en fracción del pozo) tiene que
	// salir un farol puro para hacerlo; ExigeApuesta, lo mismo para el resto
	// de las apuestas.
	ExigeFarol   float64 `json:"exige_farol"`
	ExigeApuesta float64 `json:"exige_apuesta"`

	// MargenPagar es la equidad extra, sobre el precio, que se pide para
	// pagar antes del river; MargenPagarRiver, en el river.
	MargenPagar      float64 `json:"margen_pagar"`
	MargenPagarRiver float64 `json:"margen_pagar_river"`

	// Abrir escala los rangos con que se abre preflop (1 = los de la tabla).
	Abrir float64 `json:"abrir"`
}

// PorDefecto devuelve los parámetros con los que juega el bot.
func PorDefecto() Parametros {
	return Parametros{
		ReservaCiegas:    20,
		ReservaFinal:     60,
		MezclaRetiro:     0,
		MezclaPago:       0,
		SubirSiPagan:     0.72,
		SubirSiSeRetiran: 0.55,
		ExigeFarol:       0.06,
		ExigeApuesta:     0.02,
		MargenPagar:      0.035,
		MargenPagarRiver: 0.015,
		Abrir:            1,
	}
}
