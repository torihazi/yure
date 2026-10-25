package japanmap

import (
	"math"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderSkipsBogusRings(t *testing.T) {
	m := New(40, 20)
	rings := []Ring{
		{Lon: 140, Lat: 36, Radius: math.Inf(1)},
		{Lon: 140, Lat: 36, Radius: math.NaN()},
		{Lon: 140, Lat: 36, Radius: 1e300},
	}
	done := make(chan struct{})
	go func() {
		m.Render(lipgloss.NewStyle(), rings, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Render did not return")
	}
}
