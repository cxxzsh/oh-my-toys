package trust

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// securityCmd is the tool that changes the trust settings and the keychain.
// It is named by absolute path, never through PATH: a tool planted earlier
// on PATH could show a fake approval dialog and keep the password.
const securityCmd = "/usr/bin/security"

// configure leaves the command as it is: macOS has no console window to
// keep out of sight for a windowed app.
func configure(cmd *exec.Cmd) *exec.Cmd { return cmd }

// install gives the certificate user trust as a TLS server root. `security`
// shows the system's own approval dialog, and imports the certificate into
// the default keychain, where Remove finds it again.
func install(c cert) error { return installWith(c, newRunner()) }

// installWith is install with the command runner to test it through.
func installWith(c cert, r *runner) error {
	keychain, err := defaultKeychain(r)
	if err != nil {
		return err
	}
	return withTempPEM(c, func(path string) error {
		out, err := r.exec(securityCmd, "add-trusted-cert", "-r", "trustRoot", "-p", "ssl", "-k", keychain, path)
		return securityErr("add trusted certificate", out, err, false)
	})
}

// remove takes the trust setting away and then the certificate, so the
// keychain is as it was before Install.
func remove(c cert) error { return removeWith(c, newRunner()) }

// removeWith is remove with the command runner to test it through.
func removeWith(c cert, r *runner) error {
	err := withTempPEM(c, func(path string) error {
		out, err := r.exec(securityCmd, "remove-trusted-cert", path)
		return securityErr("remove trusted certificate", out, err, true)
	})
	if err != nil {
		return err
	}
	out, err := r.exec(securityCmd, "delete-certificate", "-Z", c.sha256)
	return securityErr("delete certificate", out, err, true)
}

// defaultKeychain is the keychain `security` imports the certificate into,
// asked rather than assumed: a person can rename the login keychain.
func defaultKeychain(r *runner) (string, error) {
	out, err := r.exec(securityCmd, "default-keychain", "-d", "user")
	if err != nil {
		return "", fmt.Errorf("find login keychain: %w", err)
	}
	return parseKeychain(out)
}

// parseKeychain reads the quoted path `security default-keychain` prints,
// indented by four spaces.
func parseKeychain(out []byte) (string, error) {
	path := strings.TrimSpace(string(out))
	if unquoted, err := strconv.Unquote(path); err == nil {
		path = unquoted
	}
	if path == "" {
		return "", errors.New("find login keychain: security printed no keychain")
	}
	return path, nil
}

// securityErr maps a failing `security` command to the package's errors;
// absent is whether a missing item counts as success.
func securityErr(what string, out []byte, err error, absent bool) error {
	switch {
	case err == nil:
		return nil
	case absent && itemNotFound(out):
		return nil
	case permissionDenied(out):
		return fmt.Errorf("%s: %w", what, ErrPermission)
	default:
		return fmt.Errorf("%s: %w: %s", what, err, outputLine(out))
	}
}

// permissionDenied reports whether `security` said the person refused its
// approval dialog. It has no status code for this, only words.
func permissionDenied(out []byte) bool {
	return says(out, "denied", "cancel", "-60005")
}

// itemNotFound reports whether `security` said the certificate or its trust
// setting is absent; removing an absent certificate is not a failure.
func itemNotFound(out []byte) bool {
	return says(out, "could not be found", "no trust settings", "not found", "-25263", "-25300")
}
