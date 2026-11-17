// Comando medir: juega muchas mesas locales con las condiciones de la
// clasificatoria y resume cómo le fue a nuestro bot contra los bots del kit.
//
// Una sola mesa de 50 manos no dice nada: el resultado depende de las cartas.
// Este comando juega decenas de mesas en paralelo, sortea los rivales de cada
// una (como el sorteo del torneo) y promedia.
//
// Uso, desde la raíz del repo y con el kit ya compilado:
//
//	go build -o bin/ .
//	go run ./herramientas/medir -mesas 60
//
// No modifica nada del kit: solo ejecuta sus binarios.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	kit       = flag.String("kit", "../pokerfight-arena-participantes", "carpeta del kit oficial, con bin/ ya compilado")
	nuestro   = flag.String("bot", "bin/pokerfight-bot-team-wazaaa", "ejecutable de nuestro bot")
	mesas     = flag.Int("mesas", 40, "cantidad de mesas a jugar")
	paralelo  = flag.Int("paralelo", 6, "mesas simultáneas")
	porMesa   = flag.Int("jugadores", 5, "jugadores por mesa, contando al nuestro")
	manos     = flag.Int("manos", 50, "manos por mesa")
	stack     = flag.Int("stack", 10000, "fichas iniciales")
	ciega     = flag.Int("ciega-grande", 100, "ciega grande (la chica es la mitad)")
	plazo     = flag.Int("timeout-ms", 5000, "plazo por jugada")
	lista     = flag.String("rivales", "aleatorio,conservador,base,estadistico,bandido,espia,darwin", "bots del kit entre los que se sortean los rivales")
	puerto    = flag.Int("puerto", 9300, "primer puerto a usar")
	registros = flag.String("registros", "", "si se indica, carpeta donde guardar el detalle de cada decisión de nuestro bot")
	ajustes   = flag.String("ajustes", "", "valor del flag -ajustes para nuestro bot (JSON)")
	otro      = flag.String("otro", "", "ejecutable de otra versión de nuestro bot para sentar en todas las mesas (comparación directa)")
)

// argumentos extra de cada bot del kit. La mesa agrega -addr y -token.
func argumentosDe(bot string, mesa int) []string {
	switch bot {
	case "estadistico":
		return []string{"-simulaciones", "2000", "-semilla", strconv.Itoa(mesa + 1)}
	case "aleatorio":
		return []string{"-semilla", strconv.Itoa(mesa + 1)}
	case "darwin":
		return []string{"-silencioso", "-semilla", strconv.Itoa(mesa + 1)}
	case "bandido", "espia":
		return []string{"-silencioso"}
	}
	return nil
}

type resultado struct {
	fichas map[string]int // Por nombre de bot.
	puesto map[string]int
	err    error
}

func ejecutable(ruta string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(ruta, ".exe") {
		ruta += ".exe"
	}
	absoluta, err := filepath.Abs(ruta)
	if err != nil {
		return ruta
	}
	return absoluta
}

