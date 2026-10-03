#define protected public  // BootKeyboard keeps its report in a protected member we need to forward as is
#include <HID-Project.h>
#undef protected
#include <hidcomposite.h>
#include <usbhub.h>
#include <SPI.h>

#include <avr/wdt.h>
#include <EEPROM.h>
#define FW_VERSION "13"
#define HB_TIMEOUT_MS 5000UL

USB Usb;
USBHub Hub(&Usb);

uint8_t capMask[32];
uint8_t prevDown[6]; uint8_t prevN = 0;
uint8_t physMods = 0, physKeys[6];
uint8_t injMods = 0, injKeys[6]; uint8_t injN = 0;
unsigned long lastHb = 0;
bool debugRaw = false, watch = false;
uint8_t lastRaw[6];
uint8_t lastLeds = 0xFF, ledOverride = 0xFF;
unsigned long ledOverrideUntil = 0;
bool hidWasReady = false;
unsigned long lastDeviceSeen = 0;
// Restart when the keyboard does not show up. After a power cut (a KVM switch) the shield can stop seeing the keyboard, and
// neither resetting the shield chip nor an immediate MCU reset fixes it; going through the bootloader (as when uploading a
// sketch) does. Cutting the keyboard port power from the firmware does nothing on the reference shield: it keeps the keyboard
// connected with VBUS "off". At most 3 times in a row unless a keyboard was ever seen; the counters survive the restart.
#define SOFT_MAGIC 0xA55A
#define EE_SEEN 0
uint16_t softMagic __attribute__((section(".noinit")));
uint8_t softResets __attribute__((section(".noinit")));
unsigned long lastRunningMs = 0;
bool kbdPresent = false;
bool dark = true;
bool allKeys = false;
uint8_t prevPlain[6]; uint8_t prevPlainN = 0, prevAllMods = 0;
bool seenSinceBoot = false;
#define EE_COLD 1
#define EE_RECOV 2
#define EE_FIRST 3
#define EE_SLOW 5
#define EE_INIT 6
bool wasRecovery = false;
unsigned long statN = 0, statSum = 0, statMax = 0;
uint8_t blinkLeft = 0; bool blinkOn = false; unsigned long blinkNext = 0;

static inline bool capActive() { return lastHb && (millis() - lastHb) < HB_TIMEOUT_MS; }
static inline bool isCaptured(uint8_t k) { return capMask[k >> 3] & (1 << (k & 7)); }

static void hex2(uint8_t v) { const char *h = "0123456789ABCDEF"; Serial.write(h[v >> 4]); Serial.write(h[v & 15]); }

void sendReport() {
  if (!USBDevice.configured()) return;
  uint8_t out[6] = {0, 0, 0, 0, 0, 0}, n = 0;
  for (uint8_t i = 0; i < 6 && n < 6; i++) if (physKeys[i]) out[n++] = physKeys[i];
  for (uint8_t i = 0; i < injN && n < 6; i++) {
    bool dup = false;
    for (uint8_t j = 0; j < n; j++) if (out[j] == injKeys[i]) dup = true;
    if (!dup) out[n++] = injKeys[i];
  }
  BootKeyboard._keyReport.modifiers = physMods | injMods;
  for (uint8_t i = 0; i < 6; i++) BootKeyboard._keyReport.keycodes[i] = (KeyboardKeycode)out[i];
  BootKeyboard.send();
}

static void pr(char kind, uint8_t usage) { Serial.write(kind); Serial.write(' '); hex2(usage); Serial.write('\n'); }

static void reportPlainKeys(const uint8_t *buf, bool cap) {
  uint8_t chg = buf[0] ^ prevAllMods;
  for (uint8_t b = 0; b < 8; b++) if (chg & (1 << b)) pr((buf[0] & (1 << b)) ? 'P' : 'R', 0xE0 + b);
  prevAllMods = buf[0];
  uint8_t cur[6], n = 0;
  for (uint8_t i = 0; i < 6; i++) {
    uint8_t k = buf[2 + i];
    if (k > 3 && !(cap && isCaptured(k))) cur[n++] = k;
  }
  for (uint8_t i = 0; i < n; i++) {
    bool had = false;
    for (uint8_t j = 0; j < prevPlainN; j++) if (prevPlain[j] == cur[i]) had = true;
    if (!had) pr('P', cur[i]);
  }
  for (uint8_t j = 0; j < prevPlainN; j++) {
    bool still = false;
    for (uint8_t i = 0; i < n; i++) if (cur[i] == prevPlain[j]) still = true;
    if (!still) pr('R', prevPlain[j]);
  }
  for (uint8_t i = 0; i < n; i++) prevPlain[i] = cur[i];
  prevPlainN = n;
}

