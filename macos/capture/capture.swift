import Cocoa
import Carbon.HIToolbox
import IOKit
import IOKit.hidsystem

let injectTag: Int64 = 0x5444434B
let childMarker = "TYPEDECK_CAPTURE_CHILD"

let modifierDeviceBit: [Int: UInt64] = [
    59: 0x1, 56: 0x2, 60: 0x4, 55: 0x8, 54: 0x10, 58: 0x20, 61: 0x40, 62: 0x2000,
]
let modifierGroups: [(device: UInt64, generic: UInt64)] = [
    (0x1 | 0x2000, 0x40000), (0x2 | 0x4, 0x20000), (0x20 | 0x40, 0x80000), (0x8 | 0x10, 0x100000),
]
let capsLockKey = 57

var sink: ((String) -> Void)?

func emit(_ line: String) {
    if let sink = sink {
        sink(line)
        return
    }
    print(line)
    fflush(stdout)
}

func hex(_ v: UInt64) -> String { String(v, radix: 16) }

func layoutName(_ type: Int) -> String {
    switch Int(KBGetLayoutType(Int16(truncatingIfNeeded: type))) {
    case Int(kKeyboardISO): return "iso"
    case Int(kKeyboardJIS): return "jis"
    default: return "ansi"
    }
}

func keyboardLayout() -> String { layoutName(Int(LMGetKbdType())) }

func trusted(prompt: Bool) -> (accessibility: Bool, input: Bool) {
    let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: prompt] as CFDictionary
    let ax = AXIsProcessTrustedWithOptions(options)
    let input = prompt ? CGRequestListenEventAccess() : CGPreflightListenEventAccess()
    return (ax, input)
}

func disclaimedRespawn() {
    if ProcessInfo.processInfo.environment[childMarker] != nil { return }
    guard let sym = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "responsibility_spawnattrs_setdisclaim") else { return }
    typealias Disclaim = @convention(c) (UnsafeMutablePointer<posix_spawnattr_t?>, Int32) -> Int32
    var attr: posix_spawnattr_t? = nil
    posix_spawnattr_init(&attr)
    _ = unsafeBitCast(sym, to: Disclaim.self)(&attr, 1)
    var env = ProcessInfo.processInfo.environment
    env[childMarker] = "1"
    let argv = CommandLine.arguments.map { strdup($0) } + [nil]
    let envp = env.map { strdup("\($0.key)=\($0.value)") } + [nil]
    var pid: pid_t = 0
    guard posix_spawn(&pid, CommandLine.arguments[0], nil, &attr, argv, envp) == 0 else { return }
    signal(SIGTERM, SIG_IGN)
    var status: Int32 = 0
    while waitpid(pid, &status, 0) < 0 && errno == EINTR {}
    exit((status & 0x7f) == 0 ? (status >> 8) & 0xff : 1)
}

final class Capture {
    var mask = [Bool](repeating: false, count: 128)
    var reportAll = false
    var active = true
    var held = [UInt8](repeating: 0, count: 128)
    var hidden: UInt64 = 0
    var tap: CFMachPort?
    var lastLayout = ""
    var count: UInt64 = 0
    var totalNS: UInt64 = 0
    var maxNS: UInt64 = 0
    var lastReport: UInt64 = 0
    var ready = false
    let source = CGEventSource(stateID: .hidSystemState)
    var resetCapsLock: () -> Void = { setCapsLock(false) }

