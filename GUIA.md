# Guía del equipo: compilar y probar el bot

Pasos para dejar el bot corriendo en tu PC desde cero. Los comandos son para
**PowerShell en Windows** (en Linux/Mac cambia solo las rutas y los `.exe`).

## 1. Requisitos

- **Go 1.27.4 o más nuevo**: descárgalo de https://go.dev/dl/ e instálalo.
  Verifica con:
  ```powershell
  go version
  ```
- **Git**: https://git-scm.com/downloads. Verifica con `git --version`.

## 2. Clonar el repo del bot

```powershell
git clone https://github.com/OsquiCnapi/pokerfight-bot-team-wazaaa.git
cd pokerfight-bot-team-wazaaa
```

## 3. Compilar el bot

```powershell
go build ./...
```

La primera vez, Go descarga solo las dependencias (el SDK oficial del torneo,
`botsdk`) y las guarda en su caché. Si no imprime nada, compiló bien.

> No necesitas instalar el SDK a mano: está declarado en `go.mod` y Go lo
> resuelve automáticamente.

## 4. Clonar y compilar el kit oficial (para probar en local)

El bot es un cliente: necesita una **mesa** a la cual conectarse. La mesa y los
bots de práctica vienen en el kit oficial del torneo. Clónalo **fuera** del
repo del bot (por ejemplo, como carpeta hermana):

```powershell
cd ..
git clone https://github.com/aicspucp/pokerfight-arena-participantes.git
cd pokerfight-arena-participantes
go build -o bin/ ./cmd/... ./bots/...
```

