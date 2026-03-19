package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

type ProvisioningServiceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewProvisioningServiceClient(baseURL string) *ProvisioningServiceClient {
	return &ProvisioningServiceClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{},
	}
}

type onboardRequest struct {
	FactoryID           string `json:"factoryId"`
	FactoryKey          string `json:"factoryKey"`
	TenantID            string `json:"tenantId"`
	ManagementGatewayID string `json:"managementGatewayId"`
	CSR                 string `json:"csr"`
}

type onboardResponse struct {
	CertPEM string `json:"certPem"`
	AESKey  string `json:"aesKey"`
}

func (c *ProvisioningServiceClient) Onboard(
	ctx context.Context,
	factoryID string,
	factoryKey string,
	tenantID string,
	managementGatewayID uuid.UUID,
) (domain.ProvisionResult, error) {
	// Generate key pair and CSR
	keyPEM, csrPEM, err := c.generateKeypairAndCSR(tenantID, managementGatewayID)
	if err != nil {
		return domain.ProvisionResult{}, fmt.Errorf("generate keypair and CSR: %w", err)
	}

	// Prepare request
	reqBody := onboardRequest{
		FactoryID:           factoryID,
		FactoryKey:          factoryKey,
		TenantID:            tenantID,
		ManagementGatewayID: managementGatewayID.String(),
		CSR:                 string(csrPEM),
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

	if resp.StatusCode != http.StatusOK {
		return domain.ProvisionResult{}, fmt.Errorf("onboard request failed with status: %d", resp.StatusCode)
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
		CertPEM:       []byte(onboardResp.CertPEM),
		PrivateKeyPEM: keyPEM,
		AESKey:        encKey,
	}, nil
}

func (c *ProvisioningServiceClient) generateKeypairAndCSR(tenantID string, managementGatewayID uuid.UUID) (keyPEM, csrPEM []byte, err error) {
	// Generate ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ECDSA key: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal EC private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	// Create CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   managementGatewayID.String(),
			Organization: []string{tenantID},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
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
