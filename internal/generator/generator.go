package generator

// Generator defines how to generate simulated sensor data, including the ability to inject outliers for testing purposes.
type Generator interface {
	// Next calculates and returns the next simulated value.
	Next() float64

	InjectOutlier(val float64)
}
