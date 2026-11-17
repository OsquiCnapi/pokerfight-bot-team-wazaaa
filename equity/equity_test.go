package equity

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"
	"github.com/aicspucp/pokerfight-arena-participantes/pkg/poker"
)

func aSDK(c Carta) botsdk.Carta {
	return botsdk.Carta{Rango: botsdk.Rango(Valor(c) + 2), Palo: botsdk.Palo(Palo(c))}
}

// El evaluador rápido tiene que ordenar las manos exactamente igual que el del
// crupier: si no, el bot creería que gana manos que la mesa le da por perdidas.
func TestEvaluarCoincideConElCrupier(t *testing.T) {
	azar := rand.New(rand.NewPCG(1, 2))
	for _, cuantas := range []int{5, 6, 7} {
		for vuelta := 0; vuelta < 20000; vuelta++ {
			mazo := azar.Perm(52)
			a := make([]Carta, cuantas)
			b := make([]Carta, cuantas)
			sdkA := make([]botsdk.Carta, cuantas)
			sdkB := make([]botsdk.Carta, cuantas)
			// Las dos manos comparten las comunitarias, como en un showdown.
			for i := 0; i < cuantas; i++ {
				a[i] = Carta(mazo[i])
				b[i] = Carta(mazo[i])
			}
			b[0], b[1] = Carta(mazo[cuantas]), Carta(mazo[cuantas+1])
			for i := range a {
				sdkA[i], sdkB[i] = aSDK(a[i]), aSDK(b[i])
			}

			oficialA, _ := poker.MejorDe(sdkA)
			oficialB, _ := poker.MejorDe(sdkB)
			propioA, propioB := Evaluar(a), Evaluar(b)

			if Categoria(propioA) != int(oficialA.Categoria) {
				t.Fatalf("categoría distinta para %v: propia %d, oficial %d", sdkA, Categoria(propioA), oficialA.Categoria)
			}
			if signo(int64(propioA)-int64(propioB)) != signo(int64(oficialA.Puntaje)-int64(oficialB.Puntaje)) {
				t.Fatalf("orden distinto entre %v y %v", sdkA, sdkB)
			}
		}
	}
}

func signo(x int64) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

func carta(texto string) Carta {
	const rangos, palos = "23456789TJQKA", "cdhs"
	var r, p int
	for i := range rangos {
		if rangos[i] == texto[0] {
			r = i
		}
	}
	for i := range palos {
		if palos[i] == texto[1] {
			p = i
		}
	}
	return Carta(r<<2 | p)
}

func TestEquidadConocida(t *testing.T) {
	sim := NuevoSimulador(7)
	lejos := time.Now().Add(time.Minute)

	// AA contra una mano al azar ronda el 85%.
	eq, _ := sim.Equidad([2]Carta{carta("As"), carta("Ah")}, nil, []*Rango{nil}, 200000, lejos)
	if math.Abs(eq-0.852) > 0.01 {
		t.Errorf("AA contra una mano al azar: %.3f, se esperaba ~0.852", eq)
	}

	// AKs contra un rango que solo tiene QQ: ~46%.
	soloQQ := new(Rango)
	for i := 0; i < NumCombos; i++ {
		a, b := CartasDeCombo(i)
		if Valor(a) == 10 && Valor(b) == 10 {
			soloQQ[i] = 1
		}
	}
	eq, _ = sim.Equidad([2]Carta{carta("As"), carta("Ks")}, nil, []*Rango{soloQQ}, 200000, lejos)
	if math.Abs(eq-0.46) > 0.015 {
		t.Errorf("AKs contra QQ: %.3f, se esperaba ~0.46", eq)
	}

	// Con la mejor jugada posible en el river la equidad es 1.
	mesa := []Carta{carta("Ts"), carta("Js"), carta("Qs"), carta("2d"), carta("3c")}
	eq, _ = sim.Equidad([2]Carta{carta("As"), carta("Ks")}, mesa, []*Rango{nil, nil}, 5000, lejos)
	if eq != 1 {
		t.Errorf("escalera real: equidad %.3f, se esperaba 1", eq)
	}
}

func TestPuestosPreflop(t *testing.T) {
	if p := PuestoPreflop(carta("As"), carta("Ah")); p > 0.01 {
		t.Errorf("AA debería ser la mejor mano, puesto %.3f", p)
	}
	if p := PuestoPreflop(carta("7s"), carta("2h")); p < 0.9 {
		t.Errorf("72o debería estar entre las peores, puesto %.3f", p)
	}
	if PuestoPreflop(carta("As"), carta("Ks")) > PuestoPreflop(carta("9s"), carta("8s")) {
		t.Errorf("AKs debería ser mejor que 98s")
	}
}

func TestFuerzaMesa(t *testing.T) {
	mesa := []Carta{carta("Ks"), carta("7s"), carta("2d")}
	f := CalcularFuerza(mesa)

	trio := IndiceCombo(carta("Kh"), carta("Kd"))
	aire := IndiceCombo(carta("4c"), carta("3h"))
	proyecto := IndiceCombo(carta("As"), carta("5s"))

	if f.Puesto[trio] < 0.98 {
		t.Errorf("trío de reyes: puesto %.3f, se esperaba casi 1", f.Puesto[trio])
	}
	if f.Puesto[aire] > 0.2 {
		t.Errorf("43 sin nada: puesto %.3f, se esperaba bajo", f.Puesto[aire])
	}
	if f.Efectivo[proyecto] <= f.Puesto[proyecto] {
		t.Errorf("el proyecto de color debería subir la fuerza efectiva")
	}
	if f.Posible[IndiceCombo(carta("Ks"), carta("Ah"))] {
		t.Errorf("una mano con una carta de la mesa no es posible")
	}
}

func BenchmarkEvaluar7(b *testing.B) {
	azar := rand.New(rand.NewPCG(3, 4))
	manos := make([][7]Carta, 1024)
	for i := range manos {
		mazo := azar.Perm(52)
		for j := 0; j < 7; j++ {
			manos[i][j] = Carta(mazo[j])
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Evaluar(manos[i&1023][:])
	}
}

func BenchmarkEquidadFlop(b *testing.B) {
	sim := NuevoSimulador(9)
	mesa := []Carta{carta("Ks"), carta("7s"), carta("2d")}
	lejos := time.Now().Add(time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sim.Equidad([2]Carta{carta("Ah"), carta("Kd")}, mesa, []*Rango{RangoCompleto(), nil}, 10000, lejos)
	}
}
