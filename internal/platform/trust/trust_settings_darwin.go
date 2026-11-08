package trust

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The frameworks the trust settings live in.
const (
	securityFramework = "/System/Library/Frameworks/Security.framework/Security"
	coreFoundation    = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
)

// The trust settings domains and results, and the OSStatus codes the calls
// return, from Security/SecTrustSettings.h.
const (
	domainUser   int64 = 0
	domainAdmin  int64 = 1
	domainSystem int64 = 2

	errSecSuccess         int32 = 0
	errSecNoTrustSettings int32 = -25263
	errSecItemNotFound    int32 = -25300

	resultTrustRoot   int32 = 1
	resultTrustAsRoot int32 = 2
	resultDeny        int32 = 3
	// resultUnspecified trusts nothing and distrusts nothing; the
	// constraints of such a setting only carry an allowed error.
	resultUnspecified int32 = 4

	// cfNumberSInt32 is kCFNumberSInt32Type, the type of an SInt32 result.
	cfNumberSInt32 int64 = 3
	// cfStringUTF8 is kCFStringEncodingUTF8, the encoding of the dictionary
	// keys and OIDs this package reads.
	cfStringUTF8 uint32 = 0x08000100

	// sslPolicyOID is kSecPolicyAppleSSL, the policy a trust setting made
	// for TLS servers constrains. It is the OID, not the policy object,
	// that is compared: the SSL, SSL server and SSL client policies share
	// it, and no two of them are equal as objects.
	sslPolicyOID = "1.2.840.113635.100.1.3"
	// oidBufferSize holds an OID string with room to spare.
	oidBufferSize = 128
)

// trusted reports whether the trust settings that the system uses for TLS
// server checks name the certificate as a root. The settings are read
// directly: `security verify-cert` evaluates a certificate file as a TLS
// server, and fails a root for the extended key usage a server needs, while
// `security dump-trust-settings` prints names and no hashes, so neither
// answers this.
func trusted(c cert) (bool, error) {
	api, err := trustSettings()
	if err != nil {
		return false, fmt.Errorf("read trust settings: %w", err)
	}
	// Trust settings are layered: per-user settings override the admin
	// domain, which overrides the system roots. The first domain that says
	// anything about a TLS server check decides.
	for _, domain := range []int64{domainUser, domainAdmin, domainSystem} {
		decided, trust, err := api.verdict(domain, c.der)
		if err != nil {
			return false, err
		}
		if decided {
			return trust, nil
		}
	}
	return false, nil
}

// trustSetting is one usage constraint of a certificate's trust settings:
// its result, and the constraints that can keep it from applying to a
// generic TLS server check.
type trustSetting struct {
	result      int32
	otherPolicy bool // names a policy that is not SSL
	application bool // names the one program it applies to
	host        bool // names the one server it applies to
	keyUsage    bool // names the key operations it applies to
}

// sslDecision reports whether settings say anything about a generic TLS
// server check and, if so, whether they trust the certificate. Trust
// settings are ORs of usage constraints, and a constraint applies only when
// every part of it matches, so a constraint this check cannot match is
// skipped.
func sslDecision(settings []trustSetting) (decided, trusted bool) {
	for _, s := range settings {
		if s.application || s.host || s.keyUsage || s.otherPolicy {
			continue
		}
		switch s.result {
		case resultTrustRoot, resultTrustAsRoot:
			return true, true
		case resultDeny:
			return true, false
		}
	}
	return false, false
}

// trustFuncs are the Security.framework and CoreFoundation calls the trust
// settings need, resolved once.
type trustFuncs struct {
	copyCerts    func(domain int64, out *unsafe.Pointer) int32
	copySettings func(cert uintptr, domain int64, out *unsafe.Pointer) int32
	certData     func(cert uintptr) uintptr
	dataPtr      func(data uintptr) unsafe.Pointer
	dataLength   func(data uintptr) int64
	copyProps    func(policy uintptr) uintptr
	createString func(alloc uintptr, cstr *byte, encoding uint32) uintptr
	stringC      func(object uintptr, buf *byte, maxLength int64, encoding uint32) bool
	arrayCount   func(array uintptr) int64
	arrayAt      func(array uintptr, index int64) uintptr
	dictGet      func(dict, key uintptr) uintptr
	numberGet    func(number uintptr, typ int64, out unsafe.Pointer) bool
	release      func(object uintptr)
}

