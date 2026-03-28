package generator

import "sync"

type ConstantGenerator struct {
	value           float64
	outlierOverride *float64
	mu              sync.RWMutex
}

func NewConstantGenerator(min, max float64) *ConstantGenerator {
	midPoint := (min + max) / 2.0
	return &ConstantGenerator{
		value: midPoint,
	}
}

func (g *ConstantGenerator) Next() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.outlierOverride != nil {
		val := *g.outlierOverride
		g.outlierOverride = nil
		return val
	}
	return g.value
}

func (g *ConstantGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.outlierOverride = &value
}
