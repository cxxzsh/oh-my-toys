//go:build windows

package trust

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testCertutil is a certutil path in the shape the package resolves.
const testCertutil = `C:\Windows\System32\certutil.exe`

func TestCertutilPathIsAbsolute(t *testing.T) {
	tool, err := certutilPath()
	if err != nil {
		t.Fatalf("certutilPath: %v", err)
	}
	if !filepath.IsAbs(tool) || filepath.Base(tool) != certutilName {
		t.Errorf("certutilPath = %q, want an absolute path to %s", tool, certutilName)
	}
}

func TestInstallWindows(t *testing.T) {
	c := testCertificate(t)
	f := &fakeRunner{}
	var temp string
	f.inspect = func(cl call) {
		if last := len(cl.args) - 1; last >= 0 && strings.HasSuffix(cl.args[last], ".crt") {
			temp = cl.args[last]
		}
	}
	if err := installWith(c, f.runner(), testCertutil); err != nil {
		t.Fatalf("installWith: %v", err)
	}
	want := call{name: testCertutil, args: []string{"-user", "-f", "-addstore", "Root", temp}}
	if temp == "" || !sameCall(f.calls[0], want) {
		t.Fatalf("ran %v, want %v", f.calls, want)
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Errorf("the temporary certificate was left behind: %v", err)
	}
}

func TestInstallWindowsRefused(t *testing.T) {
	f := &fakeRunner{replies: []reply{{
		out: "CertUtil: -addstore command FAILED: 0x800704c7 (WIN32: 1223 ERROR_CANCELLED)\n",
		err: errors.New("exit status 1"),
	}}}
	if err := installWith(testCertificate(t), f.runner(), testCertutil); !errors.Is(err, ErrPermission) {
		t.Fatalf("installWith returned %v, want ErrPermission", err)
	}
}

func TestInstallWindowsOtherFailure(t *testing.T) {
	f := &fakeRunner{replies: []reply{{out: "CertUtil: the store is read-only\n", err: errors.New("exit status 1")}}}
	err := installWith(testCertificate(t), f.runner(), testCertutil)
	if err == nil || errors.Is(err, ErrPermission) || !strings.Contains(err.Error(), "the store is read-only") {
		t.Fatalf("installWith returned %v, want a detailed error that is not ErrPermission", err)
	}
}

func TestRemoveWindows(t *testing.T) {
	c := testCertificate(t)
	f := &fakeRunner{}
	if err := removeWith(c, f.runner(), testCertutil); err != nil {
		t.Fatalf("removeWith: %v", err)
	}
	want := call{name: testCertutil, args: []string{"-user", "-delstore", "Root", c.sha1}}
	if !sameCall(f.calls[0], want) {
		t.Fatalf("ran %v, want %v", f.calls, want)
	}
}

func TestRemoveWindowsAbsent(t *testing.T) {
	f := &fakeRunner{replies: []reply{{
		out: "CertUtil: -delstore command FAILED: 0x80092004 (CRYPT_E_NOT_FOUND)\n",
		err: errors.New("exit status 1"),
	}}}
	if err := removeWith(testCertificate(t), f.runner(), testCertutil); err != nil {
		t.Fatalf("removeWith a certificate that is not trusted: %v", err)
	}
}

func TestTrustedWindows(t *testing.T) {
	for _, c := range []struct {
		name           string
		replies        []reply
		want           bool
		wantPermission bool
		wantErr        bool
	}{
		{name: "in the Root store", replies: []reply{{out: "================ Certificate 0 ================\n"}}, want: true},
		{
			name: "not in the Root store",
			replies: []reply{{
				out: "CertUtil: -verifyStore command FAILED: 0x80092004 (CRYPT_E_NOT_FOUND)\n",
				err: errors.New("exit status 1"),
			}},
		},
		{
			name:           "access denied",
			replies:        []reply{{out: "CertUtil: -verifyStore command FAILED: 0x80070005 (WIN32: 5 ERROR_ACCESS_DENIED)\n", err: errors.New("exit status 1")}},
			wantPermission: true,
		},
		{
			name:    "another failure",
			replies: []reply{{out: "CertUtil: the store is broken\n", err: errors.New("exit status 1")}},
			wantErr: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cert := testCertificate(t)
			f := &fakeRunner{replies: c.replies}
			trust, err := trustedWith(cert, f.runner(), testCertutil)
			if trust != c.want {
				t.Errorf("trustedWith = %t, want %t", trust, c.want)
			}
			switch {
			case c.wantPermission && !errors.Is(err, ErrPermission):
				t.Errorf("trustedWith returned %v, want ErrPermission", err)
			case c.wantErr && err == nil:
				t.Error("trustedWith returned no error")
			case !c.wantPermission && !c.wantErr && err != nil:
				t.Errorf("trustedWith: %v", err)
			}
		})
	}
}

func TestCertutilWords(t *testing.T) {
	for _, c := range []struct {
		out              string
		denied, notFound bool
	}{
		{"CertUtil: 0x80070005 (WIN32: 5 ERROR_ACCESS_DENIED)\n", true, false},
		{"The operation was canceled by the user. 0x800704c7\n", true, false},
		{"CertUtil: The specified certificate is not found.\n", false, true},
		{"CertUtil: -store command FAILED: Cannot find the certificate.\n", false, true},
		{"CertUtil: something else\n", false, false},
	} {
		if got := permissionDenied([]byte(c.out)); got != c.denied {
			t.Errorf("permissionDenied(%q) = %t, want %t", c.out, got, c.denied)
		}
		if got := itemNotFound([]byte(c.out)); got != c.notFound {
			t.Errorf("itemNotFound(%q) = %t, want %t", c.out, got, c.notFound)
		}
	}
}