void processReport(const uint8_t *buf, bool quiet) {
  unsigned long t0 = micros();
  physMods = buf[0];
  if (watch) {
    for (uint8_t i = 0; i < 6; i++) {
      uint8_t k = buf[2 + i]; bool had = false;
      if (k <= 3) continue;
      for (uint8_t j = 0; j < 6; j++) if (lastRaw[j] == k) had = true;
      if (!had) { Serial.write('W'); Serial.write(' '); hex2(k); Serial.write('\n'); }
    }
    for (uint8_t i = 0; i < 6; i++) lastRaw[i] = buf[2 + i];
  }
  bool cap = capActive();
  uint8_t nowDown[6], nowN = 0;
  for (uint8_t i = 0; i < 6; i++) {
    uint8_t k = buf[2 + i];
    if (k == 0) { physKeys[i] = 0; continue; }
    if (cap && k > 3 && isCaptured(k)) { physKeys[i] = 0; nowDown[nowN++] = k; }  // 1..3 are rollover errors
    else physKeys[i] = k;
  }
  if (cap) {
    for (uint8_t i = 0; i < nowN; i++) {
      bool was = false;
      for (uint8_t j = 0; j < prevN; j++) if (prevDown[j] == nowDown[i]) was = true;
      if (!was) { Serial.write('D'); Serial.write(' '); hex2(nowDown[i]); Serial.write(' '); hex2(physMods); Serial.write('\n'); }
    }
    for (uint8_t j = 0; j < prevN; j++) {
      bool still = false;
      for (uint8_t i = 0; i < nowN; i++) if (nowDown[i] == prevDown[j]) still = true;
      if (!still) { Serial.write('U'); Serial.write(' '); hex2(prevDown[j]); Serial.write('\n'); }
    }
    for (uint8_t i = 0; i < nowN; i++) prevDown[i] = nowDown[i];
    prevN = nowN;
  } else prevN = 0;
  if (allKeys) reportPlainKeys(buf, cap);
  if (!quiet) sendReport();
  unsigned long dt = micros() - t0;
  statN++; statSum += dt; if (dt > statMax) statMax = dt;
}

uint16_t lastConsumer = 0;
void consumerReport(uint16_t usage) {
  if (usage == lastConsumer) return;
  if (lastConsumer) Consumer.release((ConsumerKeycode)lastConsumer);
  if (usage) Consumer.press((ConsumerKeycode)usage);
  lastConsumer = usage;
}

// Listens to both keyboard interfaces: 1 (ep 1) is the boot keyboard and 2 (ep 2) carries the reports with an id
// (media keys, system control, full keyboard, mouse).
class DeckHid : public HIDComposite {
  uint8_t bootIface = 255;
 public:
  DeckHid(USB *p) : HIDComposite(p) {}
 protected:
  bool SelectInterface(uint8_t iface, uint8_t proto) override { if (proto == 1 && bootIface == 255) bootIface = iface; return true; }
  uint8_t OnInitSuccessful() override {
    if (bootIface != 255) { SetProtocol(bootIface, 0); SetIdle(bootIface, 0, 0); }
    bootIface = 255;
    return 0;
  }
  void ParseHIDData(USBHID *hid, uint8_t ep, bool is_rpt_id, uint8_t len, uint8_t *buf) override {
    if (debugRaw) {
      Serial.print(F("EP")); Serial.print(ep); Serial.print(F(" RPT")); Serial.print(len); Serial.write(':');
      for (uint8_t i = 0; i < len && i < 20; i++) { Serial.write(' '); hex2(buf[i]); }
      Serial.write('\n');
    }
    if (ep == 1) { if (len == 8) processReport(buf, false); return; }
    if (ep == 2 && len == 3 && buf[0] == 2) consumerReport(buf[1] | ((uint16_t)buf[2] << 8));
  }
};
DeckHid Hid(&Usb);

