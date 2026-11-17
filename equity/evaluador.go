package equity

import "math/bits"

// Categorías de jugada, de menor a mayor. Coinciden con las de pkg/poker.
const (
	CartaAlta = iota
	Par
	DoblePar
	Trio
	Escalera
	Color
	FullHouse
	Poker
	EscaleraDeColor
)

var (
	// escaleraAlta[m] es el rango más alto de la mejor escalera que se arma con
	// los rangos de la máscara m, o -1 si no hay ninguna. La rueda (A-2-3-4-5)
	// tiene como carta alta el cinco (rango 3).
	escaleraAlta [1 << 13]int8
	// cincoMasAltas[m] es la máscara m recortada a sus cinco bits más altos.
	cincoMasAltas [1 << 13]uint16
)

func prepararEvaluador() {
	const rueda = 1<<12 | 0b1111
	for m := 0; m < 1<<13; m++ {
		escaleraAlta[m] = -1
		for alta := 12; alta >= 4; alta-- {
			if cinco := 0b11111 << (alta - 4); m&cinco == cinco {
				escaleraAlta[m] = int8(alta)
				break
			}
		}
		if escaleraAlta[m] < 0 && m&rueda == rueda {
			escaleraAlta[m] = 3
		}
		cincoMasAltas[m] = masAltas(uint16(m), 5)
	}
}

// masAltas deja en la máscara solo sus n bits más altos.
func masAltas(m uint16, n int) uint16 {
	for bits.OnesCount16(m) > n {
		m &= m - 1
	}
	return m
}

func rangoMasAlto(m uint16) int { return bits.Len16(m) - 1 }

func puntaje(categoria int, a, b uint16) uint32 {
	return uint32(categoria)<<26 | uint32(a)<<13 | uint32(b)
}

// Categoria extrae el tipo de jugada de un puntaje de Evaluar.
func Categoria(p uint32) int { return int(p >> 26) }

// Evaluar puntúa la mejor jugada de cinco cartas entre las que recibe (de 5 a
// 7). Un puntaje mayor es una mano mejor; el mismo puntaje es un empate
// exacto. El número solo sirve para comparar.
func Evaluar(cartas []Carta) uint32 {
	var (
		porPalo      [4]uint16
		cuantasPalo  [4]uint8
		cuantasRango [13]uint8
		// m1..m4: rangos que aparecen al menos 1, 2, 3 y 4 veces.
		m1, m2, m3, m4 uint16
	)
	for _, c := range cartas {
		r, p := c>>2, c&3
		bit := uint16(1) << r
		porPalo[p] |= bit
		cuantasPalo[p]++
		cuantasRango[r]++
		switch cuantasRango[r] {
		case 1:
			m1 |= bit
		case 2:
			m2 |= bit
		case 3:
			m3 |= bit
		case 4:
			m4 |= bit
		}
	}

	// Con siete cartas, un color no puede convivir con un full o un póker
	// (hacen falta cinco rangos distintos del mismo palo), así que mirarlo
	// primero no esconde una jugada mayor.
	for p := 0; p < 4; p++ {
		if cuantasPalo[p] >= 5 {
			if alta := escaleraAlta[porPalo[p]]; alta >= 0 {
				return puntaje(EscaleraDeColor, uint16(alta), 0)
			}
			return puntaje(Color, 0, cincoMasAltas[porPalo[p]])
		}
	}

	if m4 != 0 {
		r := rangoMasAlto(m4)
		return puntaje(Poker, uint16(r), uint16(rangoMasAlto(m1&^(1<<r))))
	}

	if m3 != 0 {
		trio := rangoMasAlto(m3)
		if resto := m2 &^ (1 << trio); resto != 0 {
			return puntaje(FullHouse, uint16(trio), uint16(rangoMasAlto(resto)))
		}
	}

	if alta := escaleraAlta[m1]; alta >= 0 {
		return puntaje(Escalera, uint16(alta), 0)
	}

	if m3 != 0 {
		trio := rangoMasAlto(m3)
		return puntaje(Trio, uint16(trio), masAltas(m1&^(1<<trio), 2))
	}

	if m2 != 0 {
		alto := rangoMasAlto(m2)
		if resto := m2 &^ (1 << alto); resto != 0 {
			bajo := rangoMasAlto(resto)
			pares := uint16(1)<<alto | uint16(1)<<bajo
			return puntaje(DoblePar, pares, uint16(1)<<rangoMasAlto(m1&^pares))
		}
		return puntaje(Par, uint16(alto), masAltas(m1&^(1<<alto), 3))
	}

	return puntaje(CartaAlta, 0, cincoMasAltas[m1])
}