    func start() -> Bool {
        let wanted: [CGEventType] = [.keyDown, .keyUp, .flagsChanged]
        let eventMask = wanted.reduce(CGEventMask(0)) { $0 | (1 << $1.rawValue) }
        let callback: CGEventTapCallBack = { _, type, event, refcon in
            let me = Unmanaged<Capture>.fromOpaque(refcon!).takeUnretainedValue()
            return me.handle(type, event)
        }
        tap = CGEvent.tapCreate(tap: .cgSessionEventTap, place: .headInsertEventTap, options: .defaultTap,
                                eventsOfInterest: eventMask, callback: callback,
                                userInfo: Unmanaged.passUnretained(self).toOpaque())
        guard let tap = tap else { return false }
        let src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0)
        CFRunLoopAddSource(CFRunLoopGetMain(), src, .commonModes)
        CGEvent.tapEnable(tap: tap, enable: true)
        return true
    }

    func handle(_ type: CGEventType, _ event: CGEvent) -> Unmanaged<CGEvent>? {
        let pass = Unmanaged.passUnretained(event)
        switch type {
        case .tapDisabledByTimeout, .tapDisabledByUserInput:
            recover(type == .tapDisabledByTimeout ? "timeout" : "user")
            return pass
        case .keyDown, .keyUp, .flagsChanged:
            break
        default:
            return pass
        }
        if event.getIntegerValueField(.eventSourceUserData) == injectTag { return pass }
        let start = DispatchTime.now().uptimeNanoseconds
        let swallow = decide(type, event)
        record(DispatchTime.now().uptimeNanoseconds - start)
        if swallow { return nil }
        if hidden != 0 { event.flags = CGEventFlags(rawValue: visible(event.flags.rawValue)) }
        return pass
    }

    func decide(_ type: CGEventType, _ event: CGEvent) -> Bool {
        guard active else { return false }
        noteKeyboardType(event)
        let kc = Int(event.getIntegerValueField(.keyboardEventKeycode))
        guard kc < 128 else { return false }
        let flags = event.flags.rawValue
        switch type {
        case .keyDown:
            return press(kc, flags)
        case .keyUp:
            return release(kc)
        default:
            return modifier(kc, flags)
        }
    }

    func noteKeyboardType(_ event: CGEvent) {
        let layout = layoutName(Int(event.getIntegerValueField(.keyboardEventKeyboardType)))
        if layout != lastLayout {
            lastLayout = layout
            emit("KBTYPE \(layout)")
        }
    }

    func press(_ kc: Int, _ flags: UInt64) -> Bool {
        let st = held[kc]
        if st & 1 != 0 { return st & 2 != 0 }
        if mask[kc] {
            held[kc] = 3
            emit("D \(kc) \(hex(flags))")
            return true
        }
        if reportAll {
            held[kc] = 5
            emit("P \(kc)")
        } else {
            held[kc] = 1
        }
        return false
    }

    func release(_ kc: Int) -> Bool {
        let st = held[kc]
        if st & 1 == 0 { return false }
        held[kc] = 0
        if st & 2 != 0 {
            emit("U \(kc)")
            return true
        }
        if st & 4 != 0 { emit("R \(kc)") }
        return false
    }

    func modifier(_ kc: Int, _ flags: UInt64) -> Bool {
        if kc == capsLockKey { return capsToggle(flags) }
        guard let bit = modifierDeviceBit[kc] else { return false }
        if flags & bit != 0 {
            let swallow = press(kc, flags)
            if swallow { hidden |= bit }
            return swallow
        }
        hidden &= ~bit
        return release(kc)
    }

    func capsToggle(_ flags: UInt64) -> Bool {
        if mask[capsLockKey] {
            emit("D \(capsLockKey) \(hex(flags))")
            emit("U \(capsLockKey)")
            DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(15)) { self.resetCapsLock() }
            return true
        }
        if reportAll {
            emit("P \(capsLockKey)")
            emit("R \(capsLockKey)")
        }
        return false
    }

    func visible(_ flags: UInt64) -> UInt64 {
        var f = flags & ~hidden
        for g in modifierGroups where f & g.device == 0 { f &= ~g.generic }
        return f
    }

    func releaseAll() {
        for kc in 0..<128 where held[kc] != 0 {
            let st = held[kc]
            held[kc] = 0
            if st & 2 != 0 { emit("U \(kc)") } else if st & 4 != 0 { emit("R \(kc)") }
        }
        hidden = 0
    }

    func recover(_ reason: String) {
        releaseAll()
        emit("TAPOFF \(reason)")
        reenable()
    }

    func reenable() {
        guard let tap = tap else { return }
        CGEvent.tapEnable(tap: tap, enable: true)
        if CGEvent.tapIsEnabled(tap: tap) {
            if ready { emit("TAPON") }
            return
        }
        emit("ERR tap_disabled the system keeps the keyboard tap disabled")
        exit(4)
    }

    func watchTap() {
        Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { _ in
            guard let tap = self.tap, !CGEvent.tapIsEnabled(tap: tap) else { return }
            self.recover("silent")
        }
    }

    func watchSystem() {
        let center = NSWorkspace.shared.notificationCenter
        center.addObserver(forName: NSWorkspace.willSleepNotification, object: nil, queue: .main) { _ in self.releaseAll() }
        center.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { _ in self.reenable() }
        let distributed = DistributedNotificationCenter.default()
        distributed.addObserver(forName: .init("com.apple.screenIsLocked"), object: nil, queue: .main) { _ in self.releaseAll() }
        distributed.addObserver(forName: .init("com.apple.screenIsUnlocked"), object: nil, queue: .main) { _ in self.reenable() }
    }

    func record(_ ns: UInt64) {
        count += 1
        totalNS += ns
        maxNS = max(maxNS, ns)
    }

    func reportLatency() {
        Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { _ in
            guard self.count != self.lastReport else { return }
            self.lastReport = self.count
            emit("LAT \(self.count) \(self.totalNS / self.count / 1000) \(self.maxNS / 1000)")
        }
    }

    func configure(_ parts: [Substring]) {
        guard parts.count == 4, parts[1].count == 32 else { return }
        let digits = Array(parts[1])
        for i in 0..<16 {
            let byte = UInt8(String(digits[i * 2...i * 2 + 1]), radix: 16) ?? 0
            for b in 0..<8 { mask[i * 8 + b] = byte >> b & 1 == 1 }
        }
        reportAll = parts[2] == "1"
        let nowActive = parts[3] == "1"
        if active && !nowActive { held = [UInt8](repeating: 0, count: 128); hidden = 0 }
        active = nowActive
    }

    func key(_ parts: [Substring]) {
        guard parts.count == 4, let kc = CGKeyCode(parts[2]), let flags = UInt64(parts[3], radix: 16) else { return }
        let down = parts[1] == "d"
        let isModifier = modifierDeviceBit[Int(kc)] != nil
        guard let event = CGEvent(keyboardEventSource: source, virtualKey: kc, keyDown: down) else { return }
        if isModifier { event.type = .flagsChanged }
        event.flags = CGEventFlags(rawValue: flags)
        post(event)
    }

    func post(_ event: CGEvent) {
        event.setIntegerValueField(.eventSourceUserData, value: injectTag)
        event.post(tap: .cghidEventTap)
    }

    func media(_ parts: [Substring]) {
        guard parts.count == 2, let key = Int(parts[1]) else { return }
        for down in [true, false] {
            let flags = down ? 0xa00 : 0xb00
            let event = NSEvent.otherEvent(with: .systemDefined, location: .zero,
                                           modifierFlags: NSEvent.ModifierFlags(rawValue: UInt(flags)), timestamp: 0,
                                           windowNumber: 0, context: nil, subtype: 8,
                                           data1: (key << 16) | flags, data2: -1)
            if let cg = event?.cgEvent { post(cg) }
        }
    }

    func text(_ parts: [Substring]) {
        guard parts.count == 2, let data = Data(hexString: String(parts[1])), let s = String(data: data, encoding: .utf8) else { return }
        var chunk: [UInt16] = []
        for scalar in s.unicodeScalars {
            let units = Array(String(scalar).utf16)
            if chunk.count + units.count > 16 { typeChunk(chunk); chunk = [] }
            chunk += units
        }
        if !chunk.isEmpty { typeChunk(chunk) }
    }

    func typeChunk(_ units: [UInt16]) {
        for down in [true, false] {
            guard let event = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: down) else { continue }
            event.flags = []
            event.keyboardSetUnicodeString(stringLength: units.count, unicodeString: units)
            post(event)
        }
    }

    func command(_ line: String) {
        let parts = line.split(separator: " ")
        switch parts.first {
        case "CFG": configure(parts)
        case "KEY": key(parts)
        case "CAPS": toggleCapsLock()
        case "MEDIA": media(parts)
        case "TEXT": text(parts)
        case "QUIT": exit(0)
        default: break
        }
    }
}

