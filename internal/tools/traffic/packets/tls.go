package packets

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// TLS records: the header of any record, and of a ClientHello the legacy
// version, the server name and the ALPN list, read by hand with bounds
// checks so a cut or hostile segment cannot read past its bytes.

const (
	layerTLS = "Transport Layer Security"

	tlsRecordHeaderLen = 5
	tlsVersionMajor    = 3
	tlsVersionLen      = 2
	tlsRandomLen       = 32
	tlsHandshakeHeader = 4
	tlsExtHeaderLen    = 4
	tlsClientHello     = 0x01
	tlsHostNameType    = 0x00
	tlsExtServerName   = 0x0000
	tlsExtALPN         = 0x0010
)

// TLS record content types.
const (
	tlsChangeCipherSpec = 20
	tlsAlert            = 21
	tlsHandshake        = 22
	tlsApplicationData  = 23
	tlsHeartbeat        = 24
)

// tlsRecord decodes a TLS record when the payload starts one, and reports
// whether it did.
func (d *decoder) tlsRecord(payload []byte) bool {
	if len(payload) < tlsRecordHeaderLen || payload[1] != tlsVersionMajor || !isTLSRecordType(payload[0]) {
		return false
	}
	length := int(binary.BigEndian.Uint16(payload[3:tlsRecordHeaderLen]))
	f := &fieldSet{}
	f.add("Record Type", tlsRecordTypeText(payload[0]))
	f.add("Record Version", tlsVersionText(payload[1:tlsVersionLen+1]))
	f.addInt("Record Length", length)
	d.proto = protoTLS
	body := payload[tlsRecordHeaderLen:]
	if length > len(body) {
		d.layer(layerTLS, f)
		d.info = tlsRecordTypeText(payload[0]) + " [truncated]"
		return true
	}
	body = body[:length]
	if payload[0] != tlsHandshake {
		f.addInt("Length", length)
		d.layer(layerTLS, f)
		d.info = tlsRecordTypeText(payload[0]) + ", " + strconv.Itoa(length) + " bytes"
		return true
	}
	hello, ok := parseClientHello(body)
	if !ok {
		d.layer(layerTLS, f)
		d.info = "TLS handshake"
		return true
	}
	f.add("Handshake Type", "ClientHello")
	f.add(fieldVersion, hello.version)
	if hello.sni != "" {
		f.add("SNI", hello.sni)
	}
	if len(hello.alpn) > 0 {
		f.add("ALPN", strings.Join(hello.alpn, ", "))
	}
	d.layer(layerTLS, f)
	d.info = "Client Hello"
	if hello.sni != "" {
		d.info += " (SNI " + hello.sni + ")"
	}
	return true
}

// clientHello is what the detail pane shows of a TLS hello.
type clientHello struct {
	version string
	sni     string
	alpn    []string
}

// clientHello reads a handshake message when it is a complete ClientHello,
// and reports whether it is one.
func parseClientHello(msg []byte) (clientHello, bool) {
	hello := clientHello{}
	if len(msg) < tlsHandshakeHeader || msg[0] != tlsClientHello {
		return hello, false
	}
	length := int(msg[1])<<16 | int(msg[2])<<8 | int(msg[3])
	if length > len(msg)-tlsHandshakeHeader {
		return hello, false
	}
	c := cursor{b: msg[tlsHandshakeHeader : tlsHandshakeHeader+length]}
	version, ok := c.take(tlsVersionLen)
	if !ok {
		return hello, false
	}
	hello.version = tlsVersionText(version)
	if !c.skip(tlsRandomLen) {
		return hello, false
	}
	sidLen, ok := c.u8()
	if !ok || !c.skip(int(sidLen)) || !c.skipList16() || !c.skipList8() {
		return hello, false
	}
	extensions, ok := c.extensionBlock()
	if !ok {
		return hello, false
	}
	hello.sni, hello.alpn = tlsNames(extensions)
	return hello, true
}

// tlsNames reads the SNI and ALPN extensions, and stops at the first one
// that does not fit.
func tlsNames(extensions []byte) (string, []string) {
	var sni string
	var alpn []string
	for len(extensions) >= tlsExtHeaderLen {
		typ := binary.BigEndian.Uint16(extensions[:2])
		size := int(binary.BigEndian.Uint16(extensions[2:4]))
		extensions = extensions[tlsExtHeaderLen:]
		if size > len(extensions) {
			break
		}
		data := extensions[:size]
		extensions = extensions[size:]
		switch typ {
		case tlsExtServerName:
			sni = serverName(data)
		case tlsExtALPN:
			alpn = alpnNames(data)
		}
	}
	return sni, alpn
}

