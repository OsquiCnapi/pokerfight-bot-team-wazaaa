# Análisis de los bots del kit y cómo les gana nuestro bot

Este documento resume qué hace cada bot de práctica del kit oficial (leyendo su
código, no su descripción), por dónde se le gana a cada uno, cómo está armado
nuestro bot y qué resultados dio al medirlo. Las cifras salen de
`herramientas/medir` y de la arena oficial; cómo repetirlas está en
[GUIA.md](GUIA.md), sección 6.

Vocabulario mínimo: **equidad** es la parte del pozo que una mano gana en
promedio si se reparten todas las cartas; **rango** es el conjunto de manos que
un jugador puede tener dadas sus acciones; **farol** es apostar sin mano para
que el rival se retire; **all-in** es apostar todas las fichas.

## 1. Resultados

**Mesas de 5 con las condiciones de la clasificatoria** (10 000 fichas, ciegas
50/100, 50 manos, 4 rivales del kit sorteados por mesa, 1500 mesas):

| Bot | Fichas al final (media) | ± error | Quiebra | Puesto medio |
|---|---|---|---|---|
| **Team Wazaaa** | **15 125** | 259 | **8 %** | 2.22 |
| darwin | 12 726 | 482 | 41 % | 2.85 |
| espia | 11 069 | 398 | 37 % | 2.82 |
| bandido | 10 984 | 306 | 20 % | 2.75 |
| base | 10 196 | 369 | 38 % | 2.97 |
| estadistico | 9 624 | 378 | 42 % | 3.04 |
| aleatorio | 3 977 | 328 | 82 % | 4.18 |
| conservador | 2 588 | 69 | 14 % | 3.73 |

Todos empiezan con 10 000. Nuestro bot termina con un 51 % más en promedio y es
el que menos quiebra de los que ganan fichas. El resultado es parejo sin
importar quién esté en la mesa: entre 14 542 (con aleatorio) y 15 750 (con
conservador).

**Duelos uno contra uno en la arena oficial** (500 manos por duelo, ida y vuelta
con las mismas cartas, fichas repuestas en cada mano): 16 duelos jugados, 16
ganados, 0 timeouts.

| Rival | Ganancia nuestra por mano |
|---|---|
| espia | +382 fichas |
| estadistico | +265 |
| darwin | +152 |
| conservador | +140 |
| bandido | +70 |
| aleatorio | +65 |
| novato | +49 |
| base | +39 |

**Otras condiciones** (entre 240 y 300 mesas cada una): mesas de 6, 15 092 de media y 6 % de
quiebra; mesas de 3, 15 061 y 7 %; con 30 000 fichas, 38 860 y 2 %; con solo
3000 fichas (30 ciegas) todos quiebran mucho, nosotros 25 %, pero seguimos
primeros con 5350 de media contra 3214 del segundo.

## 2. Qué hace cada bot del kit y por dónde se le gana

| Bot | Cómo decide | Debilidad | Cómo la aprovechamos |
|---|---|---|---|
| **aleatorio** | Elige al azar entre las acciones válidas, con montos al azar. Va all-in en una de cada cuatro o cinco decisiones | Sus apuestas no dicen nada de su mano | Su perfil muestra enseguida que apuesta siempre y con cualquier cosa; se le paga con manos buenas sin regalarle el stack con manos medias |
| **conservador** | Nunca apuesta. Paga si le cuesta hasta 3 ciegas grandes; si no, se retira | Se retira siempre ante más de 3 ciegas y paga siempre menos que eso | El bot aprende a qué tamaños se retira cada rival: le roba las ciegas subiendo y le cobra apuestas chicas cuando tiene mano |
| **base** | Preflop, fórmula de Chen. Después, mira solo la jugada armada: con trío o más apuesta 3/4 del pozo; con cualquier par apuesta 40 %; con carta alta pasa y se retira | Cuando pasa, no tiene nada. Cuenta como "par" el que está entero en la mesa | Apostar cuando pasa se lleva el pozo casi siempre; su apuesta grande es trío o más y se respeta |
| **estadistico** | Calcula su equidad contra manos **al azar** y la compara con el precio de pagar | Supone que quien le apuesta tiene cualquier cosa, así que paga demasiado | Apuestas grandes por valor: paga con manos que van perdiendo. Casi no se le farolea |
| **espia** | Equidad contra manos al azar más un perfil de cada rival: farolea a quien se retira mucho | Igual que estadistico, sobrevalora su mano frente a una apuesta | Apuestas por valor; es contra quien más ganamos (+382 por mano) |
| **bandido** | Cuatro estilos fijos (roca, tiburón, maníaco, pagador) y elige el que mejor le viene rindiendo. Mide la mano con reglas simples, sin simular | Con carta alta se retira ante casi cualquier apuesta; su subida es siempre mano fuerte | Apostar cuando pasa; retirarse cuando sube |
| **darwin** | Equidad contra una mano al azar, con umbrales hallados por un algoritmo genético. Paga muy seguido preflop; después del flop apuesta casi el pozo, y va all-in directo solo con manos enormes | Entra a demasiadas manos; su all-in es transparente | Subirle preflop con manos buenas (paga con peores); nunca pagarle un all-in sin una mano casi imbatible |
| **novato** | Solo condicionales, sin azar: tabla de manos iniciales y reglas por tipo de jugada | Hace siempre lo mismo en la misma situación | Lo cubre el juego normal del bot |

