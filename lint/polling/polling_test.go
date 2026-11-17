package polling

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestPolling checks sleeps, tickers and looped time.After, and accepts a
// one-shot deadline, marked timers, function literals and tests.
func TestPolling(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), NewAnalyzer(), "polling")
}
