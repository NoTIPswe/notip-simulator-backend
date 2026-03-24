package generator

import (
	"math/rand"
	"sync"
	"time"
)

type UniformRandomGenerator struct {
	minRange        float64
	maxRange        float64
	rng             *rand.Rand
	outlierOverride *float64
	mu              sync.Mutex
}

func NewUniformRandomGenerator(minRange, maxRange float64) *UniformRandomGenerator {
	source := rand.NewSource(time.Now().UnixNano())
	return &UniformRandomGenerator{
		minRange: minRange,
		maxRange: maxRange,
		rng:      rand.New(source),
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

	return g.minRange + g.rng.Float64()*(g.maxRange-g.minRange)
}

func (g *UniformRandomGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.outlierOverride = &value
}
