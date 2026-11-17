// Package rivales lleva la cuenta de cómo juega cada rival y la convierte en
// las estimaciones que usa la estrategia: con qué manos entra, cuánto apuesta,
// cuánto se retira según el tamaño de la apuesta y qué tan fuerte está cuando
// apuesta.
//
// En el torneo los rivales son alias aleatorios pero estables, así que lo que
// se aprende de uno sirve si vuelve a tocar en otra ronda. Como se ven pocas
// manos de cada rival, toda estimación parte de un prior (lo que haría un bot
// típico) y los datos lo van corrigiendo: con cero manos vistas manda el
// prior, con muchas mandan los datos.
package rivales

import (
	"fmt"
	"math"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/equity"
	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/partida"
)

// maxObservaciones acota la memoria por rival: lo más viejo se descarta.
const maxObservaciones = 120

// retiro es una vez que el rival tuvo que decidir frente a una apuesta.
type retiro struct {
	fraccion float32 // Tamaño de la apuesta sobre el pozo.
	ciegas   float32 // Lo que tenía que poner, en ciegas grandes.
	agresor  bool    // Él mismo había apostado o subido antes en esa calle.
	seRetiro bool
}

// muestra es la fuerza real de la mano con la que el rival hizo algo, vista en
// un showdown.
type muestra struct {
	fraccion float32 // Tamaño de su apuesta (o de la que pagó) sobre el pozo.
	fuerza   float32 // Puesto efectivo de su mano en esa mesa (0 peor, 1 mejor).
	allIn    bool
}

// Perfil es todo lo que se vio de un rival.
type Perfil struct {
	Manos int

	// Preflop.
	Decisiones int // Manos en las que tuvo que decidir preflop.
	Entro      int // De esas, en cuántas puso fichas por decisión propia.
	Subio      int // De esas, en cuántas subió.

	SinSubida int // Decisiones sin subida previa.
	Abrio     int // De esas, cuántas subió.
	Limpeo    int // De esas, cuántas solo pagó la ciega.

	AnteSubida int // Decisiones frente a una subida.
	PagoSubida int
	Resubio    int

	AllInPre int // Veces que fue all-in preflop apostando o subiendo.

	// Después del flop.
	PudoApostar int // Turnos sin apuesta que igualar.
	Aposto      int
	AnteApuesta int // Turnos frente a una apuesta.
	SeRetiro    int
	SubioPost   int

	retiros    []retiro // Frente a apuestas después del flop.
	retirosPre []retiro // Frente a subidas preflop.

	// Lo que mostró en showdowns.
	alApostar []muestra
	alSubir   []muestra
	preAbrir  []float32 // Puesto preflop (0 mejor) de las manos con que abrió.
	preResube []float32
	preAllIn  []float32
}

// frecuencia estima una probabilidad con un prior: es como haber visto
// `peso` casos previos con la proporción `prior`.
func frecuencia(exitos, intentos int, prior, peso float64) float64 {
	return (float64(exitos) + prior*peso) / (float64(intentos) + peso)
}

func acotar(x, minimo, maximo float64) float64 { return math.Max(minimo, math.Min(maximo, x)) }

// Entra estima en qué fracción de las manos pone fichas preflop por decisión
// propia (lo que en poker se llama VPIP).
func (p *Perfil) Entra() float64 { return frecuencia(p.Entro, p.Decisiones, 0.35, 6) }

// FrecAbrir estima con qué frecuencia sube cuando nadie subió antes.
func (p *Perfil) FrecAbrir() float64 { return frecuencia(p.Abrio, p.SinSubida, 0.15, 6) }

// FrecApuesta estima con qué frecuencia apuesta después del flop cuando puede
// pasar gratis.
func (p *Perfil) FrecApuesta() float64 { return frecuencia(p.Aposto, p.PudoApostar, 0.33, 5) }

// FrecSubida estima con qué frecuencia sube cuando le apuestan después del flop.
func (p *Perfil) FrecSubida() float64 { return frecuencia(p.SubioPost, p.AnteApuesta, 0.08, 8) }

// rangoDesdeMuestras combina una estimación por frecuencia con lo visto en
// showdowns. Si un rival hace algo con "el X% mejor" de sus manos, las manos
// que muestra tienen puesto medio X/2: cada mano mostrada estima X como el
// doble de su puesto.
func rangoDesdeMuestras(porFrecuencia float64, muestras []float32) float64 {
	const pesoFrecuencia = 3.0
	suma := porFrecuencia * pesoFrecuencia
	for _, puesto := range muestras {
		suma += math.Min(2*float64(puesto)+0.02, 1)
	}
	return suma / (pesoFrecuencia + float64(len(muestras)))
}

