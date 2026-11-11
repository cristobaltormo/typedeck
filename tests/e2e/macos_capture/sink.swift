import Cocoa

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "/tmp/typedeck-sink.txt"

final class Delegate: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var view: NSTextView!
    var events: [String] = []

    func applicationDidFinishLaunching(_ note: Notification) {
        window = NSWindow(contentRect: NSRect(x: 200, y: 200, width: 520, height: 240), styleMask: [.titled], backing: .buffered, defer: false)
        window.title = "Typedeck test sink"
        window.level = .floating
        view = NSTextView(frame: window.contentView!.bounds)
        view.autoresizingMask = [.width, .height]
        window.contentView!.addSubview(view)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        NSApp.activate(ignoringOtherApps: true)
        NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [self] event in
            events.append("\(event.keyCode)")
            return event
        }
        Timer.scheduledTimer(withTimeInterval: 0.2, repeats: true) { [self] _ in
            let body = "TEXT:" + view.string + "\nKEYS:" + events.joined(separator: ",") + "\nFRONT:" + (NSApp.isActive ? "1" : "0") + "\n"
            try? body.write(toFile: out, atomically: true, encoding: .utf8)
        }
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = Delegate()
app.delegate = delegate
app.run()