struct HexDump : public USBReadParser {
  uint16_t col = 0;
  void Parse(const uint16_t len, const uint8_t *pbuf, const uint16_t &offset) {
    for (uint16_t i = 0; i < len; i++) { hex2(pbuf[i]); if (++col % 32 == 0) Serial.write('\n'); else Serial.write(' '); }
  }
};

struct CfgScan : public USBReadParser {
  uint8_t pos = 0, dlen = 0, dtype = 0, iface = 0, eps = 0;
  uint16_t hidLen[4] = {0, 0, 0, 0}; uint8_t lo = 0;
  uint8_t ifaces = 0;
  void Parse(const uint16_t len, const uint8_t *p, const uint16_t &offset) {
    for (uint16_t i = 0; i < len; i++) {
      uint8_t b = p[i];
      if (pos == 0) dlen = b; else if (pos == 1) dtype = b;
      else if (dtype == 0x04 && pos == 2) { iface = b; if (b + 1 > ifaces) ifaces = b + 1; }
      else if (dtype == 0x21 && pos == 7) lo = b;
      else if (dtype == 0x21 && pos == 8 && iface < 4) hidLen[iface] = lo | (b << 8);
      if (++pos >= dlen) pos = 0;
    }
  }
};

static void printStr(uint8_t addr, uint8_t idx, uint16_t lang) {
  Serial.write('"');
  if (idx) {
    uint8_t buf[66];
    if (Usb.getStrDescr(addr, 0, sizeof(buf), idx, lang, buf) == 0) {
      uint8_t n = buf[0] > sizeof(buf) ? sizeof(buf) : buf[0];
      for (uint8_t i = 2; i + 1 < n; i += 2) {
        char c = buf[i + 1] == 0 && buf[i] >= 32 && buf[i] < 127 ? buf[i] : '?';
        if (c == '"') c = '\'';
        Serial.write(c);
      }
    }
  }
  Serial.write('"');
}

void cmdInfo() {
  if (Usb.getUsbTaskState() != USB_STATE_RUNNING) { Serial.println(F("NODEVICE")); return; }
  uint8_t addr = Hid.GetAddress();
  USB_DEVICE_DESCRIPTOR d;
  if (Usb.getDevDescr(addr, 0, sizeof(d), (uint8_t *)&d) != 0) { Serial.println(F("ERR descriptor")); return; }
  uint8_t sb[8]; uint16_t lang = 0x0409;
  if (Usb.getStrDescr(addr, 0, 4, 0, 0, sb) == 0 && sb[0] >= 4) lang = sb[2] | (sb[3] << 8);
  Serial.print(F("vid=")); hex2(d.idVendor >> 8); hex2(d.idVendor);
  Serial.print(F(" pid=")); hex2(d.idProduct >> 8); hex2(d.idProduct);
  Serial.print(F(" bcd=")); hex2(d.bcdDevice >> 8); hex2(d.bcdDevice);
  Serial.print(F(" usb=")); hex2(d.bcdUSB >> 8); hex2(d.bcdUSB);
  Serial.print(F(" class=")); hex2(d.bDeviceClass);
  Serial.print(F(" ep0=")); Serial.print(d.bMaxPacketSize0);
  Serial.print(F(" mfr=")); printStr(addr, d.iManufacturer, lang);
  Serial.print(F(" prod=")); printStr(addr, d.iProduct, lang);
  Serial.print(F(" serial=")); printStr(addr, d.iSerialNumber, lang);
  uint8_t cfg[9];
  if (Usb.getConfDescr(addr, 0, 9, 0, cfg) == 0) {
    Serial.print(F(" cfglen=")); Serial.print(cfg[2] | (cfg[3] << 8));
    Serial.print(F(" ifaces=")); Serial.print(cfg[4]);
    Serial.print(F(" power=")); Serial.print(cfg[8] * 2);
    CfgScan scan;
    uint8_t buf[32];
    uint16_t total = cfg[2] | (cfg[3] << 8);
    if (total > 255) total = 255;
    Usb.ctrlReq(addr, 0, 0x80, USB_REQUEST_GET_DESCRIPTOR, 0, 0x02, 0, total, sizeof(buf), buf, &scan);
    for (uint8_t i = 0; i < 4 && i < cfg[4]; i++) { Serial.print(F(" hidlen")); Serial.print(i); Serial.write('='); Serial.print(scan.hidLen[i]); }
  }
  Serial.println();
  Serial.println(F("END"));
}