extension Data {
    init?(hexString: String) {
        guard hexString.count % 2 == 0 else { return nil }
        var bytes = [UInt8]()
        var index = hexString.startIndex
        while index < hexString.endIndex {
            let next = hexString.index(index, offsetBy: 2)
            guard let b = UInt8(hexString[index..<next], radix: 16) else { return nil }
            bytes.append(b)
            index = next
        }
        self.init(bytes)
    }
}

func withHIDSystem(_ body: (io_connect_t) -> Void) {
    let service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching(kIOHIDSystemClass))
    guard service != 0 else { return }
    var connection: io_connect_t = 0
    if IOServiceOpen(service, mach_task_self_, UInt32(kIOHIDParamConnectType), &connection) == KERN_SUCCESS {
        body(connection)
        IOServiceClose(connection)
    }
    IOObjectRelease(service)
}

func setCapsLock(_ on: Bool) {
    withHIDSystem { _ = IOHIDSetModifierLockState($0, Int32(kIOHIDCapsLockState), on) }
}

func toggleCapsLock() {
    withHIDSystem { connection in
        var state = false
        IOHIDGetModifierLockState(connection, Int32(kIOHIDCapsLockState), &state)
        _ = IOHIDSetModifierLockState(connection, Int32(kIOHIDCapsLockState), !state)
    }
}

