package procnet

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		{"bundle executable", "/Applications/Safari.app/Contents/MacOS/Safari", "Safari"},
		{"nested helper uses outer bundle", "/Applications/Safari.app/Contents/Frameworks/WebKit.framework/Helper.app/Contents/MacOS/Helper", "Safari"},
		{"plain unix path", "/usr/bin/curl", "curl"},
		{"windows exe", `C:\Program Files\App\app.exe`, "app"},
		{"windows exe upper case", `C:\Tools\TOOL.EXE`, "TOOL"},
		{"exe name without suffix", "/bin/tool", "tool"},
		{"test binary keeps suffix", "/tmp/go-build/procnet.test", "procnet.test"},
		{"empty path", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayName(tt.path); got != tt.want {
				t.Fatalf("displayName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
