package cli

import (
	"bytes"
	"io"
	"testing"
)

func BenchmarkScanHealthyNode(b *testing.B) {
	root := fixture("healthy-node")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out bytes.Buffer
		Main([]string{root, "--format", "json"}, &out, io.Discard)
	}
}

func BenchmarkScanBrokenNode(b *testing.B) {
	root := fixture("broken-node")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out bytes.Buffer
		Main([]string{root, "--format", "json"}, &out, io.Discard)
	}
}

func BenchmarkScanMonorepo(b *testing.B) {
	root := fixture("monorepo")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out bytes.Buffer
		Main([]string{root, "--format", "json"}, &out, io.Discard)
	}
}
