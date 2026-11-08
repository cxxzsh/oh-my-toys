// Package ca makes the root certificate of the traffic tool and the leaf
// certificates its proxy presents to the HTTPS clients it opens. The root
// key is a secret that stays on this machine: it is written with mode 0600
// into a 0700 folder, is never logged and is never handed out.
package ca

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrNotFound is what Load returns when the folder holds no root
// certificate yet.
var ErrNotFound = errors.New("root certificate not found")

// The files the root lives in, and the modes they are made with. The key is
// a secret, so its file and the folder are closed to everyone else.
const (
	rootCertFile = "root.pem"
	rootKeyFile  = "root.key"
	dirMode      = 0o700
	certMode     = 0o644
	keyMode      = 0o600
)

// How long the root and its leaves stay valid, and how close to its expiry
// a cached leaf is replaced.
const (
	rootValidity = 10 * 365 * 24 * time.Hour
	backdate     = time.Hour
	leafValidity = 397 * 24 * time.Hour
	leafRefresh  = 24 * time.Hour
)

// maxLeaves bounds the leaf cache, and leafBits the serial numbers.
const (
	maxLeaves = 1024
	leafBits  = 128
	nameBytes = 4
)

// Authority is the app's root certificate and the leaves it signs.
type Authority struct {
	dir     string
	cert    *x509.Certificate
	certPEM []byte
	key     crypto.Signer

	mu     sync.Mutex
	leaves map[string]leafEntry
	order  []string
}

// leafEntry is one cached leaf and the moment it stops being valid.
type leafEntry struct {
	cert     *tls.Certificate
	notAfter time.Time
}

// Load reads the root certificate and its key from dir. It returns
// ErrNotFound when either file is missing, and a descriptive error when the
// files are damaged or do not belong together.
func Load(dir string) (*Authority, error) {
	certPath := filepath.Join(dir, rootCertFile)
	keyPath := filepath.Join(dir, rootKeyFile)

	certPEM, err := readFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readFile(keyPath)
	if err != nil {
		return nil, err
	}

	cert, err := parseCert(certPEM, certPath)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(keyPEM, keyPath)
	if err != nil {
		return nil, err
	}
	if !samePublicKey(cert.PublicKey, key.Public()) {
		return nil, fmt.Errorf("%s does not match the certificate in %s", keyPath, certPath)
	}
	return &Authority{
		dir:     dir,
		cert:    cert,
		certPEM: certPEM,
		key:     key,
		leaves:  make(map[string]leafEntry),
	}, nil
}

// Create makes a new root certificate and key in dir, creating the folder
// with mode 0700 and tightening a folder that is already there. It replaces a
// root that is already there. Callers must serialise Create: two calls at
// once would race for the same two files.
func Create(dir string) (*Authority, error) {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return nil, fmt.Errorf("set the mode of %s: %w", dir, err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate root key: %w", err)
	}
	cert, err := newRoot(key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode root key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writeFile(filepath.Join(dir, rootCertFile), certPEM, certMode); err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(dir, rootKeyFile), keyPEM, keyMode); err != nil {
		return nil, err
	}
	return &Authority{
		dir:     dir,
		cert:    cert,
		certPEM: certPEM,
		key:     key,
		leaves:  make(map[string]leafEntry),
	}, nil
}

// Remove deletes the root certificate and its key from dir, and any
// temporary file an interrupted Create left behind. The key goes first, so a
// failure on the certificate cannot leave a signing key behind. Files that
// are already gone are not an error, and every file is tried before the
// errors are returned together.
func Remove(dir string) error {
	paths := []string{filepath.Join(dir, rootKeyFile), filepath.Join(dir, rootCertFile)}
	if temps, err := filepath.Glob(filepath.Join(dir, "*.tmp")); err == nil {
		paths = append(paths, temps...)
	}
	var errs []error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// CertPEM returns the root certificate in PEM form, which is what a person
// imports into another program.
func (a *Authority) CertPEM() []byte { return bytes.Clone(a.certPEM) }

// CertPath is the file CertPEM is kept in.
func (a *Authority) CertPath() string { return filepath.Join(a.dir, rootCertFile) }

// Fingerprint is the SHA-256 of the certificate in hex, so a person can
// check the installed root against it.
func (a *Authority) Fingerprint() string {
	sum := sha256.Sum256(a.cert.Raw)
	return hex.EncodeToString(sum[:])
}

// NotAfter is when the root certificate stops being valid.
func (a *Authority) NotAfter() time.Time { return a.cert.NotAfter }

// newRoot makes the self-signed certificate for key.
func newRoot(key *ecdsa.PrivateKey) (*x509.Certificate, error) {
	suffix := make([]byte, nameBytes)
	if _, err := rand.Read(suffix); err != nil {
		return nil, fmt.Errorf("name the root certificate: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	notBefore := time.Now().Add(-backdate)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "oh-my-toys Root CA " + hex.EncodeToString(suffix),
			Organization: []string{"oh-my-toys"},
		},
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(rootValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("make root certificate: %w", err)
	}
	return x509.ParseCertificate(der)
}

// randomSerial makes a serial number that cannot be guessed or repeated.
func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), leafBits)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("make serial number: %w", err)
	}
	return serial, nil
}

// readFile reads a root file, turning a missing file into ErrNotFound.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path is a file in the folder the caller chose for the root.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// parseCert decodes the certificate from its PEM file.
func parseCert(data []byte, path string) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s does not hold a certificate", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cert, nil
}

// parseKey decodes the signing key from its PEM file, taking both the PKCS#8
// form this package writes and the SEC 1 form other tools produce.
func parseKey(data []byte, path string) (crypto.Signer, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s does not hold a key", path)
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		signer, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%s holds a key that cannot sign", path)
		}
		return signer, nil
	}
	k, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return k, nil
}

// samePublicKey reports whether two keys are the same EC key.
func samePublicKey(a, b crypto.PublicKey) bool {
	ka, ok := a.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	kb, ok := b.(*ecdsa.PublicKey)
	return ok && ka.Equal(kb)
}

// writeFile writes data atomically and makes sure the file has mode, even
// when it replaced an older file with another mode. The temporary file gets
// a name of its own, so a file or link another program left at a guessed
// name cannot take the write.
func writeFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	temp := file.Name()
	defer func() { _ = os.Remove(temp) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(temp, mode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
