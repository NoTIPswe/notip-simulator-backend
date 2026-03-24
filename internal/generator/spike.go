package generator

import (
	"math/rand"
	"sync"
	"time"
)

type SpikeGenerator struct {
	minRange        float64
	maxRange        float64
	spikeFrequency  float64
	spikeFactor     float64
	rng             *rand.Rand
	outlierOverride *float64
	mu              sync.Mutex
}

func NewSpikeGenerator(min, max, freq, factor float64) *SpikeGenerator {
	source := rand.NewSource(time.Now().UnixNano())
	return &SpikeGenerator{
		minRange:       min,
		maxRange:       max,
		spikeFrequency: freq,
		spikeFactor:    factor,
		rng:            rand.New(source),
	}
}

func (g *SpikeGenerator) Next() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.outlierOverride != nil {
		val := *g.outlierOverride
		g.outlierOverride = nil
		return val
	}

	if g.rng.Float64() < g.spikeFrequency {
		rangeSize := g.maxRange - g.minRange
		if g.rng.Float64() < 0.5 {
			return g.maxRange + rangeSize*g.spikeFactor
		}
		return g.minRange - rangeSize*g.spikeFactor
	}

	return g.minRange + g.rng.Float64()*(g.maxRange-g.minRange)
}

func (g *SpikeGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.outlierOverride = &value
}
