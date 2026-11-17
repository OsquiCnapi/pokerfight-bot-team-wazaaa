// Bot del equipo Wazaaa para PokerFight.
//
// Este archivo es solo el arranque: conecta el bot a la mesa, lo vuelve a
// conectar si se cae y garantiza que siempre responda a tiempo. Las decisiones
// de poker están en el paquete estrategia.
//
// Uso:
//
//	go run . -addr localhost:9000 -token mi-bot
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/aicspucp/pokerfight-arena-participantes/pkg/botsdk"

	"github.com/OsquiCnapi/pokerfight-bot-team-wazaaa/estrategia"
)

var (
	direccion = flag.String("addr", "localhost:9000", "dirección de la mesa (host:puerto)")
	token     = flag.String("token", "", "token del equipo. En una mesa local puede ser cualquier texto")
	detalle   = flag.Bool("detalle", false, "escribe en stderr cada decisión con su motivo")
	insistir  = flag.Duration("reconectar", 10*time.Minute, "cuánto tiempo seguir intentando reconectar tras perder la mesa (0 = no reconectar)")
	ajustes   = flag.String("ajustes", "", `cambia parámetros de la estrategia para hacer pruebas, en JSON. Ejemplo: {"reserva":20}`)
)

// plazoMaximo es el techo de tiempo por decisión, por si la mesa no informa
// un plazo. El de la competencia es de 5 segundos.
const plazoMaximo = 2 * time.Second

// botWazaaa envuelve la estrategia con un vigilante de tiempo: si una decisión
// tarda demasiado o falla, responde la jugada segura en lugar de dejar que la
// mesa anote un timeout.
type botWazaaa struct {
	cerebro *estrategia.Cerebro
	aviso   *log.Logger
}

func (*botWazaaa) Nombre() string { return "team-wazaaa" }

func (b *botWazaaa) Decidir(ctx context.Context, t botsdk.Turno) botsdk.Accion {
	plazo := plazoMaximo
	if t.Timeout > 0 && t.Timeout/2 < plazo {
		plazo = t.Timeout / 2
	}

	// El canal tiene lugar para una respuesta: si el vigilante ya contestó, la
	// decisión tardía se escribe igual y la goroutine termina sin quedar colgada.
	respuesta := make(chan botsdk.Accion, 1)
	go func() {
		defer func() {
			if falla := recover(); falla != nil {
				b.aviso.Printf("la estrategia falló (%v): juego la acción segura", falla)
				respuesta <- estrategia.Segura(t)
			}
		}()
		respuesta <- b.cerebro.Decidir(ctx, t)
	}()

	select {
	case accion := <-respuesta:
		return accion
	case <-time.After(plazo):
		b.aviso.Printf("la decisión pasó de %s: juego la acción segura", plazo)
		return estrategia.Segura(t)
	}
}

// Los avisos de la mesa se pasan a la estrategia tal cual. Se protegen igual
// que Decidir: un fallo al aprender de una mano no puede tirar la conexión.

func (b *botWazaaa) ManoInicio(estado botsdk.EstadoPublico, mano botsdk.Mano) {
	defer b.proteger()
	b.cerebro.ManoInicio(estado, mano)
}

func (b *botWazaaa) ManoFin(resultado botsdk.ResultadoMano) {
	defer b.proteger()
	b.cerebro.ManoFin(resultado)
}

func (b *botWazaaa) EstadoActualizado(estado botsdk.EstadoPublico) {
	defer b.proteger()
	b.cerebro.EstadoActualizado(estado)
}

func (b *botWazaaa) proteger() {
	if falla := recover(); falla != nil {
		b.aviso.Printf("fallo al procesar un mensaje de la mesa: %v", falla)
	}
}

func main() {
	flag.Parse()

	aviso := log.New(os.Stderr, "team-wazaaa: ", log.LstdFlags)
	bot := &botWazaaa{
		cerebro: estrategia.Nuevo(uint64(time.Now().UnixNano())),
		aviso:   aviso,
	}
	if *detalle {
		bot.cerebro.Registro = log.New(os.Stderr, "", 0)
	}
	if *ajustes != "" {
		// Los parámetros no mencionados conservan su valor por defecto.
		if err := json.Unmarshal([]byte(*ajustes), &bot.cerebro.P); err != nil {
			aviso.Fatalf("-ajustes no es JSON válido: %v", err)
		}
	}

	opciones := botsdk.Opciones{Direccion: *direccion, Token: *token, Log: aviso}
	if opciones.Token == "" {
		opciones.Token = bot.Nombre()
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()

	// El SDK no reconecta solo. Si la conexión se cae en medio del torneo, la
	// mesa juega por el bot hasta que vuelva con el mismo token, así que se
	// reintenta enseguida. La memoria de rivales vive en `bot` y sobrevive a
	// la reconexión.
	var caida time.Time
	for {
		inicio := time.Now()
		err := botsdk.Correr(ctx, bot, opciones)
		if ctx.Err() != nil {
			return
		}
		// Si la sesión duró, lo que se cortó era una conexión que funcionaba:
		// el tiempo para reintentar empieza de nuevo.
		if caida.IsZero() || time.Since(inicio) > 10*time.Second {
			caida = time.Now()
		}
		if time.Since(caida) >= *insistir {
			if err != nil && !errors.Is(err, botsdk.ErrMesaCerro) {
				aviso.Fatalf("sin conexión con la mesa: %v", err)
			}
			return
		}
		aviso.Printf("desconectado (%v); reintento en 1 s", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
