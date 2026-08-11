package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
)

func TestDoWorkerRequestHTTPPolicyRejectIsClientInput(t *testing.T) {
	previousWorkerURL := system_setting.WorkerUrl
	previousAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	system_setting.WorkerUrl = "https://worker.example.com"
	system_setting.WorkerAllowHttpImageRequestEnabled = false
	t.Cleanup(func() {
		system_setting.WorkerUrl = previousWorkerURL
		system_setting.WorkerAllowHttpImageRequestEnabled = previousAllowHTTP
	})

	_, err := DoWorkerRequest(&WorkerRequest{URL: "http://example.com/image.png"})
	if err == nil {
		t.Fatal("expected worker HTTP policy rejection")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("worker HTTP policy rejection must be a client-input error, got %v", err)
	}
}

func TestDoWorkerRequestDNSFailureRemainsRetryable(t *testing.T) {
	previousWorkerURL := system_setting.WorkerUrl
	previousAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	previousFetchSetting := *system_setting.GetFetchSetting()
	system_setting.WorkerUrl = "https://worker.example.com"
	system_setting.WorkerAllowHttpImageRequestEnabled = false
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowedPorts:         []string{"443"},
	}
	t.Cleanup(func() {
		system_setting.WorkerUrl = previousWorkerURL
		system_setting.WorkerAllowHttpImageRequestEnabled = previousAllowHTTP
		*system_setting.GetFetchSetting() = previousFetchSetting
	})

	_, err := DoWorkerRequest(&WorkerRequest{URL: "https://does-not-resolve.invalid/image.png"})
	if err == nil {
		t.Fatal("expected DNS resolution failure")
	}
	if types.IsClientInputError(err) {
		t.Fatalf("temporary DNS failure must remain retryable, got %v", err)
	}
}

func TestDoWorkerRequestInvalidFetchConfigIsNotClientInput(t *testing.T) {
	previousWorkerURL := system_setting.WorkerUrl
	previousAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	previousFetchSetting := *system_setting.GetFetchSetting()
	system_setting.WorkerUrl = "https://worker.example.com"
	system_setting.WorkerAllowHttpImageRequestEnabled = false
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowedPorts:         []string{"invalid-port"},
	}
	t.Cleanup(func() {
		system_setting.WorkerUrl = previousWorkerURL
		system_setting.WorkerAllowHttpImageRequestEnabled = previousAllowHTTP
		*system_setting.GetFetchSetting() = previousFetchSetting
	})

	_, err := DoWorkerRequest(&WorkerRequest{URL: "https://example.com/image.png"})
	if err == nil {
		t.Fatal("expected invalid fetch configuration error")
	}
	if types.IsClientInputError(err) {
		t.Fatalf("server fetch configuration errors must not be blamed on the client, got %v", err)
	}
}

func TestDoDownloadRequestMalformedURLIsClientInputWhenSSRFDisabled(t *testing.T) {
	previousWorkerURL := system_setting.WorkerUrl
	previousFetchSetting := *system_setting.GetFetchSetting()
	system_setting.WorkerUrl = ""
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() {
		system_setting.WorkerUrl = previousWorkerURL
		*system_setting.GetFetchSetting() = previousFetchSetting
	})

	_, err := DoDownloadRequest("http://example.com:invalid/image.png")
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("malformed URL must be a client-input error even when SSRF is disabled, got %v", err)
	}
}
