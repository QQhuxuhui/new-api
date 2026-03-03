package service

import (
	"net/http"
	"testing"
	"time"
)

func TestApplyHTTPTransportTuning_Defaults(t *testing.T) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	applyHTTPTransportTuning(base)

	if base.MaxIdleConns != 1024 {
		t.Fatalf("MaxIdleConns = %d, want %d", base.MaxIdleConns, 1024)
	}
	if base.MaxIdleConnsPerHost != 64 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", base.MaxIdleConnsPerHost, 64)
	}
	if base.MaxConnsPerHost != 0 {
		t.Fatalf("MaxConnsPerHost = %d, want %d", base.MaxConnsPerHost, 0)
	}
	if base.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want %v", base.IdleConnTimeout, 90*time.Second)
	}
	if base.ResponseHeaderTimeout != 0 {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", base.ResponseHeaderTimeout, 0)
	}
}

func TestApplyHTTPTransportTuning_EnvOverrides(t *testing.T) {
	t.Setenv("HTTP_MAX_IDLE_CONNS", "10")
	t.Setenv("HTTP_MAX_IDLE_CONNS_PER_HOST", "5")
	t.Setenv("HTTP_MAX_CONNS_PER_HOST", "7")
	t.Setenv("HTTP_IDLE_CONN_TIMEOUT_SECONDS", "12")
	t.Setenv("HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS", "9")

	base := http.DefaultTransport.(*http.Transport).Clone()
	applyHTTPTransportTuning(base)

	if base.MaxIdleConns != 10 {
		t.Fatalf("MaxIdleConns = %d, want %d", base.MaxIdleConns, 10)
	}
	if base.MaxIdleConnsPerHost != 5 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", base.MaxIdleConnsPerHost, 5)
	}
	if base.MaxConnsPerHost != 7 {
		t.Fatalf("MaxConnsPerHost = %d, want %d", base.MaxConnsPerHost, 7)
	}
	if base.IdleConnTimeout != 12*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want %v", base.IdleConnTimeout, 12*time.Second)
	}
	if base.ResponseHeaderTimeout != 9*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", base.ResponseHeaderTimeout, 9*time.Second)
	}
}
