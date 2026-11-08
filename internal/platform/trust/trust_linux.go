package trust

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The programs and directories of the Linux trust stores. Every program is
// named by absolute path, never through PATH: a tool planted earlier on
// PATH could read the certificate, fake a store, or show a fake approval
// dialog.
const (
	shellCmd  = "/bin/sh"
	copyCmd   = "/bin/cp"
	moveCmd   = "/bin/mv"
	removeCmd = "/bin/rm"
	pkexecCmd = "/usr/bin/pkexec"

	// debianAnchors and redhatAnchors are where the two common layouts keep
	// a single certificate, and debianUpdate and redhatUpdate are the
	// programs that rebuild the bundle from it.
	debianAnchors = "/usr/local/share/ca-certificates"
	debianUpdate  = "/usr/sbin/update-ca-certificates"
	redhatAnchors = "/etc/pki/ca-trust/source/anchors"
	redhatUpdate  = "/usr/bin/update-ca-trust"

	// nssDBDir is Chrome's NSS database, which the Chromium family reads
	// its roots from.
	nssDBDir = ".pki/nssdb"

	// pkexecDismissed and pkexecNotAuthorized are the statuses pkexec
	// exits with when the person dismisses its prompt or is not allowed to
	// run the command.
	pkexecDismissed     = 126
	pkexecNotAuthorized = 127
)

// configure leaves the command as it is: Linux has no console window to
// keep out of sight for a windowed app.
func configure(cmd *exec.Cmd) *exec.Cmd { return cmd }

// install adds the certificate to every trust store this system has:
// Chrome's NSS database when certutil can write it, and the system store,
// for which pkexec asks the person to allow the change.
func install(c cert) error { return installWith(c, newRunner()) }

// installWith is install with the command runner to test it through.
func installWith(c cert, r *runner) error {
	nss, hasNSS := findNSS()
	store, hasStore := findSystemStore()
	elev, hasElevation := findElevation()
	hasStore = hasStore && hasElevation
	if !hasNSS && !hasStore {
		return fmt.Errorf("install certificate: %w", ErrUnsupported)
	}
	return withTempPEM(c, func(path string) error {
		if hasNSS {
			if err := nss.add(c, path, r); err != nil {
				return err
			}
		}
		if hasStore {
			if err := store.add(c, path, elev, r); err != nil {
				return err
			}
		}
		return nil
	})
}

// remove undoes install in every store that is there.
func remove(c cert) error { return removeWith(c, newRunner()) }

// removeWith is remove with the command runner to test it through.
func removeWith(c cert, r *runner) error {
	nss, hasNSS := findNSS()
	store, hasStore := findSystemStore()
	elev, hasElevation := findElevation()
	hasStore = hasStore && hasElevation
	if !hasNSS && !hasStore {
		return fmt.Errorf("remove certificate: %w", ErrUnsupported)
	}
	if hasNSS {
		if err := nss.remove(c, r); err != nil {
			return err
		}
	}
	if hasStore {
		if err := store.remove(c, elev, r); err != nil {
			return err
		}
	}
	return nil
}

// trusted reports whether one of the trust stores holds the certificate.
// Chrome reads the NSS database, and the rest of the system reads the
// bundle rebuilt from the anchor directory, so either store counts.
func trusted(c cert) (bool, error) { return trustedWith(c, newRunner()) }

// trustedWith is trusted with the command runner to test it through.
func trustedWith(c cert, r *runner) (bool, error) {
	nss, hasNSS := findNSS()
	store, hasStore := findSystemStore()
	if !hasNSS && !hasStore {
		return false, fmt.Errorf("check trusted certificate: %w", ErrUnsupported)
	}
	if hasNSS {
		trust, err := nss.trusted(c, r)
		if err != nil {
			return false, err
		}
		if trust {
			return true, nil
		}
	}
	return hasStore && store.trusted(c), nil
}