// RangoAbrir estima con qué fracción de las manos (las mejores) abre subiendo.
func (p *Perfil) RangoAbrir() float64 {
	return acotar(rangoDesdeMuestras(p.FrecAbrir()*1.1, p.preAbrir), 0.03, 1)
}

// RangoEntrar estima con qué fracción de las manos entra al pozo sin subida.
func (p *Perfil) RangoEntrar() float64 {
	entra := frecuencia(p.Abrio+p.Limpeo, p.SinSubida, 0.40, 5)
	return acotar(entra*1.05, 0.08, 1)
}

// RangoResubir estima con qué fracción de las manos vuelve a subir.
func (p *Perfil) RangoResubir() float64 {
	porFrecuencia := frecuencia(p.Resubio, p.AnteSubida, 0.045, 10)
	return acotar(rangoDesdeMuestras(porFrecuencia, p.preResube), 0.015, 1)
}

// RangoAllInPre estima con qué fracción de las manos va all-in preflop con un
// stack grande. Arranca muy cerrado (un all-in de 100 ciegas suele ser una
// mano enorme) y se abre rápido contra quien lo hace seguido.
func (p *Perfil) RangoAllInPre() float64 {
	porFrecuencia := frecuencia(p.AllInPre, p.Manos, 0.02, 14) * 1.3
	return acotar(rangoDesdeMuestras(porFrecuencia, p.preAllIn), 0.02, 1)
}

// priorRetiro es la probabilidad de retirarse de un bot típico frente a una
// apuesta de ese tamaño después del flop.
func priorRetiro(fraccion float64) float64 {
	return acotar(0.27+0.20*fraccion, 0.22, 0.72)
}

// priorRetiroPre es lo mismo frente a una subida preflop que cuesta `ciegas`.
func priorRetiroPre(ciegas float64) float64 {
	return acotar(0.48+0.035*ciegas, 0.5, 0.93)
}

// estimarRetiro promedia los casos vistos, pesando más los que se parecen a la
// situación consultada en tamaño relativo al pozo y en tamaño absoluto. Hay
// bots que deciden por uno de los dos criterios (por ejemplo, "pago hasta tres
// ciegas"), y mirar los dos los cubre a ambos. Los casos en que el rival venía
// de apostar se separan de los que no: son situaciones distintas.
func estimarRetiro(casos []retiro, fraccion, ciegas float64, agresor bool, prior, pesoPrior float64) float64 {
	const anchoFraccion, anchoCiegas = 0.5, 0.6
	suma, peso := prior*pesoPrior, pesoPrior
	lf, lc := math.Log(math.Max(fraccion, 0.02)), math.Log(math.Max(ciegas, 0.2))
	for _, c := range casos {
		df := (lf - math.Log(math.Max(float64(c.fraccion), 0.02))) / anchoFraccion
		dc := (lc - math.Log(math.Max(float64(c.ciegas), 0.2))) / anchoCiegas
		k := math.Exp(-0.5 * (df*df + dc*dc))
		if c.agresor != agresor {
			k *= 0.2
		}
		peso += k
		if c.seRetiro {
			suma += k
		}
	}
	return suma / peso
}

// ProbRetiro estima la probabilidad de que el rival se retire frente a una
// apuesta después del flop que es `fraccion` del pozo y le cuesta `ciegas`.
// `agresor` indica si él mismo ya apostó o subió en esa calle: quien apuesta y
// recibe una subida casi nunca se retira, porque apostó teniendo algo.
func (p *Perfil) ProbRetiro(fraccion, ciegas float64, agresor bool) float64 {
	prior := priorRetiro(fraccion)
	if agresor {
		prior *= 0.35
	}
	return estimarRetiro(p.retiros, fraccion, ciegas, agresor, prior, 2.5)
}

// ProbRetiroPre es lo mismo frente a una subida preflop.
func (p *Perfil) ProbRetiroPre(fraccion, ciegas float64, agresor bool) float64 {
	prior := priorRetiroPre(ciegas)
	if agresor {
		prior *= 0.6
	}
	return estimarRetiro(p.retirosPre, fraccion, ciegas, agresor, prior, 2.5)
}

