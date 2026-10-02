# Typedeck

Convierte cualquier teclado USB en un teclado de macros. Una pequeña placa Arduino Leonardo con un USB Host Shield se
coloca entre tu teclado y el ordenador, lo reenvía todo y te deja dar a **cualquier tecla** sus propias acciones:
pulsar, mantener, doble pulsación, capas, lanzadores de apps, comandos, SSH, textos, teclas multimedia, temporizadores...
Tu teclado sigue funcionando con normalidad aunque el programa no esté en marcha.

[Read in English](README.md)

## En qué se diferencia

- **Todo tu teclado, no un pad aparte.** Detecta el teclado que enchufas (marca, modelo, datos USB, qué envía) y lo
  dibuja en el editor con su formato real: 100, 80, 75, 65 o 60 %, ISO o ANSI. Un asistente de teclas deduce la
  disposición cuando el teclado no la dice.
- **Galería con 38 paquetes en 11 categorías**, colocados sobre las teclas que de verdad tiene tu teclado: OBS Studio,
  Discord, OBS + Discord, Zoom, Meet, Teams, Slack, Photoshop, Figma, Premiere, Final Cut, VS Code, Xcode, Git, casa
  conectada, estudio y más.
- **Rápido y ligero.** El reenvío de teclas ocurre en la placa (unos 0,13 ms). El programa del ordenador es un binario de
  Go con unos 11 MB y sin CPU en reposo; el editor carga en unos 100 KB.
- **macOS, Linux y Windows.** Un binario por sistema, del mismo código. La placa es un teclado USB normal y funciona en
  cualquier sitio; el programa adapta lo que depende del sistema (aplicaciones, carteles, arranque con la sesión). La
  página de compatibilidad dice qué se ha probado dónde.
- **OBS Studio sin atajos.** Cambiar de escena, empezar el directo, grabar, silenciar el micro o guardar el replay por el
  servidor WebSocket del propio OBS: no hay teclas que asignar y funciona igual en todos los sistemas.
- **Seguro por diseño.** Las teclas solo se retiran del teclado mientras el programa está vivo. Si se para, todas vuelven
  a escribir en menos de 5 segundos.
- **Sin permisos para teclear.** Atajos, texto y teclas multimedia salen como pulsaciones USB reales de la placa, así que
  no hace falta el permiso de Accesibilidad de macOS (ni xdotool en Linux). Se compensan los modificadores remapeados y
  las peculiaridades de teclas ISO/ANSI de un Mac, y el texto en español usa la disposición correcta de cada sistema
  (Alt Gr en Windows y Linux).

## Qué necesitas

| Pieza | Notas |
|---|---|
| Arduino Leonardo (ATmega32U4) | probado con una Keystudio KS0248 |
| USB Host Shield 2.0 (MAX3421E) **con conector ICSP** | probado con un Yanmis |
| Un teclado USB | hasta 500 mA |
| macOS 12 o posterior, Linux (X11 o Wayland) o Windows 10/11 | la [compatibilidad](docs/COMPATIBILITY.md) dice qué está probado dónde |

## Primeros pasos

Se flashea la placa una vez (necesita `arduino-cli` y `avrdude`) y se ejecuta el programa en el ordenador donde está
enchufada:

```sh
make flash                  # compila y sube el firmware
make dist                   # binarios para macOS, Linux y Windows en dist/
dist/typedeck-linux-amd64 install     # arranca con tu sesión (y también: typedeck uninstall)
```

Después se abre <http://127.0.0.1:7788>. `typedeck doctor` comprueba que todo está en su sitio.

- **macOS:** `make install` compila e instala un servicio de sesión y el cartel, y deja `Typedeck.app` en `~/Applications`.
- **Linux:** copia `packaging/linux/99-typedeck.rules` a `/etc/udev/rules.d/` para que tu usuario abra la placa sin estar en
  el grupo `dialout`. Instala `xdg-utils` y `libnotify-bin`; `xdotool` (X11) o `wtype` (Wayland) solo importan si la placa
  no está conectada. Registro: `journalctl --user -u typedeck`.
- **Windows:** `typedeck.exe install` crea una tarea programada que lo arranca al iniciar sesión. El registro está en
  `%LOCALAPPDATA%\typedeck`. Windows 11 con el *Control inteligente de aplicaciones* activado bloquea los programas sin
  firmar; hasta que las versiones estén firmadas, hay que desactivarlo (Seguridad de Windows, Control de aplicaciones y
  navegador) o compilar el programa tú mismo.

Con `TYPEDECK_PORT=/dev/ttyACM1` (o `COM5`) se indica el puerto si la placa está en uno poco habitual.

## Documentación

La documentación técnica está en inglés: [compatibilidad](docs/COMPATIBILITY.md) (qué está probado y qué debería funcionar),
[arquitectura](docs/ARCHITECTURE.md), [protocolo serie](docs/PROTOCOL.md), [hardware y alimentación](docs/HARDWARE.md),
[seguridad](docs/SECURITY.md) y [problemas frecuentes](docs/TROUBLESHOOTING.md).

## Límites

Seis teclas a la vez (protocolo de arranque, comprobado con el teclado real) y sin reenvío del ratón ni de las teclas de sistema
integradas del teclado. Detrás de un conmutador KVM el teclado puede dejar de verse tras un corte de corriente: el firmware lo
recupera solo en menos de un minuto, y [Hardware](docs/HARDWARE.md) explica cómo evitarlo alimentando la placa por separado.

## Licencia

MIT, ver [LICENSE](LICENSE).