func readCommands(_ capture: Capture) {
    Thread.detachNewThread {
        while let line = readLine() {
            DispatchQueue.main.async { capture.command(line) }
        }
        exit(0)
    }
}

func check() -> Never {
    let t = trusted(prompt: false)
    let probe = Capture()
    let tapOK = probe.start()
    print("accessibility \(t.accessibility ? 1 : 0)")
    print("input_monitoring \(t.input ? 1 : 0)")
    print("tap \(tapOK ? 1 : 0)")
    print("layout \(keyboardLayout())")
    exit(0)
}

func run(prompt: Bool) -> Never {
    let capture = Capture()
    let t = trusted(prompt: prompt)
    if !capture.start() {
        emit("ERR permission accessibility=\(t.accessibility ? 1 : 0) input_monitoring=\(t.input ? 1 : 0)")
        exit(3)
    }
    capture.watchTap()
    capture.watchSystem()
    capture.reportLatency()
    emit("READY \(keyboardLayout())")
    capture.ready = true
    readCommands(capture)
    CFRunLoopRun()
    exit(0)
}

func maskHex(_ codes: [Int]) -> String {
    var bytes = [UInt8](repeating: 0, count: 16)
    for c in codes { bytes[c / 8] |= 1 << UInt8(c % 8) }
    return bytes.map { String(format: "%02x", $0) }.joined()
}