// visto resume lo que el rival mostró en showdowns al hacer algo: a partir de
// qué fuerza lo hace y cuánto pesa esa evidencia. Si lo hace con las manos de
// fuerza mayor que U, las que muestra tienen fuerza media (1+U)/2: cada mano
// mostrada estima U como 2·fuerza − 1. Pesan más las muestras de tamaño
// parecido al consultado, y un all-in se compara sobre todo con otros all-in.
func visto(muestras []muestra, fraccion float64, allIn bool) (umbral, peso float64) {
	const ancho = 0.7
	lf := math.Log(math.Max(fraccion, 0.05))
	suma := 0.0
	for _, m := range muestras {
		d := (lf - math.Log(math.Max(float64(m.fraccion), 0.05))) / ancho
		k := math.Exp(-0.5 * d * d)
		if m.allIn && allIn {
			k = 1 // Entre all-ins el tamaño relativo al pozo importa poco.
		} else if m.allIn != allIn {
			k *= 0.3
		}
		suma += k * acotar(2*float64(m.fuerza)-1, 0, 0.97)
		peso += k
	}
	if peso == 0 {
		return 0, 0
	}
	return suma / peso, peso
}

// VistoAlApostar devuelve la fuerza mínima con la que se lo vio apostar un
// tamaño parecido, y cuánta evidencia hay (0 si nunca mostró).
func (p *Perfil) VistoAlApostar(fraccion float64, allIn bool) (float64, float64) {
	return visto(p.alApostar, fraccion, allIn)
}

// VistoAlSubir es lo mismo para cuando sube una apuesta.
func (p *Perfil) VistoAlSubir(fraccion float64, allIn bool) (float64, float64) {
	return visto(p.alSubir, fraccion, allIn)
}

// ParteQueApuesta estima con qué fracción de su rango (la mejor) hace una
// apuesta de ese tamaño cuando nadie apostó antes. Las apuestas grandes se
// hacen con menos manos que las chicas, y un all-in de varias veces el pozo
// con muy pocas.
func (p *Perfil) ParteQueApuesta(fraccion float64, allIn bool) float64 {
	porTamano := acotar(1.15-0.45*fraccion, 0.25, 1)
	if allIn && fraccion > 1.2 {
		porTamano = math.Min(porTamano, 0.2)
	}
	return acotar(p.FrecApuesta()*porTamano, 0.03, 0.95)
}

// ParteQueSube estima con qué fracción de su rango sube una apuesta. `resube`
// indica que ya había una subida antes en la calle.
func (p *Perfil) ParteQueSube(allIn, resube bool) float64 {
	parte := p.FrecSubida()
	if resube {
		parte *= 0.5
	}
	if allIn {
		parte *= 0.8
	}
	return acotar(parte, 0.015, 0.9)
}

// Farol estima qué parte de sus apuestas son con cualquier cosa, sin relación
// con la fuerza de la mano.
func (p *Perfil) Farol() float64 {
	return acotar(0.08+0.5*(p.FrecApuesta()-0.4), 0.05, 0.45)
}

// Resumen describe al rival en una línea, para los registros.
func (p *Perfil) Resumen() string {
	return fmt.Sprintf("%d manos | entra %.0f%% | abre %.0f%% | apuesta %.0f%% | sube %.0f%% | se retira ante medio pozo %.0f%% | mostró %d apuestas",
		p.Manos, 100*p.Entra(), 100*p.FrecAbrir(), 100*p.FrecApuesta(), 100*p.FrecSubida(), 100*p.ProbRetiro(0.5, 3, false), len(p.alApostar)+len(p.alSubir))
}

// Modelo guarda un perfil por rival.
type Modelo struct {
	perfiles map[string]*Perfil
}

// NuevoModelo crea un modelo vacío.
func NuevoModelo() *Modelo { return &Modelo{perfiles: map[string]*Perfil{}} }

// De devuelve el perfil de un rival, creándolo si es la primera vez que se lo
// ve. Las estimaciones de un perfil vacío son las del prior.
func (m *Modelo) De(id string) *Perfil {
	p, ok := m.perfiles[id]
	if !ok {
		p = &Perfil{}
		m.perfiles[id] = p
	}
	return p
}

func recortar[T any](s []T) []T {
	if len(s) > maxObservaciones {
		return append(s[:0:0], s[len(s)-maxObservaciones:]...)
	}
	return s
}

