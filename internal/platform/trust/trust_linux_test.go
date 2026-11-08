//go:build linux

package trust

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testStore is a system store laid out as Debian has it.
func testStore() systemStore {
	return systemStore{
		anchorDir: "/usr/local/share/ca-certificates",
		update:    "/usr/sbin/update-ca-certificates",
		sha256sum: "/usr/bin/sha256sum",
	}
}

func TestShellQuote(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "''"},
		{"/tmp/a.crt", "'/tmp/a.crt'"},
		{"/tmp/a b.crt", "'/tmp/a b.crt'"},
		{"/tmp/it's.crt", `'/tmp/it'\''s.crt'`},
	} {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestAnchorName(t *testing.T) {
	c := testCertificate(t)
	if got, want := anchorName(c), "omt-traffic-"+c.sha256+".crt"; got != want {
		t.Errorf("anchorName = %q, want %q", got, want)
	}
	if got, want := stagingName(c), anchorName(c)+".part"; got != want {
		t.Errorf("stagingName = %q, want %q", got, want)
	}
}

func TestSystemStoreCommands(t *testing.T) {
	c := testCertificate(t)
	store := testStore()
	anchor := store.anchorDir + "/" + anchorName(c)
	staging := store.anchorDir + "/" + stagingName(c)
	for _, tc := range []struct {
		name string
		run  func(*runner) error
		want []string
	}{
		{
			name: "add",
			run:  func(r *runner) error { return store.add(c, "/tmp/omt-trust-1.crt", elevation{}, r) },
			want: []string{
				"-c",
				"/bin/cp -f '/tmp/omt-trust-1.crt' '" + staging + "'" +
					" && printf '%s  %s\\n' '" + c.pemSHA256 + "' '" + staging + "'" +
					" | /usr/bin/sha256sum -c -" +
					" && /bin/mv -f '" + staging + "' '" + anchor + "'" +
					" && /usr/sbin/update-ca-certificates",
			},
		},
		{
			name: "remove",
			run:  func(r *runner) error { return store.remove(c, elevation{}, r) },
			want: []string{"-c", "/bin/rm -f '" + anchor + "' '" + staging + "' && /usr/sbin/update-ca-certificates"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRunner{}
			if err := tc.run(f.runner()); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			want := call{name: "/bin/sh", args: tc.want}
			if !sameCall(f.calls[0], want) {
				t.Errorf("ran %v, want %v", f.calls[0], want)
			}
		})
	}
}

// TestAddChecksTheCopiedFile pins the order of the elevated install: copy,
// check the copy against the certificate's checksum, move, update.
func TestAddChecksTheCopiedFile(t *testing.T) {
	c := testCertificate(t)
	store := testStore()
	staging := store.anchorDir + "/" + stagingName(c)
	f := &fakeRunner{}
	if err := store.add(c, "/tmp/omt-trust-1.crt", elevation{}, f.runner()); err != nil {
		t.Fatalf("store.add: %v", err)
	}
	script := f.calls[0].args[1]
	steps := []string{
		"/bin/cp -f '/tmp/omt-trust-1.crt' '" + staging + "'",
		"printf '%s  %s\\n' '" + c.pemSHA256 + "' '" + staging + "'",
		"| /usr/bin/sha256sum -c -",
		"/bin/mv -f '" + staging + "' '" + store.anchorDir + "/" + anchorName(c) + "'",
		"/usr/sbin/update-ca-certificates",
	}
	at := -1
	for _, step := range steps {
		next := strings.Index(script, step)
		if next <= at {
			t.Fatalf("the script does not run %q after the step before it: %s", step, script)
		}
		at = next
	}
}

func TestAddQuotesPaths(t *testing.T) {
	c := testCertificate(t)
	store := testStore()
	path := "/tmp/a b/it's.crt"
	f := &fakeRunner{}
	if err := store.add(c, path, elevation{}, f.runner()); err != nil {
		t.Fatalf("store.add: %v", err)
	}
	script := f.calls[0].args[1]
	if !strings.Contains(script, shellQuote(path)) {
		t.Errorf("the source path is not quoted: %s", script)
	}
	if strings.Contains(script, path) {
		t.Errorf("the source path reached the script unquoted: %s", script)
	}
}

func TestElevationCommands(t *testing.T) {
	const script = "/bin/cp -f a b && update"
	for _, c := range []struct {
		name     string
		elev     elevation
		wantName string
		wantArgs []string
	}{
		{"root runs it directly", elevation{}, "/bin/sh", []string{"-c", script}},
		{"elsewhere pkexec asks", elevation{pkexec: "/usr/bin/pkexec"}, "/usr/bin/pkexec", []string{"/bin/sh", "-c", script}},
	} {
		name, args := c.elev.command(script)
		if name != c.wantName || !slices.Equal(args, c.wantArgs) {
			t.Errorf("%s: command = %s %v, want %s %v", c.name, name, args, c.wantName, c.wantArgs)
		}
	}
}

func TestElevationRefuses(t *testing.T) {
	c := testCertificate(t)
	for _, tc := range []struct {
		name    string
		reply   reply
		wantErr bool
	}{
		{name: "dismissed prompt", reply: reply{err: exitError(pkexecDismissed)}, wantErr: true},
		{name: "not authorized", reply: reply{err: exitError(pkexecNotAuthorized)}, wantErr: true},
		{name: "another failure", reply: reply{err: errors.New("exit status 1")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRunner{replies: []reply{tc.reply}}
			err := testStore().add(c, "/tmp/omt-trust-1.crt", elevation{pkexec: "/usr/bin/pkexec"}, f.runner())
			if got := errors.Is(err, ErrPermission); got != tc.wantErr {
				t.Fatalf("store.add returned %v, want ErrPermission: %t", err, tc.wantErr)
			}
		})
	}
}

func TestRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"another error", errors.New("exit status 1"), false},
		{"dismissed", exitError(pkexecDismissed), true},
		{"not authorized", exitError(pkexecNotAuthorized), true},
		{"wrapped dismissal", fmt.Errorf("run pkexec: %w", exitError(pkexecDismissed)), true},
		{"a broken command", exitError(1), false},
	} {
		if got := refused(c.err); got != c.want {
			t.Errorf("%s: refused = %t, want %t", c.name, got, c.want)
		}
	}
}

