# Answers Yes to the "Security Warning" that Windows shows when a root
# certificate is added to the current user's store, the way a person does.
# A CI runner has an interactive desktop but nobody in front of it, so the
# dialog would wait for ever and the live trust test would hang.
$ErrorActionPreference = 'Stop'

Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;

public static class Dialog {
    delegate bool EnumProc(IntPtr h, IntPtr l);

    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc p, IntPtr l);
    [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h, EnumProc p, IntPtr l);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);

    const uint BM_CLICK = 0x00F5;
    const int MaxText = 256;

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

    // ApproveSecurityWarning clicks Yes on a visible Security Warning dialog
    // and reports whether it found one.
    public static bool ApproveSecurityWarning() {
        bool clicked = false;
        EnumWindows((top, l) => {
            if (!IsWindowVisible(top) || Class(top) != "#32770" || Title(top) != "Security Warning") {
                return true;
            }
            EnumChildWindows(top, (child, l2) => {
                if (Class(child) == "Button" && Title(child) == "&Yes") {
                    SendMessage(child, BM_CLICK, IntPtr.Zero, IntPtr.Zero);
                    clicked = true;
                    return false;
                }
                return true;
            }, IntPtr.Zero);
            return !clicked;
        }, IntPtr.Zero);
        return clicked;
    }
}
'@

$pollMilliseconds = 300
while ($true) {
    if ([Dialog]::ApproveSecurityWarning()) {
        Write-Host 'answered Yes to a Security Warning'
    }
    Start-Sleep -Milliseconds $pollMilliseconds
}