// Observar actualiza los perfiles con una mano terminada. `mostradas` son las
// cartas reveladas en el showdown (puede estar vacío) y `mesa` las
// comunitarias finales.
func (m *Modelo) Observar(mano *partida.Mano, yo string, mostradas map[string][2]equity.Carta, mesa []equity.Carta) {
	if mano == nil || !mano.Completa {
		return
	}
	cg := float64(mano.CiegaGrande)

	for _, a := range mano.Asientos {
		if a.ID != yo {
			m.De(a.ID).Manos++
		}
	}

	decidio := map[string]bool{}
	entro := map[string]bool{}
	subio := map[string]bool{}
	// aposto[jugador] es la última calle en la que apostó o subió.
	aposto := map[string]partida.Calle{}

	for _, j := range mano.Jugadas {
		calleAgresiva, yaAposto := aposto[j.Jugador]
		agresor := yaAposto && calleAgresiva == j.Calle
		if j.Tipo.Agresiva() {
			aposto[j.Jugador] = j.Calle
		}
		if j.Jugador == yo {
			continue
		}
		p := m.De(j.Jugador)
		ciegas := float64(j.PorIgualar) / cg

		if j.Calle == partida.Preflop {
			if j.PorIgualar > 0 || j.Tipo.Agresiva() {
				decidio[j.Jugador] = true
			}
			if j.Tipo == partida.Call || j.Tipo.Agresiva() {
				entro[j.Jugador] = true
			}
			if j.Tipo.Agresiva() {
				subio[j.Jugador] = true
				if j.AllIn {
					p.AllInPre++
				}
			}
			if j.Subidas == 0 {
				p.SinSubida++
				switch {
				case j.Tipo.Agresiva():
					p.Abrio++
				case j.Tipo == partida.Call:
					p.Limpeo++
				}
			} else if j.PorIgualar > 0 {
				p.AnteSubida++
				switch {
				case j.Tipo.Agresiva():
					p.Resubio++
				case j.Tipo == partida.Call:
					p.PagoSubida++
				}
				p.retirosPre = recortar(append(p.retirosPre, retiro{float32(j.Fraccion()), float32(ciegas), agresor, j.Tipo == partida.Fold}))
			}
			continue
		}

		if j.PorIgualar == 0 {
			p.PudoApostar++
			if j.Tipo == partida.Bet {
				p.Aposto++
			}
			continue
		}
		p.AnteApuesta++
		switch j.Tipo {
		case partida.Fold:
			p.SeRetiro++
		case partida.Raise:
			p.SubioPost++
		}
		p.retiros = recortar(append(p.retiros, retiro{float32(j.Fraccion()), float32(ciegas), agresor, j.Tipo == partida.Fold}))
	}

	for id := range decidio {
		p := m.De(id)
		p.Decisiones++
		if entro[id] {
			p.Entro++
		}
		if subio[id] {
			p.Subio++
		}
	}

	m.observarShowdown(mano, yo, mostradas, mesa)
}

// observarShowdown anota con qué fuerza real hizo cada cosa quien mostró sus
// cartas.
func (m *Modelo) observarShowdown(mano *partida.Mano, yo string, mostradas map[string][2]equity.Carta, mesa []equity.Carta) {
	if len(mostradas) == 0 || len(mesa) < 5 {
		return
	}
	var fuerzas [4]*equity.FuerzaMesa
	fuerzaEn := func(calle partida.Calle) *equity.FuerzaMesa {
		if fuerzas[calle] == nil {
			fuerzas[calle] = equity.CalcularFuerza(mesa[:2+int(calle)])
		}
		return fuerzas[calle]
	}

	for _, j := range mano.Jugadas {
		cartas, mostro := mostradas[j.Jugador]
		if !mostro || j.Jugador == yo {
			continue
		}
		p := m.De(j.Jugador)
		combo := equity.IndiceCombo(cartas[0], cartas[1])

		if j.Calle == partida.Preflop {
			puesto := float32(equity.PuestoDeCombo(combo))
			switch {
			case j.Tipo.Agresiva() && j.AllIn:
				p.preAllIn = recortar(append(p.preAllIn, puesto))
			case j.Tipo.Agresiva() && j.Subidas == 0:
				p.preAbrir = recortar(append(p.preAbrir, puesto))
			case j.Tipo.Agresiva():
				p.preResube = recortar(append(p.preResube, puesto))
			}
			continue
		}

		fuerza := fuerzaEn(j.Calle).Efectivo[combo]
		switch j.Tipo {
		case partida.Bet:
			p.alApostar = recortar(append(p.alApostar, muestra{float32(j.FraccionPuesta()), fuerza, j.AllIn}))
		case partida.Raise:
			p.alSubir = recortar(append(p.alSubir, muestra{float32(j.FraccionPuesta()), fuerza, j.AllIn}))
		}
	}
}
