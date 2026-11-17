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
        view.isRichText = false
        view.isAutomaticQuoteSubstitutionEnabled = false
        view.isAutomaticDashSubstitutionEnabled = false
        view.isAutomaticTextReplacementEnabled = false
        view.isAutomaticSpellingCorrectionEnabled = false
        view.isContinuousSpellCheckingEnabled = false
        view.isAutomaticTextCompletionEnabled = false
        view.smartInsertDeleteEnabled = false
        window.contentView!.addSubview(view)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        NSApp.activate(ignoringOtherApps: true)
        NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [self] event in
            events.append("\(event.keyCode)" + (event.modifierFlags.contains(.command) ? "+cmd" : ""))
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
let menu = NSMenu()
let appItem = NSMenuItem()
appItem.submenu = NSMenu()
appItem.submenu!.addItem(withTitle: "Quit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
let editItem = NSMenuItem()
editItem.submenu = NSMenu(title: "Edit")
for (title, action, key) in [("Cut", #selector(NSText.cut(_:)), "x"), ("Copy", #selector(NSText.copy(_:)), "c"),
                             ("Paste", #selector(NSText.paste(_:)), "v"), ("Select All", #selector(NSText.selectAll(_:)), "a")] {
    editItem.submenu!.addItem(withTitle: title, action: action, keyEquivalent: key)
}
menu.addItem(appItem)
menu.addItem(editItem)
app.mainMenu = menu
let delegate = Delegate()
app.delegate = delegate
app.run()