func TestNSSCommands(t *testing.T) {
	c := testCertificate(t)
	n := nssStore{certutil: "/usr/bin/certutil", db: "sql:/home/x/.pki/nssdb"}
	nick := "oh-my-toys " + c.sha256
	for _, tc := range []struct {
		name    string
		run     func(*runner) (bool, error)
		want    []string
		replies []reply
		trusted bool
	}{
		{
			name: "add",
			run:  func(r *runner) (bool, error) { return false, n.add(c, "/tmp/omt-trust-1.crt", r) },
			want: nssArgs("-A", "-t", "C,,", "-n", nick, "-i", "/tmp/omt-trust-1.crt"),
		},
		{
			name: "remove",
			run:  func(r *runner) (bool, error) { return false, n.remove(c, r) },
			want: nssArgs("-D", "-n", nick),
		},
		{
			name: "trusted",
			run:  func(r *runner) (bool, error) { return n.trusted(c, r) },
			want: nssArgs("-L", "-n", nick),
		},
		{
			name:    "not trusted",
			run:     func(r *runner) (bool, error) { return n.trusted(c, r) },
			want:    nssArgs("-L", "-n", nick),
			replies: []reply{{out: "certutil: could not find cert: " + nick + "\n", err: errors.New("exit status 255")}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRunner{replies: tc.replies}
			trust, err := tc.run(f.runner())
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if tc.name == "trusted" && !trust {
				t.Error("a certificate in the database is not trusted")
			}
			if tc.name == "not trusted" && trust {
				t.Error("a certificate that is not in the database is trusted")
			}
			want := call{name: "/usr/bin/certutil", args: tc.want}
			if !sameCall(f.calls[0], want) {
				t.Errorf("ran %v, want %v", f.calls[0], want)
			}
		})
	}
}

func TestFirstExisting(t *testing.T) {
	dir := t.TempDir()
	there := filepath.Join(dir, "there")
	if err := os.WriteFile(there, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", there, err)
	}
	got, ok := firstExisting(filepath.Join(dir, "missing"), there)
	if !ok || got != there {
		t.Errorf("firstExisting = %q, %t, want %q, true", got, ok, there)
	}
	if got, ok := firstExisting(filepath.Join(dir, "missing")); ok {
		t.Errorf("firstExisting found %q, want nothing", got)
	}
}

// nssArgs is the argument list certutil takes, a fresh one per call.
func nssArgs(rest ...string) []string {
	return append([]string{"-d", "sql:/home/x/.pki/nssdb"}, rest...)
}

// exitError is a failed command with an exit status, the way
// *exec.ExitError reports it.
type exitError int

// Error is the message of a failed command.
func (e exitError) Error() string { return "exit status" }

// ExitCode is what the command exited with.
func (e exitError) ExitCode() int { return int(e) }
