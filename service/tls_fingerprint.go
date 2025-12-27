package service

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	tls "github.com/refraction-networking/utls"
)

const (
	// nodeJSJA3Version is the TLS version used in JA3 fingerprint calculation.
	// For TLS 1.3 compatibility mode, the client_version field is fixed at 0x0303 (TLS 1.2 = 771).
	nodeJSJA3Version = tls.VersionTLS12
)

var (
	// NodeJS22ClientHelloSpec defines the Node.js v22 TLS fingerprint.
	NodeJS22ClientHelloSpec = newNodeJS22ClientHelloSpec()

	nodeJS22CipherSuites = []uint16{
		4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103,
		49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315,
		49311, 49245, 49249, 49239, 49235, 162, 49326, 49324, 49314, 49310,
		49244, 49248, 49238, 49234, 49188, 106, 49187, 64, 49162, 49172,
		57, 56, 49161, 49171, 51, 50, 157, 49313, 49309, 49233,
		156, 49312, 49308, 49232, 61, 60, 53, 47, 255,
	}

	nodeJS22SupportedGroups = []tls.CurveID{
		tls.X25519, tls.CurveP256, tls.CurveID(30), tls.CurveP521, tls.CurveP384, // 30 = X448
		tls.CurveID(256), tls.CurveID(257), tls.CurveID(258), tls.CurveID(259), tls.CurveID(260),
	}

	nodeJS22PointFormats = []byte{0, 1, 2}
)

func newNodeJS22ClientHelloSpec() *tls.ClientHelloSpec {
	return &tls.ClientHelloSpec{
		TLSVersMin:         tls.VersionTLS12,
		TLSVersMax:         tls.VersionTLS13,
		CipherSuites:       append([]uint16(nil), nodeJS22CipherSuites...),
		CompressionMethods: []byte{0},
		Extensions: []tls.TLSExtension{
			&tls.SNIExtension{},
			&tls.SupportedPointsExtension{SupportedPoints: append([]byte(nil), nodeJS22PointFormats...)},
			&tls.SupportedCurvesExtension{Curves: append([]tls.CurveID(nil), nodeJS22SupportedGroups...)},
			&tls.SessionTicketExtension{},
			&tls.GenericExtension{Id: 22}, // encrypt_then_mac
			&tls.ExtendedMasterSecretExtension{},
			&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: defaultSignatureSchemes()},
			&tls.SupportedVersionsExtension{Versions: []uint16{tls.VersionTLS13, tls.VersionTLS12}},
			&tls.PSKKeyExchangeModesExtension{Modes: []uint8{1}},
			&tls.KeyShareExtension{KeyShares: []tls.KeyShare{{Group: tls.X25519}}},
		},
	}
}

func defaultSignatureSchemes() []tls.SignatureScheme {
	return []tls.SignatureScheme{
		tls.ECDSAWithP256AndSHA256,
		tls.ECDSAWithP384AndSHA384,
		tls.ECDSAWithP521AndSHA512,
		tls.ECDSAWithSHA1,
		tls.PSSWithSHA256,
		tls.PSSWithSHA384,
		tls.PSSWithSHA512,
		tls.PKCS1WithSHA256,
		tls.PKCS1WithSHA384,
		tls.PKCS1WithSHA512,
		tls.PKCS1WithSHA1,
	}
}

// CloneNodeJS22ClientHelloSpec returns an isolated copy so callers can safely mutate SNI or session settings.
func CloneNodeJS22ClientHelloSpec(serverName string) *tls.ClientHelloSpec {
	spec := newNodeJS22ClientHelloSpec()
	if serverName == "" {
		return spec
	}

	for _, ext := range spec.Extensions {
		if sniExt, ok := ext.(*tls.SNIExtension); ok {
			sniExt.ServerName = serverName
			break
		}
	}
	return spec
}

// ComputeJA3 derives the JA3 text and hash for the provided ClientHello spec.
// JA3 uses the client_version field from ClientHello, which is 0x0303 (771 = TLS 1.2)
// for TLS 1.3 compatibility mode.
func ComputeJA3(spec *tls.ClientHelloSpec) (string, string, error) {
	if spec == nil {
		return "", "", errors.New("client hello spec is nil")
	}
	if spec.TLSVersMax == 0 {
		return "", "", errors.New("TLSVersMax is not set")
	}

	ciphers := joinUint16(spec.CipherSuites)
	extensions, curves, points := extractJA3Fields(spec.Extensions)
	// JA3 uses the client_version field, which is 0x0303 (TLS 1.2) for TLS 1.3 compatibility
	ja3Text := strings.Join([]string{
		strconv.Itoa(int(nodeJSJA3Version)),
		strings.Join(ciphers, "-"),
		strings.Join(extensions, "-"),
		strings.Join(curves, "-"),
		strings.Join(points, "-"),
	}, ",")

	hash := md5.Sum([]byte(ja3Text))
	return ja3Text, hex.EncodeToString(hash[:]), nil
}

func extractJA3Fields(exts []tls.TLSExtension) (extensionIDs, curves, points []string) {
	for _, ext := range exts {
		switch e := ext.(type) {
		case *tls.SNIExtension:
			extensionIDs = append(extensionIDs, "0")
		case *tls.SupportedPointsExtension:
			extensionIDs = append(extensionIDs, "11")
			for _, p := range e.SupportedPoints {
				points = append(points, strconv.Itoa(int(p)))
			}
		case *tls.SupportedCurvesExtension:
			extensionIDs = append(extensionIDs, "10")
			for _, c := range e.Curves {
				curves = append(curves, strconv.Itoa(int(c)))
			}
		case *tls.SessionTicketExtension:
			extensionIDs = append(extensionIDs, "35")
		case *tls.GenericExtension:
			extensionIDs = append(extensionIDs, strconv.Itoa(int(e.Id)))
		case *tls.ExtendedMasterSecretExtension:
			extensionIDs = append(extensionIDs, "23")
		case *tls.SignatureAlgorithmsExtension:
			extensionIDs = append(extensionIDs, "13")
		case *tls.SupportedVersionsExtension:
			extensionIDs = append(extensionIDs, "43")
		case *tls.PSKKeyExchangeModesExtension:
			extensionIDs = append(extensionIDs, "45")
		case *tls.KeyShareExtension:
			extensionIDs = append(extensionIDs, "51")
		}
	}
	return
}

func joinUint16(values []uint16) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strconv.Itoa(int(v))
	}
	return out
}
