package domain_test

import (
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

//EncryptionKey.

func TestNewEncryptionKeyValid32Bytes(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	key, err := domain.NewEncryptionKey(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := key.Bytes(); len(got) != 32 {
		t.Errorf("want 32 bytes, got %d", len(got))
	}
}

func TestNewEncryptionKeyWrongLengthReturnsError(t *testing.T) {
	cases := [][]byte{
		{},
		make([]byte, 16),
		make([]byte, 31),
		make([]byte, 33),
		make([]byte, 64),
	}
	for _, raw := range cases {
		_, err := domain.NewEncryptionKey(raw)
		if err == nil {
			t.Errorf("expected error for key of length %d, got nil", len(raw))
		}
	}
}

func TestEncryptionKeyBytesReturnsCopy(t *testing.T) {
	raw := make([]byte, 32)
	raw[0] = 0xAB
	key, _ := domain.NewEncryptionKey(raw)

	b1 := key.Bytes()
	b1[0] = 0x00 // mutate the returned slice.

	b2 := key.Bytes()
	if b2[0] == 0x00 {
		t.Error("Bytes() should return a copy, not expose internal state")
	}
}

// GatewayStatus.
func TestGatewayStatusValuesDistinct(t *testing.T) {
	statuses := []domain.GatewayStatus{
		domain.Provisioning,
		domain.Online,
		domain.Offline,
		domain.Paused,
	}
	seen := map[domain.GatewayStatus]bool{}
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate GatewayStatus value: %v", s)
		}
		seen[s] = true
	}
}

//SensorType.

func TestSensorTypeAllConstantsDeclared(t *testing.T) {
	types := []domain.SensorType{
		domain.Temperature,
		domain.Humidity,
		domain.Pressure,
		domain.Movement,
		domain.Biometric,
	}
	if len(types) != 5 {
		t.Errorf("expected 5 sensor types, got %d", len(types))
	}
}

// GenerationAlgorithmtype.
func TestGenerationAlgorithmTypeAllConstantsDeclared(t *testing.T) {
	algos := []domain.GenerationAlgorithmType{
		domain.UniformRandom,
		domain.SineWave,
		domain.Spike,
		domain.Constant,
	}
	if len(algos) != 4 {
		t.Errorf("expected 4 algorithm types, got %d", len(algos))
	}
}

//AnomalyType.

func TestAnomalyTypeValuesDistinct(t *testing.T) {
	if domain.NetworkDegradation == domain.Disconnect {
		t.Error("NetworkDegradation and Disconnect must be distinct")
	}
}

//CommandType & CommandACKStatus.

func TestCommandTypeValuesDistinct(t *testing.T) {
	if domain.ConfigUpdate == domain.FirmwarePush {
		t.Error("ConfigUpdate and FirmwarePush must be distinct")
	}
}

func TestCommandACKStatusValuesDistinct(t *testing.T) {
	statuses := []domain.CommandACKStatus{domain.ACK, domain.NACK, domain.Expired}
	seen := map[domain.CommandACKStatus]bool{}
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate CommandACKStatus: %v", s)
		}
		seen[s] = true
	}
}

//Value Objects, zero-value sanity.

func TestTelemetryEnvelopeZeroValueNoPanic(t *testing.T) {
	var e domain.TelemetryEnvelope
	_ = e.GatewayID
	_ = e.SensorID
}

func TestCreateGatewayRequestFields(t *testing.T) {
	r := domain.CreateGatewayRequest{
		FactoryID:       "fid",
		FactoryKey:      "fkey",
		SendFrequencyMs: 1000,
	}
	if r.FactoryID != "fid" {
		t.Error("FactoryID not set")
	}
	if r.FactoryKey != "fkey" {
		t.Error("FactoryKey not set")
	}
	if r.SendFrequencyMs != 1000 {
		t.Error("SendFrequencyMs not set")
	}
}

func TestGatewayConfigUpdateNilPointersNoPanic(t *testing.T) {
	u := domain.GatewayConfigUpdate{}
	if u.SendFrequencyMs != nil {
		t.Error("expected nil SendFrequencyMs")
	}
	if u.Status != nil {
		t.Error("expected nil Status")
	}
}

func TestNetworkDegradationParamsFields(t *testing.T) {
	p := domain.NetworkDegradationParams{DurationSeconds: 30, PacketLossPct: 75.5}
	if p.DurationSeconds != 30 {
		t.Error("DurationSeconds not set")
	}
	if p.PacketLossPct != 75.5 {
		t.Error("PacketLossPct not set")
	}
}

func TestSensorOutlierCommandNilValueNoPanic(t *testing.T) {
	cmd := domain.SensorOutlierCommand{}
	if cmd.Value != nil {
		t.Error("expected nil Value pointer")
	}
}
