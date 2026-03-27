package generator

import (
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type GeneratorFactory struct{}

func NewGeneratorFactory() *GeneratorFactory {
	return &GeneratorFactory{}
}

func (f *GeneratorFactory) New(sensor *domain.SimSensor, clock ports.Nower) Generator {
	switch sensor.Algorithm {
	case domain.Constant:
		return NewConstantGenerator(sensor.MinRange, sensor.MaxRange)
	case domain.SineWave:
		return NewSineWaveGenerator(sensor.MinRange, sensor.MaxRange, 60.0, clock)
	case domain.Spike:
		return NewSpikeGenerator(sensor.MinRange, sensor.MaxRange, 0.1, 1.5)
	case domain.UniformRandom:
		fallthrough
	default:
		return NewUniformRandomGenerator(sensor.MinRange, sensor.MaxRange)
	}
}