La idea común: **casi todos los bots del kit miden su mano contra manos al
azar**. Les va bien entre ellos, pero frente a alguien que solo apuesta fuerte
cuando tiene mano, pagan de más. Y casi todos deciden con umbrales fijos, así
que lo que hacen revela bastante lo que tienen.

## 3. Cómo juega nuestro bot

Cada decisión sigue los mismos cuatro pasos.

**1. Reconstruir la mano** (`partida/`). El historial de la mesa no trae los
montos de los call ni de los all-in. El bot vuelve a jugar la mano desde la foto
inicial y anota, para cada acción, cuánto había en el pozo y cuánto costaba.

**2. Leer el rango de cada rival** (`estrategia/lectura.go`). Cada rival empieza
pudiendo tener cualquiera de las 1326 manos. Cada acción suya cambia el peso de
cada mano: quien sube preflop rara vez tiene 7-2; quien apuesta fuerte en el
river suele tener una jugada hecha. Cuánto se le cree a cada acción depende de
su perfil.

**3. Conocer a cada rival** (`rivales/`). Por cada alias se cuenta con qué
frecuencia entra, sube, apuesta y se retira según el tamaño de la apuesta, y
con qué manos hizo cada cosa cuando hubo showdown. Como se ven pocas manos de
cada uno, todo parte de lo que haría un bot típico y los datos lo corrigen. Los
alias son estables durante todo el torneo, así que lo aprendido sirve si un
rival vuelve a tocar.

**4. Elegir la jugada de mayor valor** (`estrategia/preflop.go` y
`postflop.go`). Se simula la equidad de la mano propia **contra esos rangos**,
no contra manos al azar (10 000 simulaciones tardan unos 2 milisegundos). Para
pasar, pagar y cada tamaño de apuesta o subida se calcula cuánto vale: con qué
probabilidad se retiran todos, y cuánto se gana o se pierde si pagan con la
parte de su rango que sigue. Se elige la mayor.

Tres decisiones de diseño que salieron de medir:

- **Aversión a quebrar.** En la clasificatoria quedarse sin fichas es quedar
  eliminado. El valor de una jugada no es el promedio simple de fichas sino el
  promedio de su logaritmo (criterio de Kelly): las apuestas chicas se valoran
  casi igual, pero jugarse el stack entero exige cerca de 75 % de equidad en
  vez de 50 %. En la mesa final, donde solo cuenta ganar, el bot arriesga más;
  la reconoce porque todos vuelven a empezar con las mismas fichas.
- **No inflar pozos con manos medias.** Las apuestas grandes y las subidas se
  reservan para cuando hay ventaja clara si pagan, o cuando casi siempre se
  retiran.
- **Un pago que deja pocas fichas detrás se valora como jugarse todo.** Lo que
  queda va a entrar en la calle siguiente.

Lo que protege al bot de perder fichas por errores que no son de poker
(`main.go`): un vigilante que responde la jugada segura si una decisión tarda
más de 2 segundos o falla, la traducción de cada decisión a una acción que la
mesa acepta, y la reconexión automática con el mismo token.

## 4. Lo que enseñaron las mediciones

El bot pasó por varias versiones; cada cambio se midió con cientos o miles de
mesas.

| Versión | Fichas (media) | Quiebra |
|---|---|---|
| Primera versión completa | ~16 000 a 18 000 | 25 a 33 % |
| + aversión a quebrar y rangos de pago más fuertes | 15 309 | 14 % |
| + subir solo con ventaja clara, compromiso del stack | 15 359 | 11 % |
| + creerle más a las apuestas de quien ya venía pagando | 14 824 | 7 % |
| + modelo de retiro por fuerza absoluta (el actual) | 15 125 | 8 % |

- La primera versión ganaba más fichas en promedio pero quebraba en una de cada
  tres mesas: sobrevivir diez rondas así es casi imposible. Bajar la quiebra a
  8 % costó poco promedio.
- Comparadas con 1500 mesas cada una, dos formas de estimar con qué manos un
  rival se retira dieron 14 860 y 16 218 fichas con la misma quiebra. Sin medir
  no había forma de saber cuál era mejor.
- El umbral para farolear se probó en tres valores y no cambió nada medible
  (15 594, 15 513 y 15 590): no vale la pena afinarlo.
- La cautela se regula con un solo número (`reserva` en
  `estrategia/parametros.go`): con 15, quiebra 6 % y gana 15 166; con 60,
  quiebra 12 % y gana 16 518. Quedó en 20.

## 5. Límites conocidos

- Todo está medido contra los ocho bots del kit. Los bots de otros equipos
  pueden jugar distinto; el modelo de rivales se adapta, pero con 50 manos por
  mesa aprende poco de cada uno.
- La quiebra de 8 % por mesa viene sobre todo de pagar all-ins preflop con
  buena equidad contra bots que los hacen con cualquier cosa, y de perder con
  una mano fuerte contra otra mejor. Se puede bajar más subiendo la cautela, a
  costa de fichas.
- Contra base y bandido ganamos menos que contra el resto: son los que menos
  pagan de más.
- Si el programa se cierra y se vuelve a lanzar, recupera silla y fichas pero
  pierde lo aprendido de los rivales.
- Falta probarlo en el servidor de pruebas real.
