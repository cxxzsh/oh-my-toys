//go:build darwin

package trust

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testKeychain is a keychain path in the shape `security` prints.
const testKeychain = "    \"/Users/x/Library/Keychains/login.keychain-db\"\n"

// TestSecurityPathIsAbsolute pins that the tool is named by absolute path:
// a `security` found through PATH could fake the approval dialog.
func TestSecurityPathIsAbsolute(t *testing.T) {
	if !filepath.IsAbs(securityCmd) {
		t.Errorf("securityCmd = %q, want an absolute path", securityCmd)
	}
}

// TestTrustedRejectsThrowawayCertificate asks the real trust settings about
// a certificate that is generated here and never installed. It reads only.
func TestTrustedRejectsThrowawayCertificate(t *testing.T) {
	certPEM := testPEM(t)
	trust, err := Trusted(certPEM)
	if err != nil {
		t.Fatalf("Trusted: %v", err)
	}
	if trust {
		t.Fatal("a certificate that was never installed is reported as trusted")
	}
	t.Logf("Trusted(throwaway certificate) = %t, %v", trust, err)
}

func TestInstallDarwin(t *testing.T) {
	f := &fakeRunner{replies: []reply{{out: testKeychain}}}
	var temp string
	f.inspect = func(cl call) { temp = certArg(cl) }
	if err := installWith(testCertificate(t), f.runner()); err != nil {
		t.Fatalf("installWith: %v", err)
	}
	wantCalls := []call{
		{name: securityCmd, args: []string{"default-keychain", "-d", "user"}},
		{name: securityCmd, args: []string{"add-trusted-cert", "-r", "trustRoot", "-p", "ssl", "-k", "/Users/x/Library/Keychains/login.keychain-db", temp}},
	}
	if !slices.EqualFunc(f.calls, wantCalls, sameCall) {
		t.Fatalf("ran %v, want %v", f.calls, wantCalls)
	}
	if temp == "" {
		t.Fatal("the certificate was not written to a temporary file")
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Errorf("the temporary certificate was left behind: %v", err)
	}
}

// installFailure is one failing install to check.
type installFailure struct {
	name           string
	replies        []reply
	wantCalls      int
	wantErr        string
	wantPermission bool
}

func TestInstallDarwinFailures(t *testing.T) {
	for _, c := range []installFailure{
		{
			name: "a refused approval is ErrPermission",
			replies: []reply{
				{out: testKeychain},
				{out: "security: SecTrustSettingsSetTrustSettings: The authorization was denied.\n", err: errors.New("exit status 1")},
			},
			wantCalls:      2,
			wantPermission: true,
		},
		{
			name: "another failure keeps its detail",
			replies: []reply{
				{out: testKeychain},
				{out: "security: the keychain is locked\n", err: errors.New("exit status 1")},
			},
			wantCalls: 2,
			wantErr:   "the keychain is locked",
		},
		{
			name:      "a missing keychain path stops before the install",
			replies:   []reply{{err: errors.New("exit status 1")}},
			wantCalls: 1,
			wantErr:   "find login keychain",
		},
	} {
		t.Run(c.name, func(t *testing.T) { checkInstallFailure(t, c) })
	}
}

// checkInstallFailure runs one failing install and checks what it did.
func checkInstallFailure(t *testing.T, c installFailure) {
	t.Helper()
	var temp string
	f := &fakeRunner{replies: c.replies, inspect: func(cl call) { temp = certArg(cl) }}
	err := installWith(testCertificate(t), f.runner())
	if len(f.calls) != c.wantCalls {
		t.Fatalf("ran %d commands, want %d: %v", len(f.calls), c.wantCalls, f.calls)
	}
	if temp != "" {
		if _, statErr := os.Stat(temp); !os.IsNotExist(statErr) {
			t.Errorf("the temporary certificate was left behind: %v", statErr)
		}
	}
	checkFailure(t, err, c.wantPermission, c.wantErr)
}

