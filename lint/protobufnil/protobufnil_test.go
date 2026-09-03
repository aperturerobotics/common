package protobufnil

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), &analysis.Analyzer{
		Name: "protobufnil",
		Doc:  "test",
		Run:  run,
	}, "a")
}
