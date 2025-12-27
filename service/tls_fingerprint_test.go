package service

import (
	"reflect"
	"strconv"
	"testing"

	tls "github.com/refraction-networking/utls"
)

func TestJA3(t *testing.T) {
	spec := CloneNodeJS22ClientHelloSpec("example.com")

	// TLSVersMax should be TLS 1.3 for proper negotiation
	if spec.TLSVersMax != tls.VersionTLS13 {
		t.Fatalf("unexpected TLSVersMax: got %d want %d", spec.TLSVersMax, tls.VersionTLS13)
	}
	// TLSVersMin should be TLS 1.2
	if spec.TLSVersMin != tls.VersionTLS12 {
		t.Fatalf("unexpected TLSVersMin: got %d want %d", spec.TLSVersMin, tls.VersionTLS12)
	}

	if got, want := len(spec.CipherSuites), len(nodeJS22CipherSuites); got != want {
		t.Fatalf("cipher suite count mismatch: got %d want %d", got, want)
	}
	if !reflect.DeepEqual(spec.CipherSuites, nodeJS22CipherSuites) {
		t.Fatalf("cipher suites do not match expected order")
	}

	extIDs, curves, points := extractJA3Fields(spec.Extensions)
	expectedExts := []string{"0", "11", "10", "35", "22", "23", "13", "43", "45", "51"}
	if !reflect.DeepEqual(extIDs, expectedExts) {
		t.Fatalf("extension order mismatch: got %v want %v", extIDs, expectedExts)
	}

	expectedCurves := make([]string, len(nodeJS22SupportedGroups))
	for i, c := range nodeJS22SupportedGroups {
		expectedCurves[i] = strconv.Itoa(int(c))
	}
	if !reflect.DeepEqual(curves, expectedCurves) {
		t.Fatalf("supported groups mismatch: got %v want %v", curves, expectedCurves)
	}

	expectedPoints := []string{"0", "1", "2"}
	if !reflect.DeepEqual(points, expectedPoints) {
		t.Fatalf("point formats mismatch: got %v want %v", points, expectedPoints)
	}

	ja3Text, ja3Hash, err := ComputeJA3(spec)
	if err != nil {
		t.Fatalf("ComputeJA3 error: %v", err)
	}

	expectedJA3Text := "771,4866-4867-4865-49199-49195-49200-49196-158-49191-103-49192-107-163-159-52393-52392-52394-49327-49325-49315-49311-49245-49249-49239-49235-162-49326-49324-49314-49310-49244-49248-49238-49234-49188-106-49187-64-49162-49172-57-56-49161-49171-51-50-157-49313-49309-49233-156-49312-49308-49232-61-60-53-47-255,0-11-10-35-22-23-13-43-45-51,29-23-30-25-24-256-257-258-259-260,0-1-2"
	if ja3Text != expectedJA3Text {
		t.Fatalf("unexpected JA3 text:\n got: %s\nwant: %s", ja3Text, expectedJA3Text)
	}

	if ja3Hash != "0cce74b0d9b7f8528fb2181588d23793" {
		t.Fatalf("unexpected JA3 hash: got %s", ja3Hash)
	}

	foundSNI := false
	for _, ext := range spec.Extensions {
		if sni, ok := ext.(*tls.SNIExtension); ok {
			foundSNI = true
			if sni.ServerName != "example.com" {
				t.Fatalf("SNI not set on clone: got %s", sni.ServerName)
			}
		}
	}
	if !foundSNI {
		t.Fatalf("SNI extension missing in spec")
	}
}

func TestComputeJA3NilSpec(t *testing.T) {
	if _, _, err := ComputeJA3(nil); err == nil {
		t.Fatalf("expected error when spec is nil")
	}
}
