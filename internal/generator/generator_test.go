package generator_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/generator"
)

func makeSensor(algo domain.GenerationAlgorithmType, min, max float64) *domain.SimSensor {
	return &domain.SimSensor{
		SensorID:  uuid.New(),
		Type:      domain.Temperature,
		MinRange:  min,
		MaxRange:  max,
		Algorithm: algo,
	}
}

//UniformRandom.

func TestUniformRandomInRange(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.UniformRandom, 10, 20), clk)

	for i := 0; i < 200; i++ {
		v := g.Next()
		if v < 10 || v > 20 {
			t.Errorf("iteration %d: value %.4f out of range [10, 20]", i, v)
		}
	}
}

func TestUniformRandomInjectOutlierConsumedOnce(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.UniformRandom, 0, 1), clk)

	g.InjectOutlier(999.9)

	first := g.Next()
	if first != 999.9 {
		t.Errorf("first call after InjectOutlier: want 999.9, got %.4f", first)
	}
	//After first the values should be normal again.
	second := g.Next()
	if second == 999.9 {
		t.Error("outlier should be consumed after first Next() call")
	}
}

func TestUniformRandomValuesAreNotAllIdentical(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.UniformRandom, 0, 100), clk)

	seen := map[float64]bool{}
	for range 50 {
		seen[g.Next()] = true
	}
	if len(seen) < 2 {
		t.Error("UniformRandom should produce different values")
	}
}

//SineWave.

func TestSineWaveInRange(t *testing.T) {
	clk := fakes.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.SineWave, 0, 100), clk)

	for i := 0; i < 60; i++ {
		v := g.Next()
		if v < 0 || v > 100 {
			t.Errorf("tick %d: sine value %.4f out of range [0, 100]", i, v)
		}
		clk.Advance(time.Second)
	}
}

func TestSineWaveOscillates(t *testing.T) {
	clk := fakes.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.SineWave, 0, 100), clk)

	vals := make([]float64, 30)
	for i := range vals {
		vals[i] = g.Next()
		clk.Advance(2 * time.Second)
	}
	allSame := true
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("SineWave should produce different values over time")
	}
}

func TestSineWaveInjectOutlier(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.SineWave, 0, 100), clk)

	g.InjectOutlier(777.7)
	v := g.Next()
	if v != 777.7 {
		t.Errorf("want outlier 777.7, got %.4f", v)
	}
}

//Spike.

func TestSpikeMostlyInRange(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Spike, 0, 100), clk)

	inRange := 0
	total := 500
	for i := 0; i < total; i++ {
		v := g.Next()
		if v >= 0 && v <= 100 {
			inRange++
		}
	}
	// At least 70% must be in range.
	pct := float64(inRange) / float64(total) * 100
	if pct < 70 {
		t.Errorf("expected >=70%% values in range, got %.1f%%", pct)
	}
}

func TestSpikeInjectOutlier(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Spike, 0, 100), clk)

	g.InjectOutlier(500.0)
	v := g.Next()
	if v != 500.0 {
		t.Errorf("want outlier 500.0, got %.4f", v)
	}
}

func TestSpikeProducesSomeSpikedValues(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Spike, 0, 10), clk)

	outOfRange := 0
	for range 2000 {
		v := g.Next()
		if v < 0 || v > 10 {
			outOfRange++
		}
	}
	//With a Spike frequency > 0 we should have some spike.
	if outOfRange == 0 {
		t.Error("expected at least some spike values out of range over 2000 iterations")
	}
}

// SineWave direct constructor.

func TestSineWaveGeneratorZeroPeriodDefaultsToSixty(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	g := generator.NewSineWaveGenerator(0, 100, 0, clk)
	// With period defaulted to 60 the value must still be in range.
	v := g.Next()
	if v < 0 || v > 100 {
		t.Errorf("value %.4f out of range [0, 100] with default period", v)
	}
}

func TestSineWaveGeneratorNegativePeriodDefaultsToSixty(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	g := generator.NewSineWaveGenerator(0, 100, -5, clk)
	v := g.Next()
	if v < 0 || v > 100 {
		t.Errorf("value %.4f out of range [0, 100] with default period", v)
	}
}

//Constant.

func TestConstantAlwaysReturnsSameValue(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Constant, 0, 100), clk)

	first := g.Next()
	for i := 0; i < 50; i++ {
		v := g.Next()
		if v != first {
			t.Errorf("iteration %d: constant returned different values %.4f vs %.4f", i, first, v)
		}
	}
}

func TestConstantInjectOutlierThenReturnsConstant(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Constant, 42, 42), clk)

	g.InjectOutlier(100.0)
	v1 := g.Next() // outlier
	if v1 != 100.0 {
		t.Errorf("want outlier 100.0, got %.4f", v1)
	}
	//After it consumes v1 it should be constant again.
	v2 := g.Next()
	if v2 == 100.0 {
		t.Error("constant should resume after outlier is consumed")
	}
}

//GeneratorFactory.

func TestFactoryAllAlgorithmsReturnNonNil(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	algos := []domain.GenerationAlgorithmType{
		domain.UniformRandom,
		domain.SineWave,
		domain.Spike,
		domain.Constant,
	}
	for _, algo := range algos {
		g := factory.New(makeSensor(algo, 0, 100), clk)
		if g == nil {
			t.Errorf("expected non-nil generator for algorithm %v", algo)
		}
	}
}

func TestFactoryEachAlgorithmProducesValues(t *testing.T) {
	clk := fakes.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	factory := generator.NewGeneratorFactory()

	algos := []domain.GenerationAlgorithmType{
		domain.UniformRandom,
		domain.SineWave,
		domain.Spike,
		domain.Constant,
	}

	for _, algo := range algos {
		g := factory.New(makeSensor(algo, 0, 100), clk)

		// No panic on Next()
		v := g.Next()
		_ = v
		clk.Advance(time.Second)
	}
}