void cmdRdesc(uint8_t iface, uint16_t len) {
  if (Usb.getUsbTaskState() != USB_STATE_RUNNING) { Serial.println(F("NODEVICE")); return; }
  if (len > 512) len = 512;
  HexDump dump; uint8_t buf[32];
  uint8_t rc = Usb.ctrlReq(Hid.GetAddress(), 0, 0x81, USB_REQUEST_GET_DESCRIPTOR, 0, 0x22, iface, len, sizeof(buf), buf, &dump);
  if (dump.col % 32) Serial.write('\n');
  if (rc) { Serial.print(F("ERR ")); Serial.println(rc); }
  Serial.println(F("END"));
}

static long readVccMv() {
  ADMUX = _BV(REFS0) | 0x1E;
  delayMicroseconds(400);
  ADCSRA |= _BV(ADSC);
  while (bit_is_set(ADCSRA, ADSC)) {}
  return 1125300L / ADC;
}
static int freeRam() { extern int __heap_start, *__brkval; int v; return (int)&v - (__brkval == 0 ? (int)&__heap_start : (int)__brkval); }

void cmdSys() {
#if defined(ARDUINO_AVR_LEONARDO)
  const __FlashStringHelper *board = F("leonardo");
#elif defined(ARDUINO_AVR_MICRO)
  const __FlashStringHelper *board = F("micro");
#else
  const __FlashStringHelper *board = F("atmega32u4");
#endif
  Serial.print(F("mcu=atmega32u4 f_cpu=")); Serial.print((unsigned long)F_CPU);
  Serial.print(F(" board=")); Serial.print(board);
  Serial.print(F(" fw=" FW_VERSION " vcc_mv=")); Serial.print(readVccMv());
  Serial.print(F(" free_ram=")); Serial.print(freeRam());
  Serial.print(F(" max3421e_rev=")); hex2(Usb.regRd(rREVISION));
  Serial.print(F(" uptime_s=")); Serial.println(millis() / 1000UL);
}

static uint8_t hexv(char c) { return c >= '0' && c <= '9' ? c - '0' : c >= 'a' && c <= 'f' ? c - 'a' + 10 : c >= 'A' && c <= 'F' ? c - 'A' + 10 : 0; }
static uint16_t parseHex(const char *&s) {
  while (*s == ' ') s++;
  uint16_t v = 0;
  while ((*s >= '0' && *s <= '9') || (*s >= 'a' && *s <= 'f') || (*s >= 'A' && *s <= 'F')) v = (v << 4) | hexv(*s++);
  return v;
}

void tapKey(uint8_t mods, uint8_t usage) {
  injMods = mods; injKeys[0] = usage; injN = usage ? 1 : 0;
  sendReport(); delay(7);
  injMods = 0; injN = 0;
  sendReport(); delay(5);
}

void bootloaderReset();

