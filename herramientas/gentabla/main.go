// Comando gentabla: calcula la equidad preflop de las 169 manos iniciales
// contra una mano al azar y escribe equity/tabla_preflop.go.
//
// Se corre una sola vez (o si cambia el evaluador), desde la raíz del repo:
//
//	go run ./herramientas/gentabla
package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
)

const simulacionesPorClase = 400_000

func main() {
	// Una mano representante por clase.
	representante := make(map[int][2]equity.Carta)
	for i := 0; i < equity.NumCombos; i++ {
		a, b := equity.CartasDeCombo(i)
		clase := equity.Clase(a, b)
		if _, ok := representante[clase]; !ok {
			representante[clase] = [2]equity.Carta{a, b}
		}
	}

	var tabla [equity.NumClases]float64
	var grupo sync.WaitGroup
	lejos := time.Now().Add(time.Hour)
	for clase, mano := range representante {
		grupo.Add(1)
		go func(clase int, mano [2]equity.Carta) {
			defer grupo.Done()
			sim := equity.NuevoSimulador(uint64(clase) + 1)
			tabla[clase], _ = sim.Equidad(mano, nil, []*equity.Rango{nil}, simulacionesPorClase, lejos)
		}(clase, mano)
	}
	grupo.Wait()

	var b strings.Builder
	b.WriteString("package equity\n\n")
	b.WriteString("// Código generado por herramientas/gentabla. NO EDITAR.\n//\n")
	b.WriteString("// equidadVsAzar[clase] es la equidad preflop de cada mano inicial contra una\n")
	fmt.Fprintf(&b, "// mano al azar, estimada con %d simulaciones por clase.\n", simulacionesPorClase)
	b.WriteString("var equidadVsAzar = [NumClases]float32{\n")
	for clase := 0; clase < equity.NumClases; clase++ {
		fmt.Fprintf(&b, "\t%.4f, // %s\n", tabla[clase], equity.NombreClase(clase))
	}
	b.WriteString("}\n")

	if err := os.WriteFile("equity/tabla_preflop.go", []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("equity/tabla_preflop.go actualizado")
}
