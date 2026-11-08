package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"
)

// Leaf returns a certificate for host, which is a DNS name or an IP address,
// signed by the root. Only the leaf is in the chain: the root is never sent
// to a client. Leaves are cached, and replaced a day before they expire.
func (a *Authority) Leaf(host string) (*tls.Certificate, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	if entry, ok := a.leaves[host]; ok && now.Add(leafRefresh).Before(entry.notAfter) {
		return entry.cert, nil
	}
	cert, notAfter, err := a.newLeaf(host, now)
	if err != nil {
		return nil, err
	}
	a.store(host, leafEntry{cert: cert, notAfter: notAfter})
	return cert, nil
}

// newLeaf makes and signs one leaf certificate for host.
func (a *Authority) newLeaf(host string, now time.Time) (*tls.Certificate, time.Time, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("generate leaf key for %s: %w", host, err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, time.Time{}, err
	}
	notBefore := now.Add(-backdate)
	notAfter := notBefore.Add(leafValidity)
	if notAfter.After(a.cert.NotAfter) {
		notAfter = a.cert.NotAfter
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host, Organization: []string{"oh-my-toys"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("make leaf certificate for %s: %w", host, err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse leaf certificate for %s: %w", host, err)
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        parsed,
	}
	return cert, notAfter, nil
}

// store puts a leaf in the bounded cache, dropping the oldest entry when the
// cache is full.
func (a *Authority) store(host string, entry leafEntry) {
	if _, ok := a.leaves[host]; !ok {
		a.order = append(a.order, host)
	}
	a.leaves[host] = entry
	for len(a.leaves) > maxLeaves && len(a.order) > 0 {
		oldest := a.order[0]
		a.order = a.order[1:]
		delete(a.leaves, oldest)
	}
}