func jugarMesa(numero int, rivales []string) resultado {
	direccion := fmt.Sprintf("127.0.0.1:%d", *puerto+numero)
	bin := filepath.Join(*kit, "bin")

	mesa := exec.Command(ejecutable(filepath.Join(bin, "mesa")),
		"-addr", direccion,
		"-jugadores", strconv.Itoa(len(rivales)+1), "-min-jugadores", strconv.Itoa(len(rivales)+1),
		"-rondas", strconv.Itoa(*manos), "-stack", strconv.Itoa(*stack),
		"-ciega-chica", strconv.Itoa(*ciega/2), "-ciega-grande", strconv.Itoa(*ciega),
		"-timeout-ms", strconv.Itoa(*plazo))
	salida, err := mesa.StdoutPipe()
	if err != nil {
		return resultado{err: err}
	}
	if err := mesa.Start(); err != nil {
		return resultado{err: err}
	}

	// Esperar a que la mesa escuche.
	for intento := 0; intento < 100; intento++ {
		if conexion, err := net.DialTimeout("tcp", direccion, 100*time.Millisecond); err == nil {
			conexion.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Todos los bots, en orden al azar para que las sillas cambien.
	type lanzamiento struct {
		nombre string
		cmd    *exec.Cmd
	}
	var bots []lanzamiento
	for i, rival := range rivales {
		nombre := fmt.Sprintf("%s#%d", rival, i)
		args := append(argumentosDe(rival, numero), "-addr", direccion, "-token", nombre)
		if rival == "otro" {
			args = []string{"-addr", direccion, "-token", nombre, "-reconectar", "0"}
			bots = append(bots, lanzamiento{nombre, exec.Command(ejecutable(*otro), args...)})
			continue
		}
		bots = append(bots, lanzamiento{nombre, exec.Command(ejecutable(filepath.Join(bin, rival)), args...)})
	}
	propio := exec.Command(ejecutable(*nuestro), "-addr", direccion, "-token", "wazaaa", "-reconectar", "0")
	if *ajustes != "" {
		propio.Args = append(propio.Args, "-ajustes", *ajustes)
	}
	if *registros != "" {
		propio.Args = append(propio.Args, "-detalle")
		if archivo, err := os.Create(filepath.Join(*registros, fmt.Sprintf("mesa-%03d.log", numero))); err == nil {
			defer archivo.Close()
			fmt.Fprintf(archivo, "rivales: %v\n", rivales)
			propio.Stderr = archivo
		}
	}
	bots = append(bots, lanzamiento{"wazaaa", propio})
	rand.Shuffle(len(bots), func(i, j int) { bots[i], bots[j] = bots[j], bots[i] })

	for _, b := range bots {
		if err := b.cmd.Start(); err != nil {
			mesa.Process.Kill()
			return resultado{err: fmt.Errorf("no se pudo lanzar %s: %w", b.nombre, err)}
		}
	}
	defer func() {
		for _, b := range bots {
			b.cmd.Process.Kill()
			b.cmd.Wait()
		}
	}()

	// Si algo se cuelga, no esperar para siempre.
	vigilante := time.AfterFunc(15*time.Minute, func() { mesa.Process.Kill() })
	defer vigilante.Stop()

	r := resultado{fichas: map[string]int{}, puesto: map[string]int{}}
	lector := bufio.NewScanner(salida)
	for lector.Scan() {
		// Línea esperada: "  #1 nombre   12345 fichas"
		campos := strings.Fields(lector.Text())
		if len(campos) == 4 && strings.HasPrefix(campos[0], "#") && campos[3] == "fichas" {
			puesto, _ := strconv.Atoi(campos[0][1:])
			fichas, _ := strconv.Atoi(campos[2])
			r.fichas[campos[1]] = fichas
			r.puesto[campos[1]] = puesto
		}
	}
	mesa.Wait()
	if _, ok := r.fichas["wazaaa"]; !ok {
		r.err = fmt.Errorf("la mesa %d terminó sin resultado para nuestro bot", numero)
	}
	return r
}

type acumulado struct {
	valores []float64
	quiebra int
	puestos float64
}

func (a *acumulado) sumar(fichas, puesto int) {
	a.valores = append(a.valores, float64(fichas))
	a.puestos += float64(puesto)
	if fichas == 0 {
		a.quiebra++
	}
}

func (a *acumulado) media() float64 {
	total := 0.0
	for _, v := range a.valores {
		total += v
	}
	return total / float64(len(a.valores))
}

// errorTipico es el error estándar de la media: cuánto puede moverse el
// promedio por pura suerte.
func (a *acumulado) errorTipico() float64 {
	n := float64(len(a.valores))
	if n < 2 {
		return 0
	}
	media, suma := a.media(), 0.0
	for _, v := range a.valores {
		suma += (v - media) * (v - media)
	}
	return math.Sqrt(suma/(n-1)) / math.Sqrt(n)
}

func main() {
	flag.Parse()
	disponibles := strings.Split(*lista, ",")
	if *registros != "" {
		os.MkdirAll(*registros, 0o755)
	}

	type trabajo struct {
		numero  int
		rivales []string
	}
	cola := make(chan trabajo)
	var candado sync.Mutex
	porBot := map[string]*acumulado{}
	// contra[rival] junta nuestros resultados en las mesas donde estuvo ese rival.
	contra := map[string]*acumulado{}
	fallos := 0

	var grupo sync.WaitGroup
	for i := 0; i < *paralelo; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			for t := range cola {
				r := jugarMesa(t.numero, t.rivales)
				candado.Lock()
				if r.err != nil {
					fallos++
					fmt.Fprintln(os.Stderr, "medir:", r.err)
				} else {
					for nombre, fichas := range r.fichas {
						tipo := strings.SplitN(nombre, "#", 2)[0]
						if porBot[tipo] == nil {
							porBot[tipo] = &acumulado{}
						}
						porBot[tipo].sumar(fichas, r.puesto[nombre])
					}
					for _, rival := range t.rivales {
						if contra[rival] == nil {
							contra[rival] = &acumulado{}
						}
						contra[rival].sumar(r.fichas["wazaaa"], r.puesto["wazaaa"])
					}
					fmt.Fprintf(os.Stderr, "mesa %3d %v -> wazaaa %d fichas (puesto %d)\n", t.numero, t.rivales, r.fichas["wazaaa"], r.puesto["wazaaa"])
				}
				candado.Unlock()
			}
		}()
	}

	for numero := 0; numero < *mesas; numero++ {
		// Sorteo de rivales sin repetir, como el sorteo de mesas del torneo.
		orden := rand.Perm(len(disponibles))
		var rivales []string
		if *otro != "" {
			rivales = append(rivales, "otro")
		}
		for _, i := range orden {
			if len(rivales) >= *porMesa-1 {
				break
			}
			rivales = append(rivales, disponibles[i])
		}
		cola <- trabajo{numero, rivales}
	}
	close(cola)
	grupo.Wait()

	nombres := make([]string, 0, len(porBot))
	for nombre := range porBot {
		nombres = append(nombres, nombre)
	}
	sort.Slice(nombres, func(a, b int) bool { return porBot[nombres[a]].media() > porBot[nombres[b]].media() })

	fmt.Printf("\n%d mesas de %d jugadores, %d manos, stack %d, ciegas %d/%d (%d fallidas)\n\n",
		*mesas, *porMesa, *manos, *stack, *ciega/2, *ciega, fallos)
	fmt.Printf("%-13s %6s %14s %9s %8s %8s\n", "bot", "mesas", "fichas (media)", "± error", "quiebra", "puesto")
	for _, nombre := range nombres {
		a := porBot[nombre]
		n := float64(len(a.valores))
		fmt.Printf("%-13s %6d %14.0f %9.0f %7.0f%% %8.2f\n", nombre, len(a.valores), a.media(), a.errorTipico(), 100*float64(a.quiebra)/n, a.puestos/n)
	}

	fmt.Printf("\nNuestro resultado según quién estaba en la mesa:\n")
	fmt.Printf("%-13s %6s %14s %9s %8s\n", "con", "mesas", "fichas (media)", "± error", "quiebra")
	rivales := make([]string, 0, len(contra))
	for rival := range contra {
		rivales = append(rivales, rival)
	}
	sort.Strings(rivales)
	for _, rival := range rivales {
		a := contra[rival]
		n := float64(len(a.valores))
		fmt.Printf("%-13s %6d %14.0f %9.0f %7.0f%%\n", rival, len(a.valores), a.media(), a.errorTipico(), 100*float64(a.quiebra)/n)
	}
}
