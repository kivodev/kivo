//go:build race

package query_test

// raceDetector says the tests run under the race detector, where each
// statement takes many times longer.
const raceDetector = true
