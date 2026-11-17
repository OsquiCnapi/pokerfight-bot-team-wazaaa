package equity

import (
	"math/rand/v2"
	"time"
)

// Rango es el peso de cada una de las 1326 manos iniciales en lo que se cree
// que puede tener un rival. Un peso de cero es una mano descartada; los pesos
// no hace falta que sumen 1.
type Rango [NumCombos]float32

// RangoCompleto devuelve el rango de un rival del que no se sabe nada: todas
// las manos con el mismo peso.
func RangoCompleto() *Rango {
	r := new(Rango)
	for i := range r {
		r[i] = 1
	}
	return r
}

// Peso es la suma de los pesos de las manos que no usan ninguna carta muerta.
func (r *Rango) Peso(muertas uint64) float64 {
	total := 0.0
	for i, w := range r {
		if w > 0 && mascaraCombo[i]&muertas == 0 {
			total += float64(w)
		}
	}
	return total
}

// Simulador estima equidades por Monte Carlo. No es seguro para uso
// concurrente: cada goroutine necesita el suyo.
type Simulador struct {
	azar *rand.Rand
}

// NuevoSimulador crea un simulador con su propia fuente de azar.
func NuevoSimulador(semilla uint64) *Simulador {
	return &Simulador{azar: rand.New(rand.NewPCG(semilla, semilla^0x9E3779B97F4A7C15))}
}

// cadaCuantasMirarReloj es cada cuántas simulaciones se revisa el plazo.
const cadaCuantasMirarReloj = 512

// intentosPorRival es cuántas veces se reintenta sortear la mano de un rival
// cuando la sorteada choca con una carta ya usada.
const intentosPorRival = 12

type rivalSimulado struct {
	acumulado []float32 // Suma acumulada de pesos; nil si juega una mano al azar.
	total     float32
}

// Equidad estima qué parte del pozo gana `mano` en el showdown contra los
// rivales dados, repartiendo al azar las comunitarias que faltan. Cada rival
// recibe una mano sorteada según su rango (nil es una mano cualquiera). Un
// empate entre k jugadores cuenta 1/k.
//
// Corta al llegar a maxSims o al pasar `limite`, lo que ocurra primero, pero
// siempre juega al menos un bloque de simulaciones. Devuelve la equidad y
// cuántas simulaciones la respaldan.
func (s *Simulador) Equidad(mano [2]Carta, mesa []Carta, rivales []*Rango, maxSims int, limite time.Time) (float64, int) {
	if len(rivales) == 0 {
		return 1, 0
	}

	muertas := Mascara(mano[0]) | Mascara(mano[1])
	for _, c := range mesa {
		muertas |= Mascara(c)
	}

	preparados := make([]rivalSimulado, len(rivales))
	for i, rango := range rivales {
		if rango == nil {
			continue
		}
		acumulado := make([]float32, NumCombos)
		var suma float32
		for j, w := range rango {
			if w > 0 && mascaraCombo[j]&muertas == 0 {
				suma += w
			}
			acumulado[j] = suma
		}
		if suma > 0 {
			preparados[i] = rivalSimulado{acumulado: acumulado, total: suma}
		}
	}

	var cartas [7]Carta
	cartas[0], cartas[1] = mano[0], mano[1]
	copy(cartas[2:], mesa)
	faltan := 5 - len(mesa)

	var rival [7]Carta
	var ganado float64
	jugadas := 0

	for jugadas < maxSims {
		if jugadas > 0 && jugadas%cadaCuantasMirarReloj == 0 && time.Now().After(limite) {
			break
		}
		usadas := muertas

		// Primero las manos de los rivales: son las que tienen restricciones.
		var manos [8][2]Carta
		for i := range preparados {
			a, b, ok := s.sortearMano(&preparados[i], usadas)
			if !ok {
				a, b = s.sortearAlAzar(usadas)
			}
			manos[i] = [2]Carta{a, b}
			usadas |= Mascara(a) | Mascara(b)
		}

		for i := 0; i < faltan; i++ {
			c := s.sortearCarta(usadas)
			cartas[2+len(mesa)+i] = c
			usadas |= Mascara(c)
		}

		propio := Evaluar(cartas[:])
		copy(rival[2:], cartas[2:])
		empatados := 0
		perdio := false
		for i := range preparados {
			rival[0], rival[1] = manos[i][0], manos[i][1]
			valor := Evaluar(rival[:])
			if valor > propio {
				perdio = true
				break
			}
			if valor == propio {
				empatados++
			}
		}
		if !perdio {
			ganado += 1 / float64(empatados+1)
		}
		jugadas++
	}

	return ganado / float64(jugadas), jugadas
}

func (s *Simulador) sortearCarta(usadas uint64) Carta {
	for {
		c := Carta(s.azar.IntN(52))
		if usadas&Mascara(c) == 0 {
			return c
		}
	}
}

func (s *Simulador) sortearAlAzar(usadas uint64) (Carta, Carta) {
	a := s.sortearCarta(usadas)
	b := s.sortearCarta(usadas | Mascara(a))
	return a, b
}

// sortearMano elige una mano según los pesos del rival, evitando las cartas ya
// usadas. Devuelve false si el rival no tiene rango o si no encontró una mano
// libre en unos pocos intentos.
func (s *Simulador) sortearMano(r *rivalSimulado, usadas uint64) (Carta, Carta, bool) {
	if r.acumulado == nil {
		return 0, 0, false
	}
	for intento := 0; intento < intentosPorRival; intento++ {
		objetivo := s.azar.Float32() * r.total
		// Búsqueda binaria del primer índice cuyo acumulado supera el objetivo.
		bajo, alto := 0, NumCombos-1
		for bajo < alto {
			medio := (bajo + alto) / 2
			if r.acumulado[medio] > objetivo {
				alto = medio
			} else {
				bajo = medio + 1
			}
		}
		if mascaraCombo[bajo]&usadas == 0 {
			return comboA[bajo], comboB[bajo], true
		}
	}
	return 0, 0, false
}