// trustKeys are the CFString keys that name the parts of a usage
// constraint dictionary, and of a policy. They are made here rather than
// read from the framework's globals: a string is compared by content, so
// one this package makes finds the same entry.
type trustKeys struct {
	result   uintptr
	policy   uintptr
	app      uintptr
	host     uintptr
	keyUsage uintptr
	oid      uintptr
}

// trustSettings resolves the framework calls on first use; the error says
// what is missing when the framework cannot be used.
var trustSettings = sync.OnceValues(loadTrustFuncs)

// loadTrustFuncs resolves the symbols of Security.framework and
// CoreFoundation that the trust settings need.
func loadTrustFuncs() (*trustFuncs, error) {
	sec, err := purego.Dlopen(securityFramework, purego.RTLD_LAZY)
	if err != nil {
		return nil, fmt.Errorf("open Security.framework: %w", err)
	}
	cf, err := purego.Dlopen(coreFoundation, purego.RTLD_LAZY)
	if err != nil {
		return nil, fmt.Errorf("open CoreFoundation: %w", err)
	}
	f := &trustFuncs{}
	err = errors.Join(
		register(&f.copyCerts, sec, "SecTrustSettingsCopyCertificates"),
		register(&f.copySettings, sec, "SecTrustSettingsCopyTrustSettings"),
		register(&f.certData, sec, "SecCertificateCopyData"),
		register(&f.copyProps, sec, "SecPolicyCopyProperties"),
		register(&f.stringC, cf, "CFStringGetCString"),
		register(&f.arrayCount, cf, "CFArrayGetCount"),
		register(&f.arrayAt, cf, "CFArrayGetValueAtIndex"),
		register(&f.dictGet, cf, "CFDictionaryGetValue"),
		register(&f.numberGet, cf, "CFNumberGetValue"),
		register(&f.release, cf, "CFRelease"),
		register(&f.dataPtr, cf, "CFDataGetBytePtr"),
		register(&f.dataLength, cf, "CFDataGetLength"),
		register(&f.createString, cf, "CFStringCreateWithCString"),
	)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// register makes the C function name of lib callable through dst.
func register[T any](dst *T, lib uintptr, name string) error {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		return fmt.Errorf("load %s: %w", name, err)
	}
	purego.RegisterFunc(dst, sym)
	return nil
}

// keys makes the dictionary keys once per certificate read. The strings are
// the values of the framework's constants, which for the trust settings are
// the symbol names, and for kSecPolicyOid is "SecPolicyOid".
func (f *trustFuncs) keys() (*trustKeys, error) {
	k := &trustKeys{}
	names := []struct {
		dst  *uintptr
		name string
	}{
		{&k.result, "kSecTrustSettingsResult"},
		{&k.policy, "kSecTrustSettingsPolicy"},
		{&k.app, "kSecTrustSettingsApplication"},
		{&k.host, "kSecTrustSettingsPolicyString"},
		{&k.keyUsage, "kSecTrustSettingsKeyUsage"},
		{&k.oid, "SecPolicyOid"},
	}
	for _, n := range names {
		s := f.createString(0, cString(n.name), cfStringUTF8)
		if s == 0 {
			k.release(f)
			return nil, fmt.Errorf("make key %s", n.name)
		}
		*n.dst = s
	}
	return k, nil
}

// release frees the keys.
func (k *trustKeys) release(f *trustFuncs) {
	for _, key := range []uintptr{k.result, k.policy, k.app, k.host, k.keyUsage, k.oid} {
		if key != 0 {
			f.release(key)
		}
	}
}