void handle(char *cmd) {
  const char *p = cmd;
  if (!strcmp(cmd, "HB")) { lastHb = millis(); if (!lastHb) lastHb = 1; return; }
  if (!strcmp(cmd, "WHO")) { Serial.println(F("TYPEDECK-FW " FW_VERSION)); return; }
  if (!strcmp(cmd, "PING")) { Serial.println(F("PONG")); return; }
  if (!strcmp(cmd, "SYS")) { cmdSys(); return; }
  if (!strncmp(cmd, "MASK ", 5)) {
    p += 5; if (strlen(p) < 64) { Serial.println(F("ERR mask")); return; }
    for (uint8_t i = 0; i < 32; i++) capMask[i] = (hexv(p[i * 2]) << 4) | hexv(p[i * 2 + 1]);
    lastHb = millis();
    Serial.println(F("OK")); return;
  }
  if (!strncmp(cmd, "KEY ", 4)) { p += 4; uint8_t m = parseHex(p); uint8_t u = parseHex(p); tapKey(m, u); Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "CONS ", 5)) { p += 5; uint16_t u = parseHex(p); Consumer.write((ConsumerKeycode)u); Serial.println(F("OK")); return; }
  if (!strcmp(cmd, "INFO")) { cmdInfo(); return; }
  if (!strncmp(cmd, "RDESC ", 6)) { p += 6; uint8_t i = atoi(p); while (*p && *p != ' ') p++; cmdRdesc(i, atoi(p)); return; }
  if (!strncmp(cmd, "SIMQ ", 5)) {
    p += 5; if (strlen(p) < 16) { Serial.println(F("ERR sim")); return; }
    uint8_t b[8]; for (uint8_t i = 0; i < 8; i++) b[i] = (hexv(p[i * 2]) << 4) | hexv(p[i * 2 + 1]);
    processReport(b, true); Serial.println(F("OK")); return;
  }
  if (!strcmp(cmd, "STATS")) {
    Serial.print(F("n=")); Serial.print(statN);
    Serial.print(F(" avg_us=")); Serial.print(statN ? statSum / statN : 0);
    Serial.print(F(" max_us=")); Serial.print(statMax);
    Serial.print(F(" hid_ready=")); Serial.print(Hid.isReady());
    Serial.print(F(" state=0x")); Serial.print(Usb.getUsbTaskState(), HEX);
    Serial.print(F(" capture=")); Serial.println(capActive());
    statN = statSum = statMax = 0; return;
  }
  if (!strncmp(cmd, "VBUS ", 5)) { Usb.vbusPower(atoi(cmd + 5) ? vbus_on : vbus_off); Serial.println(F("OK")); return; }
  if (!strcmp(cmd, "REBOOT")) { Serial.println(F("OK")); Serial.flush(); delay(50); bootloaderReset(); }
  if (!strncmp(cmd, "KEYS ", 5)) { allKeys = atoi(cmd + 5) != 0; prevPlainN = 0; prevAllMods = 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "DARK ", 5)) { dark = atoi(cmd + 5) != 0; Serial.println(F("OK")); return; }
  if (!strcmp(cmd, "BOOTLOG")) {
    Serial.print(F("cold=")); Serial.print(EEPROM.read(EE_COLD));
    Serial.print(F(" recoveries=")); Serial.print(EEPROM.read(EE_RECOV));
    Serial.print(F(" slow=")); Serial.print(EEPROM.read(EE_SLOW));
    uint16_t f; EEPROM.get(EE_FIRST, f);
    Serial.print(F(" first_seen_ds=")); Serial.print(f == 0xFFFF ? -1 : (long)f);
    Serial.print(F(" this_boot=")); Serial.println(wasRecovery ? F("recovery") : F("cold"));
    return;
  }
  if (!strcmp(cmd, "BUS")) {
    Serial.print(F("hrsl=0x")); Serial.print(Usb.regRd(rHRSL), HEX);
    Serial.print(F(" state=0x")); Serial.print(Usb.getUsbTaskState(), HEX);
    Serial.print(F(" restarts=")); Serial.print(softResets);
    Serial.print(F(" no_keyboard_s=")); Serial.println((millis() - lastDeviceSeen) / 1000);
    return;
  }
  if (!strncmp(cmd, "WATCH ", 6)) { watch = atoi(cmd + 6) != 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "DBG ", 4)) { debugRaw = atoi(cmd + 4) != 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "LEDS ", 5)) { ledOverride = atoi(cmd + 5); ledOverrideUntil = millis() + 4000; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "LAYER ", 6)) { blinkLeft = atoi(cmd + 6) + 1; blinkNext = 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "L ", 2)) { digitalWrite(LED_BUILTIN, atoi(cmd + 2) ? HIGH : LOW); Serial.println(F("OK")); return; }
  Serial.println(F("?"));
}