func selftest() -> Never {
    var lines: [String] = []
    sink = { lines.append($0) }
    let c = Capture()
    c.resetCapsLock = {}
    var failures = 0
    func expect(_ name: String, _ ok: Bool) {
        if !ok { failures += 1; print("FAIL \(name)") }
    }
    func event(_ type: CGEventType, _ kc: Int, _ flags: UInt64 = 0, again: Bool = false) -> CGEvent {
        let e = CGEvent(keyboardEventSource: nil, virtualKey: CGKeyCode(kc), keyDown: type == .keyDown)!
        e.type = type
        e.flags = CGEventFlags(rawValue: flags)
        if again { e.setIntegerValueField(.keyboardEventAutorepeat, value: 1) }
        return e
    }
    func taken() -> [String] {
        defer { lines = [] }
        return lines.filter { !$0.hasPrefix("KBTYPE") }
    }
    c.command("CFG \(maskHex([0, 55, 57, 10])) 0 1")
    expect("masked key down is swallowed", c.decide(.keyDown, event(.keyDown, 0, 0x100108)))
    expect("masked key reports D with flags", taken() == ["D 0 100108"])
    expect("repeat of a swallowed key is swallowed", c.decide(.keyDown, event(.keyDown, 0, 0, again: true)))
    expect("repeat reports nothing", taken().isEmpty)
    expect("masked key up is swallowed", c.decide(.keyUp, event(.keyUp, 0)))
    expect("masked key reports U", taken() == ["U 0"])
    expect("other key passes", !c.decide(.keyDown, event(.keyDown, 11)))
    expect("other key up passes", !c.decide(.keyUp, event(.keyUp, 11)))
    expect("other keys report nothing", taken().isEmpty)
    expect("masked modifier is swallowed", c.decide(.flagsChanged, event(.flagsChanged, 55, 0x100008)))
    expect("modifier reports D", taken() == ["D 55 100008"])
    expect("hidden modifier is stripped from later events", c.visible(0x100008 | 0x20000 | 0x2) == 0x20002)
    expect("modifier release is swallowed", c.decide(.flagsChanged, event(.flagsChanged, 55, 0)))
    expect("modifier reports U", taken() == ["U 55"])
    expect("modifier no longer hidden", c.visible(0x100008) == 0x100008)
    expect("unmasked modifier passes", !c.decide(.flagsChanged, event(.flagsChanged, 56, 0x2)))
    expect("unmasked modifier release passes", !c.decide(.flagsChanged, event(.flagsChanged, 56, 0)))
    expect("caps lock in the mask is swallowed", c.decide(.flagsChanged, event(.flagsChanged, 57, 0x10000)))
    expect("caps lock reports a tap", taken() == ["D 57 10000", "U 57"])
    c.command("CFG \(maskHex([])) 1 1")
    expect("report-all lets keys through", !c.decide(.keyDown, event(.keyDown, 12)))
    expect("report-all reports P", taken() == ["P 12"])
    expect("report-all release passes", !c.decide(.keyUp, event(.keyUp, 12)))
    expect("report-all reports R", taken() == ["R 12"])
    c.command("CFG \(maskHex([0])) 0 1")
    _ = c.decide(.keyDown, event(.keyDown, 0))
    _ = taken()
    c.releaseAll()
    expect("releaseAll lets go of a swallowed key", taken() == ["U 0"])
    _ = c.decide(.keyDown, event(.keyDown, 0))
    _ = taken()
    c.command("CFG \(maskHex([0])) 0 0")
    expect("inactive passes everything", !c.decide(.keyDown, event(.keyDown, 0)) && taken().isEmpty)
    c.command("CFG \(maskHex([0])) 0 1")
    expect("a key held before pausing is forgotten", c.decide(.keyDown, event(.keyDown, 0)))
    print(failures == 0 ? "selftest ok" : "selftest failed: \(failures)")
    exit(failures == 0 ? 0 : 1)
}

let args = Array(CommandLine.arguments.dropFirst())
setvbuf(stdout, nil, _IOLBF, 0)
switch args.first {
case "--check":
    disclaimedRespawn()
    check()
case "--selftest":
    selftest()
case "run":
    disclaimedRespawn()
    run(prompt: args.contains("--prompt"))
default:
    FileHandle.standardError.write(Data("usage: typedeck-capture run [--prompt] | --check | --selftest\n".utf8))
    exit(2)
}