func TestRemoveDarwin(t *testing.T) {
	c := testCertificate(t)
	f := &fakeRunner{}
	if err := removeWith(c, f.runner()); err != nil {
		t.Fatalf("removeWith: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("ran %d commands, want 2: %v", len(f.calls), f.calls)
	}
	path := ""
	if args := f.calls[0].args; len(args) == 2 && args[0] == "remove-trusted-cert" {
		path = args[1]
	} else {
		t.Fatalf("first command %v, want security remove-trusted-cert <file>", f.calls[0])
	}
	want := call{name: securityCmd, args: []string{"delete-certificate", "-Z", c.sha256}}
	if !sameCall(f.calls[1], want) {
		t.Errorf("then ran %v, want %v", f.calls[1], want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the temporary certificate was left behind: %v", err)
	}
}

func TestRemoveDarwinAbsent(t *testing.T) {
	for _, c := range []struct {
		name    string
		replies []reply
	}{
		{
			name: "no trust setting and no certificate",
			replies: []reply{
				{out: "security: SecTrustSettingsRemoveTrustSettings: No Trust Settings were found.\n", err: errors.New("exit status 1")},
				{out: "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n", err: errors.New("exit status 1")},
			},
		},
		{
			name:    "a certificate that is not there",
			replies: []reply{{}, {out: "security: The specified item could not be found in the keychain.\n", err: errors.New("exit status 1")}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: c.replies}
			if err := removeWith(testCertificate(t), f.runner()); err != nil {
				t.Fatalf("removeWith a certificate that is not trusted: %v", err)
			}
			if len(f.calls) != 2 {
				t.Errorf("ran %d commands, want the trust removal and the deletion: %v", len(f.calls), f.calls)
			}
		})
	}
}

func TestRemoveDarwinPermission(t *testing.T) {
	f := &fakeRunner{replies: []reply{
		{},
		{out: "security: The authorization was denied.\n", err: errors.New("exit status 1")},
	}}
	if err := removeWith(testCertificate(t), f.runner()); !errors.Is(err, ErrPermission) {
		t.Fatalf("removeWith returned %v, want ErrPermission", err)
	}
}

func TestParseKeychain(t *testing.T) {
	for _, c := range []struct {
		out  string
		want string
	}{
		{testKeychain, "/Users/x/Library/Keychains/login.keychain-db"},
		{"/Users/x/Library/Keychains/login.keychain-db\n", "/Users/x/Library/Keychains/login.keychain-db"},
		{"", ""},
	} {
		got, err := parseKeychain([]byte(c.out))
		if c.want == "" {
			if err == nil {
				t.Errorf("parseKeychain(%q) = %q, want an error", c.out, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKeychain(%q): %v", c.out, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseKeychain(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}

func TestSSLDecision(t *testing.T) {
	for _, c := range []struct {
		name     string
		settings []trustSetting
		decided  bool
		trusted  bool
	}{
		{"trust root", []trustSetting{{result: resultTrustRoot}}, true, true},
		{"trust as root", []trustSetting{{result: resultTrustAsRoot}}, true, true},
		{"deny", []trustSetting{{result: resultDeny}}, true, false},
		{"no result", []trustSetting{{result: resultUnspecified}}, false, false},
		{"another policy", []trustSetting{{result: resultTrustRoot, otherPolicy: true}}, false, false},
		{"one application", []trustSetting{{result: resultTrustRoot, application: true}}, false, false},
		{"one host", []trustSetting{{result: resultTrustRoot, host: true}}, false, false},
		{"one key usage", []trustSetting{{result: resultTrustRoot, keyUsage: true}}, false, false},
		{"deny before trust", []trustSetting{{result: resultDeny}, {result: resultTrustRoot}}, true, false},
		{"trust before deny", []trustSetting{{result: resultTrustRoot}, {result: resultDeny}}, true, true},
		{
			"a constraint for one host, then trust",
			[]trustSetting{{result: resultTrustRoot, host: true}, {result: resultTrustRoot}},
			true,
			true,
		},
	} {
		decided, trusted := sslDecision(c.settings)
		if decided != c.decided || trusted != c.trusted {
			t.Errorf("%s: sslDecision = %t, %t, want %t, %t", c.name, decided, trusted, c.decided, c.trusted)
		}
	}
}

func TestSecurityWords(t *testing.T) {
	for _, c := range []struct {
		out              string
		denied, notFound bool
	}{
		{"security: The authorization was denied.\n", true, false},
		{"security: User canceled.\n", true, false},
		{"security: SecTrustSettingsRemoveTrustSettings: No Trust Settings were found.\n", false, true},
		{"security: The specified item could not be found in the keychain.\n", false, true},
		{"security: something else\n", false, false},
	} {
		if got := permissionDenied([]byte(c.out)); got != c.denied {
			t.Errorf("permissionDenied(%q) = %t, want %t", c.out, got, c.denied)
		}
		if got := itemNotFound([]byte(c.out)); got != c.notFound {
			t.Errorf("itemNotFound(%q) = %t, want %t", c.out, got, c.notFound)
		}
	}
}

// certArg is the temporary certificate path of a call, or an empty string.
func certArg(cl call) string {
	if last := len(cl.args) - 1; last >= 0 && strings.HasSuffix(cl.args[last], ".crt") {
		return cl.args[last]
	}
	return ""
}

// checkFailure checks a mapped error against what a test expects.
func checkFailure(t *testing.T, err error, wantPermission bool, wantErr string) {
	t.Helper()
	switch {
	case wantPermission && !errors.Is(err, ErrPermission):
		t.Fatalf("returned %v, want ErrPermission", err)
	case wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)):
		t.Fatalf("returned %v, want an error saying %q", err, wantErr)
	case wantErr == "" && !wantPermission && err != nil:
		t.Fatalf("returned %v, want no error", err)
	}
}
