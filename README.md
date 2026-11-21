# El Tiburón Wazaaa 🦈

Bot de Texas Hold'em No-Limit del **equipo Wazaaa** para PokerFight (AICS PUCP + DSC PUCP), escrito en Go.

**En una frase:** es un tiburón que lee la mente de sus rivales con estadística, calcula cada jugada con miles de simulaciones por segundo y, a pesar de ser un tralalero tralala, le tiene un miedo sano a quedarse sin fichas, humo mi causa.

| Resultado medido (1500 mesas de 5, reglas de la clasificatoria) | Fichas al final | Quiebra |
|---|---|---|
| **El Tiburón Wazaaa** | **15 125** | **8 %** |
| darwin (el mejor bot del kit) | 12 726 | 41 % |
| espía, bandido, base, estadístico | 9 600 a 11 100 | 20 % a 42 % |

Todos empiezan con 10 000 fichas. En duelos uno contra uno con la arena oficial ganó **16 de 16** partidas, sin un solo timeout.

## Cómo piensa (la lógica de creación)

Casi todos los bots del kit miden su mano **contra manos al azar** y deciden con umbrales fijos. Eso los vuelve fáciles de leer: pagan de más cuando alguien apuesta fuerte. Nuestro bot parte de la idea contraria: *lo que hace un rival dice qué mano tiene*. En cada jugada hace cuatro cosas:

1. **Reconstruye la mano** (`partida/`). La mesa no informa los montos de los call ni de los all-in. El bot vuelve a jugar la mano desde la foto inicial y anota, para cada acción, cuánto había en el pozo y cuánto costaba.
2. **Conoce a cada rival** (`rivales/`). Cada alias (estable todo el torneo) tiene un perfil: con qué frecuencia entra, sube, apuesta y se retira según el tamaño de la apuesta, y con qué manos lo hizo cuando hubo showdown. Como se ven pocas manos de cada uno, todo parte de lo que haría un bot típico y los datos lo van corrigiendo (estimación bayesiana con priors).
3. **Lee su rango** (`estrategia/lectura.go`). Cada rival empieza pudiendo tener cualquiera de las 1326 manos iniciales. Cada acción suya cambia el peso de cada mano: quien sube preflop rara vez tiene 7-2, quien apuesta fuerte en el river suele tener una jugada hecha.
4. **Elige la jugada de mayor valor** (`estrategia/preflop.go` y `postflop.go`). Simula su equidad **contra esos rangos**, no contra manos al azar, y compara pasar, pagar y cuatro tamaños de apuesta o subida: con qué probabilidad se retiran todos, y cuánto se gana o se pierde si pagan con la parte de su rango que sigue.

Cómo juega según la situación:

- **Preflop:** abre con entre el 12 % y el 48 % de las manos según cuántos rivales faltan por hablar (más ancho en el botón, más cerrado desde posiciones tempranas), y roba las ciegas contra quienes se retiran seguido. Con 12 ciegas o menos de stack, juega a todo o nada.
- **Después del flop:** apuesta por valor cuando le pagan manos peores, hace faroles cuando es probable que se retiren manos mejores, y se retira cuando pagar no compensa.

### La parte malcriada: miedo a quebrar, lento pero seguro

En la clasificatoria quedarse sin fichas es quedar eliminado, así que perder todo el stack cuesta mucho más de lo que vale ganar otro igual. En lugar de promediar fichas, el bot promedia su **logaritmo** (el criterio de Kelly): las apuestas chicas se valoran casi igual, pero jugarse el stack entero exige cerca de 75 % de equidad en vez de 50 %. El tralalero solo muerde cuando tiene ventaja clara. En la mesa final, donde solo importa ganar, **se da cuenta de que es la final** (todos vuelven a empezar con las mismas fichas) y arriesga más.

La primera versión ganaba más fichas pero quebraba en una de cada tres mesas; bajar la quiebra a 8 % costó poco promedio. Esa y cada decisión de diseño se comprobó jugando cientos o miles de mesas: una sola mesa de 50 manos es puro ruido.

## Mecánica y robustez

- **Velocidad:** evaluador de manos propio con máscaras de bits (26 ns por mano de 7 cartas, 10 000 simulaciones en unos 2 ms). Una decisión toma milisegundos; el plazo de la mesa es de 5 s. Su resultado se compara en las pruebas contra el evaluador oficial en 60 000 manos aleatorias.
- **Siempre responde algo válido:** las acciones salen del SDK oficial (`botsdk`), que recorta montos a lo que la mesa acepta. Un vigilante juega check o fold si una decisión tarda más de 2 s o falla. Una prueba automática lanza 3000 situaciones inventadas y exige una acción válida en todas.
- **Reconexión:** si la conexión se cae vuelve a entrar sola, con el mismo token, durante 10 minutos. Recupera silla y fichas, y conserva lo aprendido de los rivales mientras el programa siga corriendo.
- **Seguridad:** el token se pasa por `-token` y nunca vive en el código ni en Git.

## Cómo está organizado

| Carpeta | Qué hace |
|---|---|
| `main.go` | Arranque: conexión, reconexión y vigilante de tiempo. Sin lógica de poker |
| `equity/` | Evaluador rápido, simulación Monte Carlo y fuerza de cada mano sobre una mesa |
| `partida/` | Reconstrucción de la mano en curso |
| `rivales/` | Perfil de cada rival |
| `estrategia/` | Lectura de rangos, preflop, postflop y parámetros ajustables |
| `herramientas/` | `medir` (muchas mesas locales) y `gentabla` (tabla preflop) |

Más detalle: [ANALISIS.md](ANALISIS.md) (qué hace cada bot del kit, por dónde se le gana y qué mediciones guiaron cada cambio) y [GUIA.md](GUIA.md) (compilar, probar y medir).

## Cómo ejecutarlo

```powershell
go build -o bin/ .
.\bin\pokerfight-bot-team-wazaaa.exe -addr HOST:PUERTO -token TU_TOKEN
```