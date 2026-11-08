# Answers Yes to the confirmation dialogs that certutil shows when a root
# certificate is added to or taken from the current user's store, the way a
# person does. A CI runner has an interactive desktop but nobody in front of
# it, so a dialog would wait for ever and the live trust test would hang.
$ErrorActionPreference = 'Stop'

Add-Type @'
using System;
using System.Diagnostics;
using System.Text;
using System.Runtime.InteropServices;

public static class Dialog {
    delegate bool EnumProc(IntPtr h, IntPtr l);

    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc p, IntPtr l);
    [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h, EnumProc p, IntPtr l);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);

    const uint BM_CLICK = 0x00F5;
    const int MaxText = 256;
    const string DialogClass = "#32770";
    const string OwnerProcess = "certutil";

    static string Title(IntPtr h) {
        var s = new StringBuilder(MaxText);
        GetWindowText(h, s, MaxText);
        return s.ToString();
    }

    static string Class(IntPtr h) {
        var s = new StringBuilder(MaxText);
        GetClassName(h, s, MaxText);
        return s.ToString();
    }

    static bool IsCertutilDialog(IntPtr h) {
        if (!IsWindowVisible(h) || Class(h) != DialogClass) {
            return false;
        }
        uint pid;
        GetWindowThreadProcessId(h, out pid);
        try {
            return Process.GetProcessById((int)pid).ProcessName.Equals(OwnerProcess, StringComparison.OrdinalIgnoreCase);
        } catch (ArgumentException) {
            return false;
        }
    }

    static bool IsYes(string text) {
        return text == "&Yes" || text == "Yes";
    }

    // Approve clicks Yes on a dialog certutil shows and returns its title and
    // buttons for the log, or null when there was none.
    public static string Approve() {
        string answered = null;
        EnumWindows((top, l) => {
            if (!IsCertutilDialog(top)) {
                return true;
            }
            var buttons = new StringBuilder();
            IntPtr yes = IntPtr.Zero;
            EnumChildWindows(top, (child, l2) => {
                if (Class(child) == "Button") {
                    buttons.Append('[').Append(Title(child)).Append(']');
                    if (IsYes(Title(child))) {
                        yes = child;
                    }
                }
                return true;
            }, IntPtr.Zero);
            if (yes != IntPtr.Zero) {
                SendMessage(yes, BM_CLICK, IntPtr.Zero, IntPtr.Zero);
                answered = "answered Yes to \"" + Title(top) + "\" " + buttons;
            } else {
                answered = "has no Yes button: \"" + Title(top) + "\" " + buttons;
            }
            return false;
        }, IntPtr.Zero);
        return answered;
    }
}
'@

$pollMilliseconds = 300
$last = ''
while ($true) {
    $answered = [Dialog]::Approve()
    if ($answered -and $answered -ne $last) {
        Write-Host $answered
    }
    $last = $answered
    Start-Sleep -Milliseconds $pollMilliseconds
}