// serverName reads the one host name an SNI extension carries.
func serverName(data []byte) string {
	c := cursor{b: data}
	list, ok := c.list16()
	if !ok {
		return ""
	}
	c = cursor{b: list}
	for len(c.b) > 0 {
		typ, ok := c.u8()
		if !ok {
			return ""
		}
		name, ok := c.list16()
		if !ok {
			return ""
		}
		if typ == tlsHostNameType && isText(name) {
			return string(name)
		}
	}
	return ""
}

// alpnNames reads the protocol names of an ALPN extension.
func alpnNames(data []byte) []string {
	c := cursor{b: data}
	list, ok := c.list16()
	if !ok {
		return nil
	}
	c = cursor{b: list}
	var names []string
	for len(c.b) > 0 {
		name, ok := c.list8()
		if !ok {
			return names
		}
		if isText(name) {
			names = append(names, string(name))
		}
	}
	return names
}

// cursor reads forward through a message and never steps out of it.
type cursor struct{ b []byte }

func (c *cursor) take(n int) ([]byte, bool) {
	if n < 0 || len(c.b) < n {
		return nil, false
	}
	v := c.b[:n]
	c.b = c.b[n:]
	return v, true
}

func (c *cursor) skip(n int) bool {
	_, ok := c.take(n)
	return ok
}

func (c *cursor) u8() (byte, bool) {
	v, ok := c.take(1)
	if !ok {
		return 0, false
	}
	return v[0], true
}

func (c *cursor) u16() (uint16, bool) {
	v, ok := c.take(2)
	if !ok {
		return 0, false
	}
	return binary.BigEndian.Uint16(v), true
}

// list16 takes a 2-byte length and that many bytes.
func (c *cursor) list16() ([]byte, bool) {
	n, ok := c.u16()
	if !ok {
		return nil, false
	}
	return c.take(int(n))
}

// list8 takes a 1-byte length and that many bytes.
func (c *cursor) list8() ([]byte, bool) {
	n, ok := c.u8()
	if !ok {
		return nil, false
	}
	return c.take(int(n))
}

// skipList16 skips a 2-byte length and its bytes.
func (c *cursor) skipList16() bool {
	_, ok := c.list16()
	return ok
}

// skipList8 skips a 1-byte length and its bytes.
func (c *cursor) skipList8() bool {
	_, ok := c.list8()
	return ok
}

// extensionBlock takes the extensions that end a ClientHello; a hello
// written before extensions existed simply has none.
func (c *cursor) extensionBlock() ([]byte, bool) {
	if len(c.b) == 0 {
		return nil, true
	}
	return c.list16()
}

// isTLSRecordType reports whether b is one of the content types TLS
// defines, which keeps binary data that starts like a record out.
func isTLSRecordType(b byte) bool {
	switch b {
	case tlsChangeCipherSpec, tlsAlert, tlsHandshake, tlsApplicationData, tlsHeartbeat:
		return true
	default:
		return false
	}
}

func tlsRecordTypeText(b byte) string {
	name := ""
	switch b {
	case tlsChangeCipherSpec:
		name = "Change Cipher Spec"
	case tlsAlert:
		name = "Alert"
	case tlsHandshake:
		name = "Handshake"
	case tlsApplicationData:
		name = "Application Data"
	case tlsHeartbeat:
		name = "Heartbeat"
	default:
		return strconv.Itoa(int(b))
	}
	return name + " (" + strconv.Itoa(int(b)) + ")"
}

// tlsVersionText names the version of a 2-byte TLS field.
func tlsVersionText(b []byte) string {
	if len(b) < tlsVersionLen {
		return ""
	}
	version := binary.BigEndian.Uint16(b)
	switch version {
	case 0x0300:
		return "SSL 3.0"
	case 0x0301:
		return "TLS 1.0"
	case 0x0302:
		return "TLS 1.1"
	case 0x0303:
		return "TLS 1.2"
	case 0x0304:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}