// cString is the NUL-terminated bytes of s for a framework call, which
// copies them.
func cString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// verdict reads one domain's trust settings for der. decided is false when
// the domain holds nothing that applies to a TLS server check, so that a
// lower domain can decide instead.
func (f *trustFuncs) verdict(domain int64, der []byte) (decided, trusted bool, err error) {
	var certs unsafe.Pointer
	switch status := f.copyCerts(domain, &certs); status {
	case errSecSuccess:
	case errSecNoTrustSettings, errSecItemNotFound:
		return false, false, nil
	default:
		return false, false, fmt.Errorf("read trust settings: status %d", status)
	}
	if certs == nil {
		return false, false, nil
	}
	defer f.release(uintptr(certs))
	for i, n := int64(0), f.arrayCount(uintptr(certs)); i < n; i++ {
		cert := f.arrayAt(uintptr(certs), i)
		if !f.sameCert(cert, der) {
			continue
		}
		return f.settingsVerdict(cert, domain)
	}
	return false, false, nil
}

// settingsVerdict reads the trust settings of the certificate the domain
// found. An empty array is unconditional trust as a root.
func (f *trustFuncs) settingsVerdict(cert uintptr, domain int64) (decided, trusted bool, err error) {
	var settings unsafe.Pointer
	switch status := f.copySettings(cert, domain, &settings); status {
	case errSecSuccess:
	case errSecItemNotFound:
		return false, false, nil
	default:
		return false, false, fmt.Errorf("read trust settings: status %d", status)
	}
	if settings == nil {
		return false, false, nil
	}
	defer f.release(uintptr(settings))
	count := f.arrayCount(uintptr(settings))
	if count == 0 {
		return true, true, nil
	}
	keys, err := f.keys()
	if err != nil {
		return false, false, fmt.Errorf("read trust settings: %w", err)
	}
	defer keys.release(f)
	list := make([]trustSetting, 0, count)
	for i := int64(0); i < count; i++ {
		list = append(list, f.setting(f.arrayAt(uintptr(settings), i), keys))
	}
	decided, trusted = sslDecision(list)
	return decided, trusted, nil
}

// setting reads one usage constraint dictionary.
func (f *trustFuncs) setting(dict uintptr, keys *trustKeys) trustSetting {
	s := trustSetting{result: resultTrustRoot} // the documented default
	if v := f.dictGet(dict, keys.result); v != 0 {
		var result int32
		if f.numberGet(v, cfNumberSInt32, unsafe.Pointer(&result)) { //nolint:gosec // G103: CFNumberGetValue writes one SInt32 into result
			s.result = result
		} else {
			s.result = resultUnspecified
		}
	}
	if v := f.dictGet(dict, keys.policy); v != 0 {
		s.otherPolicy = f.policyOID(v, keys) != sslPolicyOID
	}
	s.application = f.dictGet(dict, keys.app) != 0
	s.host = f.dictGet(dict, keys.host) != 0
	s.keyUsage = f.dictGet(dict, keys.keyUsage) != 0
	return s
}

// policyOID is the OID string of a policy, or an empty string when it
// cannot be read, which no OID matches.
func (f *trustFuncs) policyOID(policy uintptr, keys *trustKeys) string {
	props := f.copyProps(policy)
	if props == 0 {
		return ""
	}
	defer f.release(props)
	value := f.dictGet(props, keys.oid)
	if value == 0 {
		return ""
	}
	buf := make([]byte, oidBufferSize)
	if !f.stringC(value, &buf[0], oidBufferSize, cfStringUTF8) {
		return ""
	}
	if end := bytes.IndexByte(buf, 0); end >= 0 {
		buf = buf[:end]
	}
	return string(buf)
}

// sameCert reports whether the certificate object holds der, so that a
// certificate is matched by its bytes and never by its name.
func (f *trustFuncs) sameCert(cert uintptr, der []byte) bool {
	data := f.certData(cert)
	if data == 0 {
		return false
	}
	defer f.release(data)
	n := f.dataLength(data)
	if n != int64(len(der)) {
		return false
	}
	if n == 0 {
		return true
	}
	return bytes.Equal(unsafe.Slice((*byte)(f.dataPtr(data)), n), der) //nolint:gosec // G103: the bytes belong to the CFData released here, and are only read
}
