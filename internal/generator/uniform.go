package generator

import (
	"math/rand/v2"
	"sync"
)

type UniformRandomGenerator struct {
	minRange        float64
	maxRange        float64
	outlierOverride *float64
	mu              sync.Mutex
}

func NewUniformRandomGenerator(minRange, maxRange float64) *UniformRandomGenerator {
	return &UniformRandomGenerator{
		minRange: minRange,
		maxRange: maxRange,
	}
}

func (g *UniformRandomGenerator) Next() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.outlierOverride != nil {
		val := *g.outlierOverride
		g.outlierOverride = nil
		return val
	}

	return g.minRange + rand.Float64()*(g.maxRange-g.minRange)
}

func (g *UniformRandomGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.outlierOverride = &value
}
