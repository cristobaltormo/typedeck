import Cocoa

var opts: [String: String] = [:]
var i = 1
let args = CommandLine.arguments
while i < args.count {
    if args[i].hasPrefix("--"), i + 1 < args.count { opts[String(args[i].dropFirst(2))] = args[i + 1]; i += 2 } else { i += 1 }
}
let title = opts["title"] ?? ""
let subtitle = opts["sub"] ?? ""
let seconds = Double(opts["sec"] ?? "") ?? 1.2
let position = opts["pos"] ?? "bottom"
let dots = Int(opts["dots"] ?? "") ?? 0
let active = Int(opts["active"] ?? "") ?? 0

func color(_ hex: String) -> NSColor {
    var s = hex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))
    if s.count != 6 { s = "2563eb" }
    let v = UInt32(s, radix: 16) ?? 0x2563eb
    return NSColor(calibratedRed: CGFloat((v >> 16) & 255) / 255, green: CGFloat((v >> 8) & 255) / 255,
                   blue: CGFloat(v & 255) / 255, alpha: 1)
}
let accent = color(opts["accent"] ?? "#2563eb")

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

let width: CGFloat = max(300, min(520, CGFloat(title.count) * 18 + 120))
var height: CGFloat = 92
if !subtitle.isEmpty { height += 24 }
if dots > 1 { height += 20 }
let screen = NSScreen.main?.frame ?? NSRect(x: 0, y: 0, width: 1440, height: 900)
let y: CGFloat
switch position {
case "top": y = screen.maxY - height - 90
case "center": y = screen.midY - height / 2
default: y = screen.minY + 150
}
let panel = NSPanel(contentRect: NSRect(x: screen.midX - width / 2, y: y, width: width, height: height),
                    styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
panel.level = .screenSaver
panel.isOpaque = false
panel.backgroundColor = .clear
panel.hasShadow = true
panel.ignoresMouseEvents = true
panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .transient]

let blur = NSVisualEffectView(frame: NSRect(x: 0, y: 0, width: width, height: height))
blur.material = .hudWindow
blur.state = .active
blur.wantsLayer = true
blur.layer?.cornerRadius = 20
blur.layer?.masksToBounds = true

func label(_ text: String, size: CGFloat, weight: NSFont.Weight, alpha: CGFloat, y: CGFloat, h: CGFloat) -> NSTextField {
    let l = NSTextField(labelWithString: text)
    l.font = NSFont.systemFont(ofSize: size, weight: weight)
    l.textColor = NSColor.white.withAlphaComponent(alpha)
    l.alignment = .center
    l.lineBreakMode = .byTruncatingTail
    l.frame = NSRect(x: 18, y: y, width: width - 36, height: h)
    return l
}

var cursorY = height - 22
if dots > 1 {
    let d: CGFloat = 7, gap: CGFloat = 8
    let total = CGFloat(dots) * d + CGFloat(dots - 1) * gap
    var x = (width - total) / 2
    for n in 0..<dots {
        let v = NSView(frame: NSRect(x: x, y: cursorY - d, width: d, height: d))
        v.wantsLayer = true
        v.layer?.cornerRadius = d / 2
        v.layer?.backgroundColor = (n == active ? accent : NSColor.white.withAlphaComponent(0.3)).cgColor
        blur.addSubview(v)
        x += d + gap
    }
    cursorY -= 18
}
blur.addSubview(label(title, size: 28, weight: .semibold, alpha: 1, y: cursorY - 44, h: 36))
if !subtitle.isEmpty {
    blur.addSubview(label(subtitle, size: 14, weight: .regular, alpha: 0.72, y: cursorY - 72, h: 20))
}
let bar = NSView(frame: NSRect(x: 0, y: 0, width: width, height: 4))
bar.wantsLayer = true
bar.layer?.backgroundColor = accent.cgColor
blur.addSubview(bar)

panel.contentView = blur
panel.alphaValue = 0
panel.orderFrontRegardless()
NSAnimationContext.runAnimationGroup({ $0.duration = 0.12; panel.animator().alphaValue = 1 })
DispatchQueue.main.asyncAfter(deadline: .now() + seconds) {
    NSAnimationContext.runAnimationGroup({ $0.duration = 0.25; panel.animator().alphaValue = 0 },
                                         completionHandler: { exit(0) })
}
app.run()
