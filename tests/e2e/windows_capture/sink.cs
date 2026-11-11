using System;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Windows.Forms;

class Sink : Form, IMessageFilter
{
    static StreamWriter log;
    static Stopwatch clock = Stopwatch.StartNew();
    Timer poll = new Timer();
    string stopFile;

    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);

    [STAThread]
    static void Main(string[] args)
    {
        log = new StreamWriter(new FileStream(args[0], FileMode.Create, FileAccess.Write, FileShare.ReadWrite)) { AutoFlush = true };
        Application.EnableVisualStyles();
        Sink s = new Sink(args[1]);
        Application.AddMessageFilter(s);
        Application.Run(s);
    }

    Sink(string stop)
    {
        stopFile = stop;
        Text = "td-sink";
        TopMost = true;
        StartPosition = FormStartPosition.Manual;
        Location = new System.Drawing.Point(20, 20);
        Size = new System.Drawing.Size(360, 140);
        Shown += delegate { Activate(); SetForegroundWindow(Handle); Write("ready caps=" + Control.IsKeyLocked(Keys.CapsLock)); };
        poll.Interval = 200;
        poll.Tick += delegate { if (File.Exists(stopFile)) Close(); };
        poll.Start();
    }

    static void Write(string line)
    {
        lock (log) log.WriteLine(clock.ElapsedMilliseconds + " " + line);
    }

    public bool PreFilterMessage(ref Message m)
    {
        int id = m.Msg;
        if (id == 0x100 || id == 0x101 || id == 0x104 || id == 0x105)
        {
            long lp = m.LParam.ToInt64();
            int sc = (int)((lp >> 16) & 0xFF);
            int ext = (int)((lp >> 24) & 1);
            int rep = (int)((lp >> 30) & 1);
            string kind = (id == 0x100 || id == 0x104) ? "kd" : "ku";
            Write(kind + " vk=" + m.WParam.ToInt64().ToString("X2") + " sc=" + sc.ToString("X2") + " ext=" + ext + " rep=" + rep);
        }
        else if (id == 0x102 || id == 0x106)
        {
            Write("ch " + m.WParam.ToInt64().ToString("X4"));
        }
        return false;
    }
}
