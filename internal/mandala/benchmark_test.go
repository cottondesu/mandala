package mandala

import (
	"fmt"
	"testing"
)

var benchmarkText string
var benchmarkGaps []Gap

func benchmarkState(b *testing.B, size int) State {
	b.Helper()
	s, err := NewState("benchmark")
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < size; i++ {
		id := string(rune('a' + i%8))
		if i >= 8 {
			id += "." + string(rune('a'+(i-8)/8))
		}
		if err := s.Add(id, false); err != nil {
			b.Fatal(err)
		}
	}
	return s
}

func BenchmarkLoadState(b *testing.B) {
	for _, size := range []int{10, 72} {
		b.Run(fmt.Sprintf("%d-nodes", size), func(b *testing.B) {
			root := b.TempDir()
			if err := Init(root, benchmarkState(b, size)); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Load(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFindGaps(b *testing.B) {
	for _, size := range []int{10, 72} {
		b.Run(fmt.Sprintf("%d-nodes", size), func(b *testing.B) {
			s := benchmarkState(b, size)
			b.ReportAllocs()
			for b.Loop() {
				benchmarkGaps = s.Gaps(true)
			}
		})
	}
}

func BenchmarkRenderStatus(b *testing.B) {
	for _, size := range []int{10, 72} {
		b.Run(fmt.Sprintf("%d-nodes", size), func(b *testing.B) {
			s := benchmarkState(b, size)
			b.ReportAllocs()
			for b.Loop() {
				benchmarkText = renderStatus(s)
			}
		})
	}
}
