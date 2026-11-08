package ca

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreateWritesRoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	authority, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	checkMode(t, dir, dirMode)
	checkMode(t, filepath.Join(dir, rootCertFile), certMode)
	checkMode(t, filepath.Join(dir, rootKeyFile), keyMode)
	checkPKCS8Key(t, filepath.Join(dir, rootKeyFile))
	checkRootCertificate(t, authority)
	checkLoadedRoot(t, dir, authority)
}

// checkPKCS8Key checks that the key file holds a PKCS#8 private key.
func checkPKCS8Key(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("key file holds %v, want a PKCS#8 PRIVATE KEY block", block)
	}
}

// checkRootCertificate checks what the root must be.
func checkRootCertificate(t *testing.T, authority *Authority) {
	t.Helper()
	cert := authority.cert
	if !cert.IsCA {
		t.Error("root is not a CA")
	}
	if !cert.MaxPathLenZero || cert.MaxPathLen != 0 {
		t.Errorf("path length = %d (zero %v), want 0", cert.MaxPathLen, cert.MaxPathLenZero)
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 || cert.KeyUsage&x509.KeyUsageCRLSign == 0 {
		t.Errorf("key usage %v is not cert sign and CRL sign", cert.KeyUsage)
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("extended key usage = %v, want only server auth", cert.ExtKeyUsage)
	}
	if len(cert.Subject.Organization) != 1 || cert.Subject.Organization[0] != "oh-my-toys" {
		t.Errorf("organization = %v, want oh-my-toys", cert.Subject.Organization)
	}
	prefix := "oh-my-toys Root CA "
	suffix := strings.TrimPrefix(cert.Subject.CommonName, prefix)
	if suffix == cert.Subject.CommonName || len(suffix) != 8 {
		t.Errorf("common name %q is not %q and 8 hex characters", cert.Subject.CommonName, prefix)
	}
	if life := time.Until(cert.NotAfter); life < 9*365*24*time.Hour || life > 11*365*24*time.Hour {
		t.Errorf("root is valid for %v, want about 10 years", life)
	}
}

// checkLoadedRoot checks that Load reads back what Create wrote.
func checkLoadedRoot(t *testing.T, dir string, authority *Authority) {
	t.Helper()
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint() != authority.Fingerprint() {
		t.Errorf("loaded fingerprint %s, want %s", loaded.Fingerprint(), authority.Fingerprint())
	}
	if !loaded.NotAfter().Equal(authority.NotAfter()) {
		t.Errorf("loaded not after %v, want %v", loaded.NotAfter(), authority.NotAfter())
	}
	if loaded.CertPath() != filepath.Join(dir, rootCertFile) {
		t.Errorf("certificate path = %q", loaded.CertPath())
	}
}

func TestLoadMissingAndRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if _, err := Load(dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load without files = %v, want ErrNotFound", err)
	}
	if err := Remove(dir); err != nil {
		t.Fatalf("Remove without files = %v, want nil", err)
	}
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load after Remove = %v, want ErrNotFound", err)
	}
	if err := Remove(dir); err != nil {
		t.Fatalf("second Remove = %v, want nil", err)
	}
}

func TestLoadRejectsDamagedFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rootCertFile), []byte("not a certificate"), certMode); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Load with a damaged certificate = %v, want a descriptive error", err)
	}
}

func TestLoadRejectsAMismatchedKey(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if _, err := Create(first); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(second); err != nil {
		t.Fatal(err)
	}
	mixed := filepath.Join(t.TempDir(), "mixed")
	if err := os.MkdirAll(mixed, dirMode); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(first, rootCertFile), filepath.Join(mixed, rootCertFile))
	copyFile(t, filepath.Join(second, rootKeyFile), filepath.Join(mixed, rootKeyFile))
	_, err := Load(mixed)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Load with another key = %v, want a mismatch error", err)
	}
}

func TestLeafVerifiesAgainstTheRoot(t *testing.T) {
	authority, err := Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := authority.Leaf("example.com")
	if err != nil {
		t.Fatal(err)
	}
	checkLeafShape(t, leaf, authority)
	checkLeafChain(t, leaf, authority)
	checkLeafKey(t, leaf)
}

// checkLeafShape checks that a leaf is what a client expects of a server
// certificate.
func checkLeafShape(t *testing.T, leaf *tls.Certificate, authority *Authority) {
	t.Helper()
	if len(leaf.Certificate) != 1 {
		t.Fatalf("leaf chain has %d certificates, want only the leaf", len(leaf.Certificate))
	}
	parsed := leaf.Leaf
	if parsed == nil {
		t.Fatal("leaf certificate is not parsed")
	}
	if parsed.IsCA {
		t.Error("leaf is a CA")
	}
	if len(parsed.DNSNames) != 1 || parsed.DNSNames[0] != "example.com" {
		t.Errorf("DNS names = %v, want example.com", parsed.DNSNames)
	}
	if !hasUsage(parsed.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		t.Errorf("extended key usage = %v, want server auth", parsed.ExtKeyUsage)
	}
	if parsed.NotAfter.After(authority.NotAfter()) {
		t.Errorf("leaf outlives its root: %v after %v", parsed.NotAfter, authority.NotAfter())
	}
	if life := parsed.NotAfter.Sub(parsed.NotBefore); life > leafValidity {
		t.Errorf("leaf is valid for %v, want at most 397 days", life)
	}
}

