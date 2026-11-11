# Typedeck

[English](README.md) | Español

[![CI](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml/badge.svg)](https://github.com/cristobaltormo/typedeck/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cristobaltormo/typedeck?sort=semver)](https://github.com/cristobaltormo/typedeck/releases)
[![Licencia](https://img.shields.io/badge/licencia-MIT-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Sistemas](https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-soportados-lightgrey)](docs/COMPATIBILITY.md)

Convierte el teclado USB que ya tienes en un teclado de macros. Una Arduino Leonardo pequeña con un USB Host Shield se coloca entre
el teclado y el ordenador, deja pasar todas las teclas sin tocarlas y permite que cualquiera ejecute sus propias acciones: pulsar,
mantener o doble pulsación, capas, lanzadores de aplicaciones, comandos, SSH, textos, teclas multimedia, OBS Studio y más.

Escribir nunca depende del programa. Si no está en marcha, o se cierra por un fallo, la placa sigue reenviando el teclado por su
cuenta y cada tecla escribe su carácter de siempre.

¿Aún sin placa? En Windows, macOS y Linux, Typedeck puede tomar las teclas por software, con un gancho de teclado (Windows), un pequeño ayudante que filtra los eventos de teclado (macOS) o los dispositivos de entrada del núcleo (Linux) que activas tú. Es un trueque: no
hay hardware que montar, pero hay límites que la placa no tiene (mira [Límites](#límites)).

Typedeck es un único binario de Go sin dependencias. En reposo gasta unos 11 MB de RAM y nada de CPU, y reenviar una tecla tarda
unos 0,13 ms dentro de la placa. Funciona en macOS, Linux y Windows, y sus pruebas corren en los tres en cada subida.

![El editor de Typedeck con un teclado, capas y la acción de la tecla seleccionada](docs/images/editor.png)

Mueve un macro a otra tecla arrastrándolo; con Alt se copia en vez de mover.

![Arrastrando un macro a otra tecla](docs/images/drag.gif)

## Cómo se ve

El editor se abre en el navegador, en tu propio ordenador. Elige una tecla en el dibujo de tu teclado real y dale una acción.

| | |
|---|---|
| ![Una secuencia de pasos con grabador](docs/images/sequence.png) **Secuencias** con grabador, condiciones y pausas editables | ![La galería de paquetes](docs/images/gallery.png) **Una galería** de paquetes colocados en las teclas que tu teclado tiene |
| ![Lo que escribes, reconstruido en directo](docs/images/history.png) **Historial de tecleo**, opcional y local, con lo que escribes en directo | ![Tema claro](docs/images/editor-light.png) **Claro y oscuro**, en inglés y español |

## Cómo funciona

![Teclado, shield, Leonardo, ordenador y el programa Typedeck](docs/images/architecture.svg)

La placa recoge el teclado del shield, reenvía sus informes al ordenador como un teclado USB normal y retira solo las teclas que
tienen macro, enviándolas al programa por una línea serie. El programa decide qué hace cada gesto. Un latido cada segundo mantiene la
captura; sin él, la placa deja de capturar a los 5 segundos. Más en [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (en inglés).

**Sin placa (Windows, macOS y Linux).** La captura por software cambia la placa por un gancho de teclado de todo el sistema, dentro del
mismo programa en Windows, y por `typedeck-capture` en macOS: un pequeño ayudante en Swift que el programa lanza y vigila, que
instala un filtro de eventos y habla con el programa con líneas de texto por su entrada y salida estándar. El gancho o el filtro solo toma las teclas que tienen macro en la capa activa y deja pasar las demás sin que el programa las lea (salvo
detectando teclas o con el historial opcional activado, que registra lo que tecleas en todas las aplicaciones). Los gestos, las capas, las acciones y el editor son los
mismos. Está desactivada hasta que la eliges: el editor ofrece **Usar sin placa** cuando no aparece ninguna, y en Ajustes hay un
interruptor para volver. Un gancho ve todas las teclas, así que el editor te pide consentimiento antes, te dice qué hace y qué no, y lo registra en esa instalación: una copia de seguridad restaurada o un fichero importado no pueden activarla. No la uses en un equipo compartido con otros usuarios: el editor escucha en un puerto que comparten todos los usuarios locales.
Si el programa muere, el gancho muere con él y todas las teclas vuelven a escribir su carácter; en macOS el ayudante nota que el
programa ya no está y termina, lo que quita el filtro.

En Linux no hay gancho que instalar, así que la captura por software trabaja por debajo del escritorio: abre los teclados reales de
`/dev/input`, toma cada uno en exclusiva (`EVIOCGRAB`) y devuelve por un teclado virtual propio (`/dev/uinput`) todas las teclas que
no toma, con modificadores, autorrepetición y teclas multimedia; los LED del teclado hacen el camino contrario. Como está por debajo
del compositor, X11 y Wayland se comportan igual. El núcleo suelta el teclado cuando se cierran los descriptores del programa, así
que si el programa muere todas las teclas vuelven a escribir al instante. Necesita tu usuario en el grupo `input` y el módulo
`uinput`: los paquetes `.deb` y `.rpm` instalan la regla udev y cargan el módulo, tú te añades al grupo.

## Qué necesitas

| Pieza | Notas |
|---|---|
| Arduino Leonardo (ATmega32U4) | probado con una Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **con conector ICSP** | probado con uno de Yanmis |
| Un teclado USB con cable | hasta 500 mA |
| macOS 12 o posterior, Linux (X11 o Wayland) o Windows 10 y 11 | en [compatibilidad](docs/COMPATIBILITY.md) está qué se ha probado dónde |

Apila el shield sobre la Leonardo, enchufa el teclado al shield y la Leonardo al ordenador. Si hay un KVM o un hub por medio, lee
antes [docs/HARDWARE.md](docs/HARDWARE.md).

En Windows 10 y 11, en macOS 12 o más nuevo y en Linux la placa y el shield son opcionales: con la captura por software sirve cualquier
teclado, con cable, inalámbrico o el del portátil. En macOS necesita el ayudante `typedeck-capture` junto al programa y dos permisos
que le das tú (Accesibilidad y Monitorización de entrada). En Linux necesita acceso a `/dev/input` y a `/dev/uinput` (mira [problemas frecuentes](docs/TROUBLESHOOTING.md#software-capture-on-linux-says-it-is-not-running)).

## Instalar

En Debian, Ubuntu y Fedora, descarga el `.deb` o el `.rpm` de tu arquitectura desde la
[página de versiones](https://github.com/cristobaltormo/typedeck/releases). Instala el programa, la regla udev que deja a tu usuario abrir
la placa y el sketch del firmware:

```sh
sudo apt install ./typedeck_<versión>_amd64.deb      # o: sudo dnf install ./typedeck-<versión>-1.x86_64.rpm
```

En cualquier otro sitio, descarga el archivo comprimido o el binario suelto de tu equipo de la misma página:

```sh
curl -L -o typedeck https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-linux-amd64
chmod +x typedeck
```

En Windows, en PowerShell:

```powershell
Invoke-WebRequest https://github.com/cristobaltormo/typedeck/releases/latest/download/typedeck-windows-amd64.exe -OutFile typedeck.exe
```

El `typedeck-cli-windows-amd64.exe` que hay a su lado es el mismo programa para terminales (mira Ponerlo en marcha más abajo); los zip traen los dos.

Hay versiones para Linux (amd64, arm64), macOS (Intel y Apple silicon) y Windows (amd64 y arm64), cada una con su suma en
`SHA256SUMS` y una atestación de procedencia de la compilación (`gh attestation verify <archivo> --repo cristobaltormo/typedeck`). Para compilarlo tú necesitas Go 1.24 o posterior:

```sh
go install github.com/cristobaltormo/typedeck/cmd/typedeck@latest
```

Flashea la placa una vez. Cada versión adjunta el firmware ya compilado, `typedeck-firmware-<número>.hex`, y `arduino-cli` lo sube igual
en Windows, Linux y macOS:

```sh
arduino-cli core install arduino:avr
arduino-cli upload --fqbn arduino:avr:leonardo --port <puerto> --input-file typedeck-firmware-15.hex
```

Detén Typedeck antes y mira [docs/HARDWARE.md](docs/HARDWARE.md#flashing) para los nombres de puerto y los pasos. Para compilar el firmware
tú mismo hacen falta las dos librerías que usa ([firmware/README.md](firmware/README.md)).

## Ponerlo en marcha

```sh
typedeck doctor     # comprueba la configuración, las herramientas del sistema, el puerto, la placa y el teclado
typedeck            # arranca el programa; el editor está en http://127.0.0.1:7788
typedeck install    # arranca con tu sesión (typedeck uninstall lo quita)
```

- **macOS:** `typedeck install` crea un agente de arranque que lo inicia al abrir sesión. El cartel en pantalla es un pequeño ayudante en Swift:
  `swiftc -O macos/hud/hud.swift -o typedeck-hud`, junto al binario `typedeck`. El archivo de macOS ya trae los dos ayudantes
  (`typedeck-hud` y `typedeck-capture`, universales y con firma ad hoc); `make macos-helpers` los construye en un Mac con las
  herramientas de línea de comandos de Xcode. Los archivos descargados con un navegador llevan una marca de cuarentena que bloquea
  ayudantes sin firmar: `xattr -dr com.apple.quarantine <carpeta>` una vez. La captura por software pide **Accesibilidad** y
  **Monitorización de entrada** para `typedeck-capture` la primera vez que la activas (nunca antes); si se te pasa el aviso,
  añádelo a mano en Ajustes del Sistema, Privacidad y seguridad, con el botón `+`, eligiendo el archivo junto a `typedeck`.
  `typedeck doctor` dice cuál falta.
- **Linux:** copia `packaging/linux/70-typedeck.rules` a `/etc/udev/rules.d/` para que tu usuario abra la placa sin estar en el grupo
  `dialout`. Instala `xdg-utils` y `libnotify-bin`; `xdotool` (X11) o `wtype` (Wayland) solo importan si la placa no está conectada.
  Registro: `journalctl --user -u typedeck`.
- **Windows:** haz doble clic en `typedeck.exe` para arrancarlo: abre el editor en el navegador y pone un icono en el área de
  notificación (Abrir editor, estado, Reiniciar, Salir). Si lo abres otra vez solo abre el editor de la copia que ya está en marcha.
  `typedeck.exe install` hace que arranque al iniciar sesión, con una tarea programada sin límite de tiempo, que también funciona con
  batería y no lanza una segunda copia; si Windows no deja a un usuario estándar crearla, usa la clave `Run` de tu cuenta.
  `typedeck.exe uninstall` lo para de forma ordenada y quita las dos cosas. El archivo trae además `typedeck-cli.exe`, el mismo programa
  compilado para terminales: úsalo para `doctor`, `install` y `uninstall` desde cmd o PowerShell, así la salida aparece en su sitio y el
  código de salida es fiable. Registro: `%LOCALAPPDATA%\typedeck\typedeck.log`. Al bloquear la pantalla o suspender se sueltan las
  teclas capturadas y se vuelven a capturar al volver. El programa no está firmado (un certificado cuesta dinero): mira
  [Windows bloquea el ejecutable](docs/TROUBLESHOOTING.md#windows-blocks-the-executable).

Con `TYPEDECK_PORT=/dev/ttyACM1` (o `COM5`) indicas el puerto si la placa está en uno poco habitual. La primera pantalla del editor
te guía por el teclado que ha encontrado y su disposición.

## Qué puede hacer

- **Gestos y capas.** Pulsar, mantener y doble pulsación en cada tecla, cada uno con su acción. Hasta nueve capas con color, que se
  cambian con una tecla, de forma momentánea mientras se mantiene una tecla o solas cuando una aplicación pasa al frente.
- **Quince tipos de acción.** Aplicación (abrir, alternar, cerrar), enlace, comando, SSH, petición web, atajo, texto, secuencia,
  condición, multimedia, sistema, historial de tecleo, OBS Studio, temporizador, capa y cartel. La lista completa está en
  [docs/MACROS.md](docs/MACROS.md).
- **Graba y luego edita.** Teclea una secuencia y Typedeck la convierte en pasos, con tus pausas como pasos de espera que puedes
  ajustar, reordenar y duplicar.
- **Condiciones.** Ejecuta una lista de pasos u otra según la aplicación de delante, la capa activa, la hora, el sistema, si OBS
  está en directo o grabando, o lo que haya en el portapapeles.
- **Comparte macros en archivos.** Exporta una capa o todo; al importar se enseñan los comandos y peticiones web del archivo antes
  de añadir nada. En [examples/](examples) hay paquetes para empezar.
- **Una galería de 38 paquetes** para OBS Studio, Discord, Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, VS Code, Git,
  domótica, estudio y más, colocados en las teclas que tu teclado tiene.
- **Todo tu teclado, no un pad aparte.** Detecta el teclado (marca, modelo, datos USB) y lo dibuja con su formato real, del 100 al
  60 por ciento, ISO o ANSI. Un asistente de teclas deduce la disposición cuando el teclado no la dice.
- **Sin permisos para teclear.** Atajos, texto y teclas multimedia salen como pulsaciones USB reales de la placa: sin permiso de
  Accesibilidad en macOS y sin `xdotool` en Linux. Se compensan los modificadores remapeados y las rarezas ISO/ANSI de un Mac, y el
  texto en español usa la disposición correcta y Alt Gr en cada sistema.
- **OBS Studio sin atajos.** Escenas, empezar el directo, grabar, silenciar el micro o guardar el replay por el propio servidor
  WebSocket de OBS.
- **Editor en directo.** Cada tecla se ilumina en el dibujo al pulsarla, un cartel avisa cuando la placa deja de ver el teclado y
  la tecla que abre el editor lleva a la pestaña que ya está abierta.
- **Historial de tecleo, opcional.** Desactivado por defecto. Una página con lo que escribes como texto, totales, tus teclas más
  usadas y las últimas pulsaciones, guardado en un archivo local, con lo más antiguo borrándose solo y un interruptor que puedes
  poner en una tecla. No sale de tu ordenador.
- **Y todo lo demás.** Página de compatibilidad con comprobaciones automáticas, registro de actividad y uso por tecla, hoja de teclas
  con buscador e imprimible, diagnóstico, paleta de comandos, deshacer y rehacer, copias automáticas.

## Seguro por diseño

- Las teclas solo se retiran del teclado mientras el programa está vivo. Si se para, todas vuelven a escribir en 5 segundos con la
  placa, y al instante con la captura por software (el gancho, el filtro o el acaparamiento desaparecen con el proceso).
- El editor escucha solo en `127.0.0.1`, cada petición necesita una clave generada en cada arranque, y un archivo de macros que
  importes se valida y se te enseña antes de añadirlo. [docs/SECURITY.md](docs/SECURITY.md) tiene el modelo y [SECURITY.md](SECURITY.md)
  cómo avisar de un problema.
- Typedeck no hace ninguna conexión de red por su cuenta, no tiene telemetría y no carga nada de internet.
- Sin el historial de tecleo opcional, el programa solo ve las teclas que tienen macro (con la placa, un editor abierto también muestra las teclas que pulsas, sin guardarlas; con la captura por software no).

## Rendimiento

| Qué | Resultado |
|---|---|
| Reenviar un informe del teclado dentro de la placa | unos 0,13 ms |
| Memoria del programa en reposo | unos 11 MB, 0 % de CPU |
| Primera carga del editor | unos 100 KB, sin framework |

Los detalles y cómo medir tu equipo están en [docs/PERFORMANCE.md](docs/PERFORMANCE.md).

## Documentación

La documentación técnica está en inglés: [macros](docs/MACROS.md), [preguntas frecuentes](docs/FAQ.md),
[compatibilidad](docs/COMPATIBILITY.md), [arquitectura](docs/ARCHITECTURE.md), [protocolo serie](docs/PROTOCOL.md),
[hardware y alimentación](docs/HARDWARE.md), [seguridad](docs/SECURITY.md), [rendimiento](docs/PERFORMANCE.md) y
[problemas frecuentes](docs/TROUBLESHOOTING.md).

## Límites

Con la placa: seis teclas a la vez (protocolo de arranque, comprobado con el teclado real) y sin reenvío del ratón ni de las teclas
de sistema integradas del teclado. Detrás de un conmutador KVM el teclado puede dejar de verse tras un corte de corriente: el
firmware lo recupera solo en menos de un minuto, y [Hardware](docs/HARDWARE.md) explica cómo evitarlo alimentando la placa por
separado.

Sin la placa (captura por software en Windows):

- Mientras Typedeck corre como usuario normal, Windows no enseña al gancho las teclas que se escriben en ventanas ejecutadas como
  administrador ni en la pantalla de UAC, así que ahí las macros no funcionan. Ejecutar Typedeck como administrador lo arregla para
  las ventanas de administrador, no para UAC.
- El antitrampas de algunos juegos considera sospechoso cualquier gancho global de teclado y puede negarse a arrancar o marcarlo.
  Para esos juegos usa la placa.
- El gancho corre dentro del ordenador, así que el retraso es mayor que los 0,13 ms de la placa (microsegundos en el propio gancho,
  más la planificación del programa). Si el programa se congela o se detiene, las teclas con macro escriben su carácter hasta que
  vuelva.
- No rige el límite de seis teclas a la vez. No se capturan las teclas multimedia, las de sistema integradas del teclado ni las de
  otros teclados: solo las del informe estándar de teclado.
- Ve las teclas que tienen macro, y todas al detectar teclas o con el historial de pulsaciones activado; nada más,
  y nada sale del equipo. Mira [docs/SECURITY.md](docs/SECURITY.md).

Sin la placa (captura por software en macOS):

- macOS oculta lo que escribes en campos de contraseña (entrada segura) a todas las herramientas de teclado, así que ahí las macros
  no funcionan, y no enseña teclas a un filtro mientras la pantalla está bloqueada.
- Necesita dos permisos para el ayudante, Accesibilidad y Monitorización de entrada, y macOS puede retirar un filtro demasiado lento
  o tras la suspensión; el ayudante lo vuelve a activar solo y suelta las teclas que tenía retenidas.
- Bloq Mayús llega a un filtro como un conmutador, no como una tecla que se mantiene, así que como tecla de capa se comporta como un
  toque; con la placa es una tecla mantenida de verdad. Viene de cómo macOS la entrega y aún no se ha probado con un teclado real.
- Typedeck envía sus propias teclas con `CGEventPost` y las multimedia como eventos de sistema; Bloq Mayús se cambia por el sistema
  HID, así que no aplica el problema de la pulsación corta de la placa.
- El ayudante solo manda al programa las teclas que tienen macro, y todas al detectar teclas o con el historial de pulsaciones
  activado; nada sale del equipo.

Sin la placa (captura por software en Linux):

- Necesita leer todos los teclados y crear uno virtual, lo que en la práctica significa estar en el grupo `input`. Cualquier
  programa que corra como miembro de ese grupo puede leer los teclados: mira [docs/SECURITY.md](docs/SECURITY.md) antes de activarla
  en un equipo compartido.
- Trabaja por debajo del escritorio, así que ve también la pantalla de bloqueo y las demás consolas virtuales: una macro puede
  ejecutarse con la pantalla bloqueada. Typedeck no detecta el bloqueo.
- Mientras está activa, el programa está en el camino de cada tecla de los teclados acaparados. Si muere, el núcleo devuelve los
  teclados al instante (probado con `kill -9`), pero si está congelado y no muerto (`SIGSTOP`, un depurador) esos teclados no
  responden hasta que reanude o lo cierres con el ratón o por ssh. Typedeck ignora `SIGTSTP`, así que Ctrl+Z en el terminal que lo
  lanzó no lo congela.
- El teclado virtual sustituye a tus teclados ante el escritorio, así que los ajustes que identifican un teclado por su nombre
  (disposiciones por dispositivo en Sway, por ejemplo) deben cubrir también `Typedeck virtual keyboard`. Se dejan en paz los
  dispositivos con puntero en el mismo nodo y los nodos sin teclas de letras (multimedia, botón de encendido, tapa).
- El teclado virtual declara las teclas de teclado 1 a 255 y 0x160 a 0x2BF (multimedia y de función como `KEY_FN`), pero no los
  códigos de botón 0x100 a 0x15F ni 0x2C0 en adelante, porque declararlos hace que el núcleo y udev lo traten como un ratón o un
  mando; una tecla enviada con uno de esos códigos no se reemite. Los números de ioctl suponen la disposición genérica de Linux (x86, ARM,
  RISC-V), no MIPS ni PowerPC. El texto que la disposición no puede teclear va por el portapapeles (`wl-copy`, `xclip` o `xsel`) y
  Ctrl+V.
- Retraso: unos 60 microsegundos más en la mediana que una lectura directa (medido en una máquina virtual, mira
  [docs/PERFORMANCE.md](docs/PERFORMANCE.md)); en reposo no gasta CPU.

## Contribuir

Un informe de error con la salida de `typedeck doctor` es lo más útil que puedes mandar; las pull requests son bienvenidas.
[CONTRIBUTING.md](CONTRIBUTING.md) tiene la preparación y las reglas, y todo se comprueba con `make check`.

## Privacidad

Typedeck no abre ninguna conexión de red por su cuenta ni envía datos a ningún sitio; el editor solo escucha en `127.0.0.1`.

## Licencia

El programa, el editor y la documentación son MIT, ver [LICENSE](LICENSE). El firmware de `firmware/` se compila con la librería USB
Host Shield 2.0 y por eso es GPL-2.0, ver [NOTICE](NOTICE).
