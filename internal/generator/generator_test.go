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

func TestUniformRandom_InRange(t *testing.T) {
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

func TestUniformRandom_InjectOutlier_ConsumedOnce(t *testing.T) {
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

func TestUniformRandom_ValuesAreNotAllIdentical(t *testing.T) {
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

func TestSineWave_InRange(t *testing.T) {
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

func TestSineWave_Oscillates(t *testing.T) {
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

func TestSineWave_InjectOutlier(t *testing.T) {
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

func TestSpike_MostlyInRange(t *testing.T) {
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

func TestSpike_InjectOutlier(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	factory := generator.NewGeneratorFactory()
	g := factory.New(makeSensor(domain.Spike, 0, 100), clk)

	g.InjectOutlier(500.0)
	v := g.Next()
	if v != 500.0 {
		t.Errorf("want outlier 500.0, got %.4f", v)
	}
}

func TestSpike_ProducesSomeSpikedValues(t *testing.T) {
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

//Constant.

func TestConstant_AlwaysReturnsSameValue(t *testing.T) {
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

func TestConstant_InjectOutlier_ThenReturnsConstant(t *testing.T) {
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

func TestFactory_AllAlgorithms_ReturnNonNil(t *testing.T) {
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

func TestFactory_EachAlgorithm_ProducesValues(t *testing.T) {
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
