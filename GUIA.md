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

Cuando se llenan los asientos, la mesa arranca sola. Al terminar imprime las
posiciones finales, y en la carpeta `resultados\` del kit queda el historial de
cada mano **con las cartas de todos**: úsalo para entender por qué ganamos o
perdimos una mano.

En local el token puede ser cualquier texto; no se valida.

## 6. El token del equipo (⚠️ leer sí o sí)

- El token real del torneo (formato `pfb-...`) **nunca va en el código, ni en
  ningún archivo del repo, ni en el historial de Git**. Se pasa siempre por
  línea de comandos con `-token`.
- Solo una persona del equipo (quien tiene el token) se conecta al servidor de
  pruebas: **una segunda conexión con el mismo token desconecta a la primera**.
- Antes de cada `git push`, revisa con `git status` que no estés subiendo nada
  con el token.

## 7. Flujo de trabajo del equipo

- `git pull` antes de empezar cada sesión.
- `main` siempre debe compilar y jugar. Cada cambio va en una rama propia:
  ```powershell
  git switch -c mi-rama
  ```
  y se integra con un Pull Request revisado por otro integrante.
- Después de cualquier cambio, verifica con `go build ./...` antes de pedir
  revisión.

## Problemas comunes

| Síntoma | Causa y solución |
|---|---|
| `go: command not found` / no se reconoce | Go no está instalado o falta reiniciar la terminal tras instalarlo |
| El bot imprime `no se pudo conectar a localhost:9000` | La mesa no está corriendo, o está en otro puerto |
| La mesa no arranca la partida | Faltan jugadores: conecta tantos bots como pida `-min-jugadores` |
| `mesa.exe` no existe | Falta el paso 4 (`go build -o bin/ ...` dentro del kit) |
