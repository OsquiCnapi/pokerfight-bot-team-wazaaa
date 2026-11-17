// Package equity es la parte numérica del bot: representa las cartas como
// enteros, evalúa manos de 5 a 7 cartas y estima por Monte Carlo qué parte del
// pozo gana una mano contra los rangos de los rivales.
//
// El evaluador de pkg/poker (el del crupier) es correcto pero lento para
// simular: ordena y reserva memoria en cada llamada. Este hace lo mismo con
// máscaras de bits y es unas cien veces más rápido. Las pruebas comprueban que
// los dos ordenan las manos exactamente igual.
package equity

import "github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

// Carta es una carta como entero de 0 a 51. El rango es c>>2 (0 es el dos,
// 12 es el as) y el palo es c&3.
type Carta = uint8

// NumCombos es la cantidad de manos iniciales distintas: C(52,2).
const NumCombos = 1326

// NumClases es la cantidad de manos iniciales sin distinguir palos concretos:
// 13 pares, 78 del mismo palo y 78 de distinto palo.
const NumClases = 169

// DeSDK convierte una carta del protocolo al entero interno.
func DeSDK(c botsdk.Carta) Carta { return Carta((int(c.Rango)-2)<<2 | int(c.Palo)) }

// Valida indica si una carta del protocolo es una carta de verdad. El valor
// cero (lo que queda si no llegó el mano_inicio) no lo es.
func Valida(c botsdk.Carta) bool {
	return c.Rango >= botsdk.Dos && c.Rango <= botsdk.As && c.Palo <= botsdk.Picas
}

// Valor devuelve el rango de la carta, de 0 (dos) a 12 (as).
func Valor(c Carta) int { return int(c >> 2) }

// Palo devuelve el palo de 0 a 3.
func Palo(c Carta) int { return int(c & 3) }

// Mascara devuelve el bit de la carta dentro de un uint64 de cartas usadas.
func Mascara(c Carta) uint64 { return 1 << c }

var (
	comboA, comboB [NumCombos]Carta
	mascaraCombo   [NumCombos]uint64
	claseCombo     [NumCombos]uint8
)

func init() {
	for b := 1; b < 52; b++ {
		for a := 0; a < b; a++ {
			i := b*(b-1)/2 + a
			comboA[i], comboB[i] = Carta(a), Carta(b)
			mascaraCombo[i] = 1<<uint(a) | 1<<uint(b)
			claseCombo[i] = uint8(Clase(Carta(a), Carta(b)))
		}
	}
	prepararEvaluador()
	prepararPreflop()
}

// IndiceCombo devuelve la posición de una mano inicial en los arreglos de 1326
// elementos. No depende del orden de las dos cartas.
func IndiceCombo(a, b Carta) int {
	if a > b {
		a, b = b, a
	}
	return int(b)*(int(b)-1)/2 + int(a)
}

// CartasDeCombo es la inversa de IndiceCombo.
func CartasDeCombo(i int) (Carta, Carta) { return comboA[i], comboB[i] }

// MascaraDeCombo devuelve los dos bits de la mano i.
func MascaraDeCombo(i int) uint64 { return mascaraCombo[i] }

// Clase devuelve un número de 0 a 168 que identifica la mano inicial sin
// distinguir palos: AKs, AKo y los pares son clases distintas.
func Clase(a, b Carta) int {
	alta, baja := Valor(a), Valor(b)
	if alta < baja {
		alta, baja = baja, alta
	}
	switch {
	case alta == baja:
		return alta*13 + alta
	case Palo(a) == Palo(b):
		return alta*13 + baja
	default:
		return baja*13 + alta
	}
}

// NombreClase devuelve la notación habitual de una clase: "AKs", "77", "T9o".
func NombreClase(clase int) string {
	const letras = "23456789TJQKA"
	fila, columna := clase/13, clase%13
	switch {
	case fila == columna:
		return string([]byte{letras[fila], letras[fila]})
	case fila > columna:
		return string([]byte{letras[fila], letras[columna], 's'})
	default:
		return string([]byte{letras[columna], letras[fila], 'o'})
	}
}