// checkLeafChain verifies a leaf against a pool holding only the root.
func checkLeafChain(t *testing.T, leaf *tls.Certificate, authority *Authority) {
	t.Helper()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(authority.CertPEM()) {
		t.Fatal("root certificate does not parse")
	}
	chains, err := leaf.Leaf.Verify(x509.VerifyOptions{
		DNSName:   "example.com",
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		t.Fatalf("verify leaf: %v", err)
	}
	if len(chains) != 1 || len(chains[0]) != 2 {
		t.Fatalf("verified chains = %v, want the leaf and the root", chains)
	}
}

// checkLeafKey checks that the leaf's key belongs to its certificate.
func checkLeafKey(t *testing.T, leaf *tls.Certificate) {
	t.Helper()
	key, ok := leaf.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("leaf key is %T, want ECDSA", leaf.PrivateKey)
	}
	public, ok := leaf.Leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || !key.PublicKey.Equal(public) {
		t.Error("leaf key does not match the leaf certificate")
	}
}

// TestLoadAcceptsARootWithoutEKU reads a root made by an older version of
// this package, before the root was restricted to server auth.
func TestLoadAcceptsARootWithoutEKU(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "oh-my-toys Root CA 00000000", Organization: []string{"oh-my-toys"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writeFile(filepath.Join(dir, rootCertFile), certPEM, certMode); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, rootKeyFile), keyPEM, keyMode); err != nil {
		t.Fatal(err)
	}

	authority, err := Load(dir)
	if err != nil {
		t.Fatalf("Load of a root without extended key usage: %v", err)
	}
	leaf, err := authority.Leaf("example.com")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(authority.CertPEM())
	_, err = leaf.Leaf.Verify(x509.VerifyOptions{
		DNSName:   "example.com",
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		t.Fatalf("an old root no longer signs a usable server leaf: %v", err)
	}
}

func TestLeafForAnIPAddress(t *testing.T) {
	authority, err := Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := authority.Leaf("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.Leaf.IPAddresses) != 1 || leaf.Leaf.IPAddresses[0].String() != "127.0.0.1" {
		t.Fatalf("IP addresses = %v, want 127.0.0.1", leaf.Leaf.IPAddresses)
	}
	if len(leaf.Leaf.DNSNames) != 0 {
		t.Errorf("DNS names = %v, want none for an IP host", leaf.Leaf.DNSNames)
	}
}

func TestLeafIsCached(t *testing.T) {
	authority, err := Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := authority.Leaf("one.example")
	if err != nil {
		t.Fatal(err)
	}
	again, err := authority.Leaf("one.example")
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Error("the same host made a second certificate instead of using the cache")
	}
	other, err := authority.Leaf("two.example")
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Error("another host reused the certificate of the first")
	}
}

func TestLeafCacheEvictsTheOldest(t *testing.T) {
	authority, err := Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= maxLeaves; i++ {
		authority.store(fmt.Sprintf("host-%d", i), leafEntry{notAfter: time.Now().Add(time.Hour)})
	}
	if len(authority.leaves) > maxLeaves {
		t.Fatalf("cache holds %d leaves, want at most %d", len(authority.leaves), maxLeaves)
	}
	if _, ok := authority.leaves["host-0"]; ok {
		t.Error("the oldest leaf stayed in a full cache")
	}
}

// TestLeafServesATLSConnection makes a real handshake, the way a client
// that trusts only this root would see the certificate.
func TestLeafServesATLSConnection(t *testing.T) {
	authority, err := Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := authority.Leaf("example.com")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(authority.CertPEM())
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close(); _ = clientConn.Close() })
	server := tls.Server(serverConn, &tls.Config{
		Certificates: []tls.Certificate{{Certificate: leaf.Certificate, PrivateKey: leaf.PrivateKey}},
	})
	client := tls.Client(clientConn, &tls.Config{ServerName: "example.com", RootCAs: roots})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.HandshakeContext(ctx) }()
	if err := client.HandshakeContext(ctx); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
}

// checkMode fails when path does not have mode. Windows keeps access control
// lists, not mode bits: Stat reports 0666 and 0777 whatever the ACL says.
func checkMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != mode {
		t.Errorf("%s has mode %o, want %o", filepath.Base(path), got, mode)
	}
}

// copyFile copies src to dst with a private mode.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// hasUsage reports whether usages holds want.
func hasUsage(usages []x509.ExtKeyUsage, want x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == want {
			return true
		}
	}
	return false
}
