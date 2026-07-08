package analytics

import (
	"math"
	"testing"
)

func TestWilsonLower(t *testing.T) {
	got := wilsonLower(2, 2, wilsonZ)
	if math.Abs(got-0.3424) > 0.001 {
		t.Fatalf("wilsonLower(2,2) = %v, want ~0.342", got)
	}
	if wilsonLower(0, 0, wilsonZ) != 0 {
		t.Fatalf("wilsonLower(0,0) must be 0")
	}
	// More games at the same rate → higher (more confident) lower bound.
	if wilsonLower(20, 20, wilsonZ) <= wilsonLower(2, 2, wilsonZ) {
		t.Fatalf("more evidence should raise the lower bound")
	}
}

func TestShrink(t *testing.T) {
	// 100% over 2 games with mu=0.5 barely moves off the mean.
	got := shrink(2, 2, 0.5, shrinkK)
	if math.Abs(got-0.5833) > 0.001 {
		t.Fatalf("shrink(2,2,0.5,10) = %v, want ~0.583", got)
	}
	// A card at the mean stays at the mean regardless of sample.
	if math.Abs(shrink(5, 10, 0.5, shrinkK)-0.5) > 1e-9 {
		t.Fatalf("shrink at the mean should equal the mean")
	}
}

func TestRatePtr(t *testing.T) {
	if ratePtr(0, 0) != nil {
		t.Fatalf("ratePtr with zero games should be nil")
	}
	if r := ratePtr(1, 2); r == nil || *r != 0.5 {
		t.Fatalf("ratePtr(1,2) = %v, want 0.5", r)
	}
}
