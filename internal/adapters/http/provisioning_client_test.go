package http_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	simhttp "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
)

const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
	testGatewayID     = "11111111-1111-1111-1111-111111111111"
	testTenantID      = "tenant-1"
	testFWVersion     = "fw-1.0"
)

func makeValidAESKey() string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestProvisioningClientOnboardSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/provision/onboard" {
			t.Errorf("want /api/provision/onboard, got %s", r.URL.Path)
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certPem": "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
			"aesKey":  makeValidAESKey(),
			"identity": map[string]string{
				"gatewayId": testGatewayID,
				"tenantId":  testTenantID,
			},
			"sendFrequencyMs": 100,
		})
	}))
	defer srv.Close()

	client := simhttp.NewProvisioningServiceClient(srv.URL)
	result, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CertPEM) == 0 {
		t.Error("expected non-empty CertPEM")
	}
	if len(result.PrivateKeyPEM) == 0 {
		t.Error("expected non-empty PrivateKeyPEM")
	}
	if result.GatewayID == "" {
		t.Error("expected non-empty GatewayID")
	}
	if result.TenantID == "" {
		t.Error("expected non-empty TenantID")
	}
}

func TestProvisioningClientOnboardServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := simhttp.NewProvisioningServiceClient(srv.URL)
	_, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err == nil {
		t.Error("expected error on 500, got nil")
	}
}

func TestProvisioningClientOnboardInvalidAESKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certPem": "fake-cert",
			"aesKey":  base64.StdEncoding.EncodeToString([]byte("short")),
			"identity": map[string]string{
				"gatewayId": testGatewayID,
				"tenantId":  testTenantID,
			},
		})
	}))
	defer srv.Close()

	client := simhttp.NewProvisioningServiceClient(srv.URL)
	_, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err == nil {
		t.Error("expected error for invalid AES key length, got nil")
	}
}

func TestProvisioningClientOnboardBadURL(t *testing.T) {
	client := simhttp.NewProvisioningServiceClient("http://localhost:0")
	_, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

func TestProvisioningClientOnboardBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	client := simhttp.NewProvisioningServiceClient(srv.URL)
	_, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err == nil {
		t.Error("expected error for invalid JSON response, got nil")
	}
}

func TestProvisioningClientOnboardInvalidBase64Key(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certPem": "fake-cert",
			"aesKey":  "not-valid-base64!!!",
			"identity": map[string]string{
				"gatewayId": testGatewayID,
				"tenantId":  testTenantID,
			},
		})
	}))
	defer srv.Close()

	client := simhttp.NewProvisioningServiceClient(srv.URL)
	_, err := client.Onboard(context.Background(), "fid", "fkey", 100, testFWVersion)
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}