char line[160]; uint8_t lineN = 0;

// Restart through the bootloader (what the Arduino core does to upload a sketch): the MCU sits in the bootloader for about 8 s
// and then starts the sketch. An immediate restart does NOT recover the keyboard; this one does.
void bootloaderReset() {
  *(volatile uint16_t *)0x0800 = 0x7777;
  wdt_enable(WDTO_120MS);
  while (1) {}
}

void setup() {
  MCUSR = 0;
  wdt_disable();
  bool coldStart = softMagic != SOFT_MAGIC;
  if (coldStart) softResets = 0;
  wasRecovery = !coldStart;
  softMagic = 0;
  if (EEPROM.read(EE_INIT) != 0x11) {
    EEPROM.write(EE_COLD, 0); EEPROM.write(EE_RECOV, 0); EEPROM.write(EE_SLOW, 0); EEPROM.put(EE_FIRST, (uint16_t)0xFFFF); EEPROM.write(EE_INIT, 0x11);
  }
  if (coldStart && EEPROM.read(EE_COLD) < 255) EEPROM.write(EE_COLD, EEPROM.read(EE_COLD) + 1);
  if (wasRecovery && EEPROM.read(EE_RECOV) < 255) EEPROM.write(EE_RECOV, EEPROM.read(EE_RECOV) + 1);
  pinMode(LED_BUILTIN, OUTPUT);
  Serial.begin(115200);
  BootKeyboard.begin();
  Consumer.begin();
  for (uint8_t i = 0; Usb.Init() == -1; i++) {
    digitalWrite(LED_BUILTIN, !digitalRead(LED_BUILTIN));
    delay(250);
    if (i >= 8) bootloaderReset();
  }
  lastDeviceSeen = millis();
  lastRunningMs = millis();
}

void loop() {
  Usb.Task();

  bool running = Usb.getUsbTaskState() == USB_STATE_RUNNING;
  if (dark) { DDRB &= ~_BV(0); DDRD &= ~_BV(5); }  // RX (PB0) and TX (PD5) LEDs as inputs: the USB core keeps toggling them
  if (running) {
    lastDeviceSeen = millis(); lastRunningMs = millis(); softResets = 0;
    if (!seenSinceBoot) {
      seenSinceBoot = true;
      uint16_t ds = millis() / 100UL;
      uint16_t old; EEPROM.get(EE_FIRST, old);
      if (old != ds) EEPROM.put(EE_FIRST, ds);
      if (ds > 50 && EEPROM.read(EE_SLOW) < 255) EEPROM.write(EE_SLOW, EEPROM.read(EE_SLOW) + 1);
    }
    if (EEPROM.read(EE_SEEN) != 0xA5) EEPROM.write(EE_SEEN, 0xA5);
  }
  else if ((softResets < 3 || EEPROM.read(EE_SEEN) == 0xA5) && millis() - lastRunningMs > 9000) {
    softResets++; softMagic = SOFT_MAGIC;
    Serial.println(F("RESET no keyboard")); Serial.flush();
    bootloaderReset();
  }
  if (running != kbdPresent) { kbdPresent = running; Serial.println(running ? F("K 1") : F("K 0")); }

  bool ready = Hid.isReady();
  uint8_t leds = BootKeyboard.getLeds();
  if (millis() < ledOverrideUntil) leds = ledOverride;
  if (ready && (leds != lastLeds || !hidWasReady)) { Hid.SetReport(0, 0, 2, 0, 1, &leds); lastLeds = leds; }
  hidWasReady = ready;
  if (!ready) lastLeds = 0xFF;

  while (Serial.available()) {
    char c = Serial.read();
    if (c == '\n') { line[lineN] = 0; if (lineN) handle(line); lineN = 0; }
    else if (c != '\r' && lineN < sizeof(line) - 1) line[lineN++] = c;
  }

  if (blinkLeft && millis() >= blinkNext) {
    blinkOn = !blinkOn;
    digitalWrite(LED_BUILTIN, blinkOn ? HIGH : LOW);
    if (!blinkOn) blinkLeft--;
    blinkNext = millis() + 150;
  }
}
