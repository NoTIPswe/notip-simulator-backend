package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

type ProvisioningServiceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewProvisioningServiceClient(baseURL string) *ProvisioningServiceClient {
	return &ProvisioningServiceClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type factoryCredentials struct {
	FactoryID  string `json:"factoryId"`
	FactoryKey string `json:"factoryKey"`
}

type onboardRequest struct {
	Credentials     factoryCredentials `json:"credentials"`
	CSR             string             `json:"csr"`
	SendFrequencyMs int                `json:"sendFrequencyMs"`
	FirmwareVersion string             `json:"firmwareVersion"`
}

type gatewayIdentity struct {
	GatewayID string `json:"gatewayId"`
	TenantID  string `json:"tenantId"`
}

type onboardResponse struct {
	CertPEM         string          `json:"certPem"`
	AESKey          string          `json:"aesKey"`
	Identity        gatewayIdentity `json:"identity"`
	SendFrequencyMs int             `json:"sendFrequencyMs"`
}

func (c *ProvisioningServiceClient) Onboard(
	ctx context.Context,
	factoryID string,
	factoryKey string,
	sendFrequencyMs int,
	firmwareVersion string,
) (domain.ProvisionResult, error) {
	// Generate key pair and CSR — identity is assigned by the provisioning service
	keyPEM, csrPEM, err := c.generateKeypairAndCSR()
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("generate keypair and CSR: %w", err)
	}

	// Prepare request
	reqBody := onboardRequest{
		Credentials: factoryCredentials{
			FactoryID:  factoryID,
			FactoryKey: factoryKey,
		},
		CSR:             string(csrPEM),
		SendFrequencyMs: sendFrequencyMs,
		FirmwareVersion: firmwareVersion,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("marshal onboard request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/provision/onboard", bytes.NewReader(body))
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("create onboard request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("send onboard request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("failed to close HTTP response body", "error", closeErr)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(resp.Body)
		responseText := strings.TrimSpace(string(responseBody))

		switch resp.StatusCode {
		case http.StatusConflict:
			if responseText == "" {
				return domain.ProvisionResult{}, domain.ErrGatewayAlreadyProvisioned
			}
			return domain.ProvisionResult{}, fmt.Errorf("%w: %s", domain.ErrGatewayAlreadyProvisioned, responseText)
		case http.StatusUnauthorized:
			if responseText == "" {
				return domain.ProvisionResult{}, domain.ErrInvalidFactoryCredentials
			}
			return domain.ProvisionResult{}, fmt.Errorf("%w: %s", domain.ErrInvalidFactoryCredentials, responseText)
		default:
			if responseText == "" {
				return domain.ProvisionResult{}, fmt.Errorf("onboard request failed with status: %d", resp.StatusCode)
			}
			return domain.ProvisionResult{}, fmt.Errorf("onboard request failed with status: %d: %s", resp.StatusCode, responseText)
		}
	}

	var onboardResp onboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&onboardResp); err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("decode onboard response: %w", err)
	}

	aesKeyBytes, err := decodeBase64Key(onboardResp.AESKey)
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("decode AES key: %w", err)
	}

	encKey, err := domain.NewEncryptionKey(aesKeyBytes)
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("create encryption key: %w", err)
	}

	return domain.ProvisionResult{
		CertPEM:         []byte(onboardResp.CertPEM),
		PrivateKeyPEM:   keyPEM,
		AESKey:          encKey,
		GatewayID:       onboardResp.Identity.GatewayID,
		TenantID:        onboardResp.Identity.TenantID,
		SendFrequencyMs: onboardResp.SendFrequencyMs,
	}, nil
}

func (c *ProvisioningServiceClient) generateKeypairAndCSR() (keyPEM, csrPEM []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Subject is intentionally empty — identity is embedded by the provisioning service when signing
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}
	csrPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return keyPEM, csrPEM, nil
}

func decodeBase64Key(encodedKey string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encodedKey)
}
