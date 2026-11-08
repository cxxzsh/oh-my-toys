package trust

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseCert(t *testing.T) {
	valid := testPEM(t)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}})
	for _, c := range []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"no PEM block", []byte("not a certificate")},
		{"private key", keyPEM},
		{"two certificates", append(append([]byte{}, valid...), valid...)},
		{"trailing text", append(append([]byte{}, valid...), []byte("garbage")...)},
		{"broken DER", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("no")})},
	} {
		if _, err := parseCert(c.in); err == nil {
			t.Errorf("%s: parsed without error", c.name)
		}
	}
}

func TestParseCertFingerprints(t *testing.T) {
	certPEM := testPEM(t)
	block, _ := pem.Decode(certPEM)
	want := sha256.Sum256(block.Bytes)
	wantOld := sha1.Sum(block.Bytes)
	c, err := parseCert(certPEM)
	if err != nil {
		t.Fatalf("parseCert: %v", err)
	}
	if got, wantHex := c.sha256, strings.ToUpper(hex.EncodeToString(want[:])); got != wantHex {
		t.Errorf("sha256 = %s, want %s", got, wantHex)
	}
	if got, wantHex := c.sha1, strings.ToUpper(hex.EncodeToString(wantOld[:])); got != wantHex {
		t.Errorf("sha1 = %s, want %s", got, wantHex)
	}
	wantPEM := sha256.Sum256(certPEM)
	if got, wantHex := c.pemSHA256, hex.EncodeToString(wantPEM[:]); got != wantHex {
		t.Errorf("pemSHA256 = %s, want %s", got, wantHex)
	}
	if !bytes.Equal(c.der, block.Bytes) {
		t.Error("der is not the certificate's DER")
	}
	if !bytes.Equal(c.pem, certPEM) {
		t.Errorf("pem = %q, want the certificate's PEM block", c.pem)
	}
}

func TestWithTempPEM(t *testing.T) {
	c := testCertificate(t)
	path := ""
	err := withTempPEM(c, func(p string) error {
		path = p
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		// Windows keeps access control lists, not mode bits: Stat reports
		// 0666 and 0777 whatever the ACL says.
		if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
			t.Errorf("mode = %#o, want 0600", mode)
		}
		got, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, c.pem) {
			t.Errorf("file holds %q, want the certificate's PEM", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withTempPEM: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the temporary certificate was left behind: %v", err)
	}
}

func TestWithTempPEMRemovesOnError(t *testing.T) {
	c := testCertificate(t)
	wantErr := errors.New("the tool failed")
	path := ""
	err := withTempPEM(c, func(p string) error {
		path = p
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("withTempPEM returned %v, want %v", err, wantErr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the temporary certificate was left behind: %v", err)
	}
}

func TestSays(t *testing.T) {
	out := []byte("CertUtil: -addstore command FAILED: Access is DENIED\n")
	for _, c := range []struct {
		text  string
		found bool
	}{
		{"denied", true},
		{"Access is denied", true},
		{"not found", false},
	} {
		if got := says(out, c.text); got != c.found {
			t.Errorf("says(%q) = %t, want %t", c.text, got, c.found)
		}
	}
}

func TestOutputLine(t *testing.T) {
	for _, c := range []struct {
		out  string
		want string
	}{
		{"", ""},
		{"\n\n  first  \nsecond\n", "first"},
		{"only\n", "only"},
	} {
		if got := outputLine([]byte(c.out)); got != c.want {
			t.Errorf("outputLine(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}

// testPEM makes a throwaway self-signed root that is never installed
// anywhere.
func testPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "oh-my-toys test root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// testCertificate is testPEM parsed the way Install, Remove and Trusted
// parse it.
func testCertificate(t *testing.T) cert {
	t.Helper()
	c, err := parseCert(testPEM(t))
	if err != nil {
		t.Fatalf("parseCert: %v", err)
	}
	return c
}

// call is one command a fake runner was asked to run.
type call struct {
	name string
	args []string
}

// reply is what a command of a fake runner answers.
type reply struct {
	out string
	err error
}

// fakeRunner is a runner whose commands and answers a test gives.
type fakeRunner struct {
	calls   []call
	replies []reply
	inspect func(c call)
}

// runner is the runner that runs the fake.
func (f *fakeRunner) runner() *runner { return &runner{exec: f.exec} }

// exec records one call and answers it with the next reply.
func (f *fakeRunner) exec(name string, args ...string) ([]byte, error) {
	c := call{name: name, args: args}
	f.calls = append(f.calls, c)
	if f.inspect != nil {
		f.inspect(c)
	}
	if len(f.replies) == 0 {
		return nil, nil
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return []byte(r.out), r.err
}

// sameCall reports whether two calls are the same command line.
func sameCall(a, b call) bool {
	return a.name == b.name && slices.Equal(a.args, b.args)
}
