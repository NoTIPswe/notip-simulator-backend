package generator

import (
	"math"
	"sync"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type SineWaveGenerator struct {
	minRange        float64
	maxRange        float64
	periodSeconds   float64
	startTime       time.Time
	clock           ports.Nower
	outlierOverride *float64
	mu              sync.Mutex
}

func NewSineWaveGenerator(min, max, period float64, clk ports.Nower) *SineWaveGenerator {
	if period <= 0 {
		period = 60 // default to 60 seconds if invalid period is provided
	}
	return &SineWaveGenerator{
		minRange:      min,
		maxRange:      max,
		periodSeconds: period,
		startTime:     clk.Now(),
		clock:         clk,
	}
}

func (g *SineWaveGenerator) Next() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.outlierOverride != nil {
		val := *g.outlierOverride
		g.outlierOverride = nil
		return val
	}

	elapsed := g.clock.Now().Sub(g.startTime).Seconds()
	amplitude := (g.maxRange - g.minRange) / 2
	offset := g.minRange + amplitude
	value := offset + amplitude*math.Sin(2*math.Pi*elapsed/g.periodSeconds)
	return value
}

func (g *SineWaveGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.outlierOverride = &value
}
