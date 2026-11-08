// Package trust makes the system trust the app's root certificate for TLS
// servers, so that the traffic proxy can open HTTPS with it. The trust is
// the person's own: macOS adds a user trust setting in the login keychain,
// Windows adds the certificate to the current user's Root store, and Linux
// writes Chrome's NSS database and, through pkexec, the system store. macOS
// and Windows show the system's own approval dialog, and a refused or
// dismissed approval is ErrPermission. Linux: not yet verified on a real
// system.
package trust

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // G505: the Windows tools identify certificates by SHA-1
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The errors the package returns, wrapped with detail, so test with
// errors.Is.
var (
	// ErrPermission: the system refused the change, usually because the
	// person dismissed or denied its approval dialog.
	ErrPermission = errors.New("permission denied")
	// ErrUnsupported: this system has no trust store this package can
	// change.
	ErrUnsupported = errors.New("certificate trust is not supported on this system")
)

// Install makes the system trust certPEM as a root for TLS servers. macOS
// and Windows ask the person to approve it in the system's own dialog.
func Install(certPEM []byte) error {
	c, err := parseCert(certPEM)
	if err != nil {
		return err
	}
	return install(c)
}

// Remove undoes Install: it takes away the trust, and the certificate from
// the store Install put it in. It is not an error when the certificate is
// not trusted.
func Remove(certPEM []byte) error {
	c, err := parseCert(certPEM)
	if err != nil {
		return err
	}
	return remove(c)
}

// Trusted reports whether the system trusts certPEM as a root for TLS
// servers.
func Trusted(certPEM []byte) (bool, error) {
	c, err := parseCert(certPEM)
	if err != nil {
		return false, err
	}
	return trusted(c)
}

// cert is one certificate and the fingerprints the system tools identify
// it by. It is never identified by name: names are not unique, and the
// fingerprint is what the trust stores already key on.
type cert struct {
	pem    []byte
	der    []byte
	sha1   string // uppercase hex, the form certutil prints and takes
	sha256 string // uppercase hex, the form `security -Z` prints and takes
	// pemSHA256 is the lowercase hex SHA-256 of pem, the form sha256sum -c
	// reads back. It binds the file an elevated command installs to the
	// certificate this package parsed, even if the file is rewritten while
	// the person is being asked.
	pemSHA256 string
}

// parseCert reads the one PEM CERTIFICATE block certPEM must hold. Anything
// else, such as a key or a chain, is refused: installing trust for the wrong
// block would quietly trust the wrong root.
func parseCert(certPEM []byte) (cert, error) {
	block, rest := pem.Decode(certPEM)
	if block == nil {
		return cert{}, errors.New("certificate: no PEM block found")
	}
	if block.Type != "CERTIFICATE" {
		return cert{}, fmt.Errorf("certificate: PEM block is %q, want CERTIFICATE", block.Type)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return cert{}, fmt.Errorf("certificate: %d more bytes after the PEM block, want one certificate", len(rest))
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return cert{}, fmt.Errorf("certificate: %w", err)
	}
	sum1 := sha1.Sum(block.Bytes) //nolint:gosec // G401: certutil takes a SHA-1 hash, not a signature
	sum256 := sha256.Sum256(block.Bytes)
	encoded := pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: block.Bytes})
	sumPEM := sha256.Sum256(encoded)
	return cert{
		pem:       encoded,
		der:       block.Bytes,
		sha1:      strings.ToUpper(hex.EncodeToString(sum1[:])),
		sha256:    strings.ToUpper(hex.EncodeToString(sum256[:])),
		pemSHA256: hex.EncodeToString(sumPEM[:]),
	}, nil
}

// execFunc runs one command and returns its combined output.
type execFunc func(name string, args ...string) ([]byte, error)

// runner runs the system's certificate tools; tests inject exec.
type runner struct{ exec execFunc }

// newRunner returns a runner that runs the real commands.
func newRunner() *runner { return &runner{exec: runCommand} }

// runCommand runs one command and keeps its output for the error it returns.
func runCommand(name string, args ...string) ([]byte, error) {
	cmd := configure(exec.CommandContext(context.Background(), name, args...)) //nolint:gosec // G204: name and args are this package's constants and generated paths
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

// withTempPEM writes the certificate to a private temporary file for a
// system tool and removes it whatever the tool does. The file is 0600
// because the tools take a path, and the certificate must not be readable
// by another user in between.
func withTempPEM(c cert, fn func(path string) error) error {
	f, err := os.CreateTemp("", "omt-trust-*.crt")
	if err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	path := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(path)
	}
	if _, err := f.Write(c.pem); err != nil {
		cleanup()
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("write certificate: %w", err)
	}
	defer func() { _ = os.Remove(path) }()
	return fn(path)
}

// says reports whether a command's output mentions any of markers,
// case-insensitively. The system tools have no machine-readable status for
// these cases, so their words are what there is to read.
func says(out []byte, markers ...string) bool {
	text := strings.ToLower(string(out))
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// outputLine is the first non-empty line of a command's output, for the
// error it caused.
func outputLine(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