Esto deja `mesa.exe` y los rivales (`conservador.exe`, `estadistico.exe`,
`bandido.exe`, `espia.exe`, etc.) en la carpeta `bin\`.

**Importante: nunca modifiques nada dentro del kit.** Si el kit se actualiza,
haz `git pull` ahí y vuelve a correr el `go build -o bin/ ...`.

## 5. Jugar una mesa local

Necesitas varias terminales (una por proceso).

**Terminal 1 — la mesa** (en la carpeta del kit). Esta es la configuración de
la clasificatoria: mesa de 5-6, stack 10 000, ciegas 50/100, 50 manos, 5 s por
jugada:

```powershell
.\bin\mesa.exe -addr :9000 -jugadores 6 -min-jugadores 5 -rondas 50 -stack 10000 -ciega-chica 50 -ciega-grande 100 -timeout-ms 5000
```

> Para una prueba rápida de que todo funciona, basta con
> `.\bin\mesa.exe -addr :9000 -jugadores 2 -min-jugadores 2 -timeout-ms 5000`
> y un solo rival.

**Terminales 2 a 5 — los rivales** (en la carpeta del kit):

```powershell
.\bin\conservador.exe -addr localhost:9000 -token rival-1
.\bin\espia.exe -addr localhost:9000 -token rival-2
.\bin\bandido.exe -addr localhost:9000 -token rival-3
.\bin\estadistico.exe -addr localhost:9000 -token rival-4
```

**Última terminal — nuestro bot** (en la carpeta del repo del bot):

```powershell
go run . -addr localhost:9000 -token mi-bot
```

Cuando se llenan los asientos, la mesa arranca sola y al terminar imprime las
posiciones finales.

En local el token puede ser cualquier texto; no se valida.

Dos opciones útiles de nuestro bot:

- `-detalle` escribe cada decisión con su motivo (equidad estimada, cuánto se
  espera que se retiren los rivales), el resultado de cada mano con las cartas
  que se mostraron y, cada 25 manos, lo que aprendió de cada rival. Es la forma
  de entender por qué ganó o perdió una mano:
  ```powershell
  go run . -addr localhost:9000 -token mi-bot -detalle
  ```
- `-reconectar 0` hace que el bot termine cuando la mesa cierra. Sin eso se
  queda 10 minutos intentando volver a conectarse, que es lo que se quiere el
  día del torneo pero molesta en una prueba local (se corta con Ctrl+C).

## 6. Medir el bot (no fiarse de una sola mesa)

Una mesa de 50 manos la decide la suerte. Para saber si un cambio mejora o
empeora el bot hay que jugar cientos de mesas y promediar. Todo esto se corre
desde la carpeta del repo del bot, con el kit ya compilado en la carpeta
hermana.

**Compilar el bot y la herramienta de medición** (después de cada cambio):

```powershell
go build -o bin/ .
go build -o bin/ ./herramientas/medir
```

**Muchas mesas con las condiciones de la clasificatoria.** Sortea 4 rivales del
kit por mesa, juega 50 manos con 10 000 fichas y ciegas 50/100, y al final
muestra las fichas promedio de cada bot, cada cuánto quiebra y nuestro
resultado según quién estaba en la mesa:

```powershell
.\bin\medir.exe -mesas 500 -paralelo 12
```

La columna `± error` dice cuánto puede moverse el promedio por pura suerte: dos
versiones del bot solo son distintas si sus promedios difieren en bastante más
que eso. Opciones: `-jugadores 6`, `-stack 3000`, `-rivales darwin,espia`,
`-registros carpeta` (guarda el `-detalle` de cada mesa) y `-ajustes` (cambia
parámetros de la estrategia sin recompilar, ver `estrategia/parametros.go`):

```powershell
.\bin\medir.exe -mesas 1000 -ajustes '{\"reserva\":30}'
```

**Duelos uno contra uno con la arena oficial** (500 manos por duelo, ida y
vuelta con las mismas cartas). Se corre desde la carpeta del kit:

```powershell
cd ..\pokerfight-arena-participantes
.\bin\arena.exe correr -config ..\pokerfight-bot-team-wazaaa\pruebas\duelos.json
```

La tabla final trae una columna `timeouts`: la nuestra tiene que estar en 0.

**Pruebas automáticas** (evaluador de manos, reconstrucción de la mano y que el
bot siempre devuelva una acción válida):

```powershell
go test ./...
```

## 7. Cómo está organizado el código

| Carpeta | Qué hace |
|---|---|
| `main.go` | Arranque: conexión, reconexión y vigilante de tiempo. Sin lógica de poker |
| `equity/` | Evaluador de manos rápido, simulación Monte Carlo y fuerza de cada mano sobre una mesa |
| `partida/` | Reconstruye la mano en curso: quién apostó cuánto, con qué pozo |
| `rivales/` | Perfil de cada rival: qué tan seguido entra, apuesta y se retira, y qué mostró en los showdowns |
| `estrategia/` | Las decisiones: lectura de rangos, preflop, postflop y los parámetros ajustables |
| `herramientas/` | `medir` (muchas mesas) y `gentabla` (genera la tabla preflop) |
| `pruebas/` | Configuración de la arena para los duelos contra el kit |

El análisis de los bots del kit y los resultados medidos están en
[ANALISIS.md](ANALISIS.md).

## 8. El token del equipo (⚠️ leer sí o sí)

- El token real del torneo (formato `pfb-...`) **nunca va en el código, ni en
  ningún archivo del repo, ni en el historial de Git**. Se pasa siempre por
  línea de comandos con `-token`.
- Solo una persona del equipo (quien tiene el token) se conecta al servidor de
  pruebas: **una segunda conexión con el mismo token desconecta a la primera**.
- Antes de cada `git push`, revisa con `git status` que no estés subiendo nada
  con el token.

## 9. Flujo de trabajo del equipo

- `git pull` antes de empezar cada sesión.
- `main` siempre debe compilar y jugar. Cada cambio va en una rama propia:
  ```powershell
  git switch -c mi-rama
  ```
  y se integra con un Pull Request revisado por otro integrante.
- Después de cualquier cambio, verifica con `go build ./...` y `go test ./...`
  antes de pedir revisión. Si el cambio toca la estrategia, mide antes y
  después (sección 6).

## 10. El día del torneo

```powershell
go build -o bin/ .
.\bin\pokerfight-bot-team-wazaaa.exe -addr HOST:PUERTO -token TU_TOKEN
```

- Usa el ejecutable compilado, no `go run`.
- El bot se conecta una vez y se queda conectado toda la sesión. Entre ronda y
  ronda no llega nada: es normal, no lo cierres.
- Si la conexión se cae, vuelve a entrar solo con el mismo token. Si se cierra
  el programa, vuelve a lanzarlo con el mismo comando: recupera la silla y las
  fichas (pierde lo que había aprendido de los rivales, nada más).
- Una sola copia corriendo: una segunda conexión con el mismo token desconecta
  a la primera.
- Máquina enchufada, sin suspensión y con buena conexión.

## Problemas comunes

| Síntoma | Causa y solución |
|---|---|
| `go: command not found` / no se reconoce | Go no está instalado o falta reiniciar la terminal tras instalarlo |
| El bot imprime `no se pudo conectar a localhost:9000` | La mesa no está corriendo, o está en otro puerto |
| La mesa no arranca la partida | Faltan jugadores: conecta tantos bots como pida `-min-jugadores` |
| `mesa.exe` no existe | Falta el paso 4 (`go build -o bin/ ...` dentro del kit) |
