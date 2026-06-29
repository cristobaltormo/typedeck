#define protected public  // BootKeyboard keeps its report in a protected member we need to forward as is
#include <HID-Project.h>
#undef protected
#include <hidcomposite.h>
#include <usbhub.h>
#include <SPI.h>

#define FW_VERSION "4"
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
bool kbdPresent = false;
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
    Serial.print(F(" media_us=")); Serial.print(statN ? statSum / statN : 0);
    Serial.print(F(" max_us=")); Serial.print(statMax);
    Serial.print(F(" hid_listo=")); Serial.print(Hid.isReady());
    Serial.print(F(" estado=0x")); Serial.print(Usb.getUsbTaskState(), HEX);
    Serial.print(F(" captura=")); Serial.println(capActive());
    statN = statSum = statMax = 0; return;
  }
  if (!strncmp(cmd, "WATCH ", 6)) { watch = atoi(cmd + 6) != 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "DBG ", 4)) { debugRaw = atoi(cmd + 4) != 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "LEDS ", 5)) { ledOverride = atoi(cmd + 5); ledOverrideUntil = millis() + 4000; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "LAYER ", 6)) { blinkLeft = atoi(cmd + 6) + 1; blinkNext = 0; Serial.println(F("OK")); return; }
  if (!strncmp(cmd, "L ", 2)) { digitalWrite(LED_BUILTIN, atoi(cmd + 2) ? HIGH : LOW); Serial.println(F("OK")); return; }
  Serial.println(F("?"));
}

void cycleVbus() { Usb.vbusPower(vbus_off); delay(600); Usb.vbusPower(vbus_on); }

char line[160]; uint8_t lineN = 0;

void setup() {
  pinMode(LED_BUILTIN, OUTPUT);
  Serial.begin(115200);
  BootKeyboard.begin();
  Consumer.begin();
  if (Usb.Init() == -1) { while (1) { digitalWrite(LED_BUILTIN, !digitalRead(LED_BUILTIN)); delay(150); } }
  cycleVbus();
  lastDeviceSeen = millis();
}

void loop() {
  Usb.Task();

  bool running = Usb.getUsbTaskState() == USB_STATE_RUNNING;
  if (running) lastDeviceSeen = millis();
  else if (millis() - lastDeviceSeen > 15000) { cycleVbus(); lastDeviceSeen = millis(); }
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