// nssStore is the current user's NSS database, which certutil writes.
type nssStore struct {
	certutil string
	db       string // the address certutil takes, "sql:" and the directory
}

// findNSS returns Chrome's NSS database when certutil is installed where
// this package looks and the database is there.
func findNSS() (nssStore, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nssStore{}, false
	}
	db := filepath.Join(home, nssDBDir)
	if !exists(db) {
		return nssStore{}, false
	}
	certutil, ok := findCertutil()
	if !ok {
		return nssStore{}, false
	}
	return nssStore{certutil: certutil, db: "sql:" + db}, true
}

// findCertutil returns the NSS tool from the places it is installed, never
// from PATH.
func findCertutil() (string, bool) {
	return firstExisting("/usr/bin/certutil", "/usr/local/bin/certutil")
}

// nick is the name the certificate gets in the database. NSS knows
// certificates by nickname, so the fingerprint is the name that cannot
// collide with another root or go stale.
func (n nssStore) nick(c cert) string { return "oh-my-toys " + c.sha256 }

// add trusts the certificate as a CA for TLS servers.
func (n nssStore) add(c cert, pemPath string, r *runner) error {
	out, err := r.exec(n.certutil, "-d", n.db, "-A", "-t", "C,,", "-n", n.nick(c), "-i", pemPath)
	switch {
	case err == nil:
		return nil
	case says(out, "already exists"):
		return nil
	default:
		return fmt.Errorf("add certificate to NSS database: %w: %s", err, outputLine(out))
	}
}

// remove takes the certificate out of the database.
func (n nssStore) remove(c cert, r *runner) error {
	out, err := r.exec(n.certutil, "-d", n.db, "-D", "-n", n.nick(c))
	switch {
	case err == nil, itemNotFound(out):
		return nil
	default:
		return fmt.Errorf("remove certificate from NSS database: %w: %s", err, outputLine(out))
	}
}

// trusted reports whether the database holds the certificate.
func (n nssStore) trusted(c cert, r *runner) (bool, error) {
	out, err := r.exec(n.certutil, "-d", n.db, "-L", "-n", n.nick(c))
	switch {
	case err == nil:
		return true, nil
	case itemNotFound(out):
		return false, nil
	default:
		return false, fmt.Errorf("read NSS database: %w: %s", err, outputLine(out))
	}
}

// itemNotFound reports whether certutil said the certificate is not in the
// database; removing an absent certificate is not a failure.
func itemNotFound(out []byte) bool {
	return says(out, "could not find cert", "no cert with nickname", "not found", "does not exist")
}

// systemStore is a distribution's system-wide trust store: a directory of
// single certificates, and the program that rebuilds the bundle from it.
type systemStore struct {
	anchorDir string
	update    string
	sha256sum string // the checksum tool the elevated script verifies with
}

// findSystemStore returns the system store of this distribution, if it is
// one of the two common layouts and its files are there. Without the
// checksum tool the certificate cannot be verified before it is installed,
// so no store is reported at all rather than an unchecked one.
func findSystemStore() (systemStore, bool) {
	sum, ok := firstExisting("/usr/bin/sha256sum", "/bin/sha256sum")
	if !ok {
		return systemStore{}, false
	}
	if exists(debianAnchors) && exists(debianUpdate) {
		return systemStore{anchorDir: debianAnchors, update: debianUpdate, sha256sum: sum}, true
	}
	if exists(redhatAnchors) && exists(redhatUpdate) {
		return systemStore{anchorDir: redhatAnchors, update: redhatUpdate, sha256sum: sum}, true
	}
	return systemStore{}, false
}

// anchorName is the file the certificate is installed as, named after its
// fingerprint so that it cannot collide with another root.
func anchorName(c cert) string { return "omt-traffic-" + c.sha256 + ".crt" }

// stagingName is where the elevated copy lands first. It sits in the anchor
// directory, which only root can write, so nothing of the person's can
// change it after root has checked it.
func stagingName(c cert) string { return anchorName(c) + ".part" }

// add installs the certificate and rebuilds the bundle. Root copies the
// file to a name of its own, checks it against the checksum of the
// certificate this package parsed, and only then moves it into place: the
// temporary file belongs to the person, so a process of theirs could
// rewrite it while the polkit prompt is open and get a certificate of its
// own trusted by the whole system.
func (s systemStore) add(c cert, pemPath string, e elevation, r *runner) error {
	staging := filepath.Join(s.anchorDir, stagingName(c))
	anchor := filepath.Join(s.anchorDir, anchorName(c))
	script := copyCmd + " -f " + shellQuote(pemPath) + " " + shellQuote(staging) +
		" && printf '%s  %s\\n' " + shellQuote(c.pemSHA256) + " " + shellQuote(staging) +
		" | " + s.sha256sum + " -c -" +
		" && " + moveCmd + " -f " + shellQuote(staging) + " " + shellQuote(anchor) +
		" && " + s.update
	return e.run(script, "add certificate to system store", r)
}

// remove deletes the certificate, any staging file left by an interrupted
// install, and rebuilds the bundle.
func (s systemStore) remove(c cert, e elevation, r *runner) error {
	staging := filepath.Join(s.anchorDir, stagingName(c))
	anchor := filepath.Join(s.anchorDir, anchorName(c))
	script := removeCmd + " -f " + shellQuote(anchor) + " " + shellQuote(staging) + " && " + s.update
	return e.run(script, "remove certificate from system store", r)
}

// trusted reports whether the anchor file is there; the bundle was rebuilt
// when it was written.
func (s systemStore) trusted(c cert) bool {
	return exists(filepath.Join(s.anchorDir, anchorName(c)))
}

// elevation runs a command that needs root: directly when this process
// already is root, and through pkexec, which asks the person, otherwise.
type elevation struct{ pkexec string }

// findElevation returns how this system elevates, or false when it has no
// way to.
func findElevation() (elevation, bool) {
	if os.Geteuid() == 0 {
		return elevation{}, true
	}
	if exists(pkexecCmd) {
		return elevation{pkexec: pkexecCmd}, true
	}
	return elevation{}, false
}

// command wraps script so that it runs with root's rights.
//
// pkexec's prompt says only that it runs /bin/sh as the super user. Its
// words come from the polkit action org.freedesktop.policykit.exec, and
// pkexec takes no option to pass a reason, so a plain-language reason would
// need a polkit policy file installed system-wide, which is itself a system
// change this package does not make, so the prompt stays generic.
func (e elevation) command(script string) (name string, args []string) {
	if e.pkexec == "" {
		return shellCmd, []string{"-c", script}
	}
	return e.pkexec, []string{shellCmd, "-c", script}
}

// run runs script with root's rights, and maps a dismissed or refused
// prompt to ErrPermission.
func (e elevation) run(script, what string, r *runner) error {
	name, args := e.command(script)
	out, err := r.exec(name, args...)
	switch {
	case err == nil:
		return nil
	case refused(err):
		return fmt.Errorf("%s: %w", what, ErrPermission)
	default:
		return fmt.Errorf("%s: %w: %s", what, err, outputLine(out))
	}
}

// refused reports whether pkexec exited because the person dismissed or was
// refused its prompt.
func refused(err error) bool {
	var exit interface{ ExitCode() int }
	if !errors.As(err, &exit) {
		return false
	}
	code := exit.ExitCode()
	return code == pkexecDismissed || code == pkexecNotAuthorized
}

// shellQuote wraps a path in single quotes for /bin/sh, so that a path with
// a space or a quote still reaches the program as one argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// firstExisting returns the first of paths that exists.
func firstExisting(paths ...string) (string, bool) {
	for _, path := range paths {
		if exists(path) {
			return path, true
		}
	}
	return "", false
}

// exists reports whether path is there.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
