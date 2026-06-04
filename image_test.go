package ordnung

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestImage_GenerateNewName(t *testing.T) {
	// Use a fixed date so the time-derived suffix is deterministic.
	d := time.Date(2021, 7, 15, 8, 15, 30, 0, time.UTC)

	tests := []struct {
		name     string
		image    Image
		pattern  string
		names    *map[string]int
		expected string
	}{
		{
			"YYYY-MM-DD unseen",
			Image{"sample.jpg", "", d, true},
			"YYYY-MM-DD",
			&map[string]int{},
			fmt.Sprintf("%v_%s-00.jpg", d.Format("2006-01-02"), timeSuffix(d)),
		},
		{
			"YYYY/MM-DD collision in the same second",
			Image{"sample.jpg", "", d, true},
			"YYYY/MM-DD",
			&map[string]int{fmt.Sprintf("%s_%s", d.Format("2006/01-02"), timeSuffix(d)): 1},
			fmt.Sprintf("%v_%s-01.jpg", d.Format("2006/01-02"), timeSuffix(d)),
		},
		{
			"unsupported pattern falls back to YYYY-MM-DD",
			Image{"sample.jpg", "", d, true},
			"MM-DD",
			&map[string]int{},
			fmt.Sprintf("%v_%s-00.jpg", d.Format("2006-01-02"), timeSuffix(d)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.image.GenerateNewName(tt.pattern, tt.names, &sync.Mutex{})
			if tt.image.NewName != tt.expected {
				t.Errorf(
					"expected %v to be the generated name, got %v instead",
					tt.expected, tt.image.NewName,
				)
			}
		})
	}
}

// TestTimeSuffix_LexicographicOrder verifies the core property: images taken
// earlier in the day must produce lexicographically smaller suffixes than
// images taken later, regardless of the order they're processed in.
func TestTimeSuffix_LexicographicOrder(t *testing.T) {
	day := func(h, m, s int) time.Time {
		return time.Date(2021, 7, 15, h, m, s, 0, time.UTC)
	}

	times := []time.Time{
		day(0, 0, 0),
		day(8, 15, 30),
		day(12, 0, 0),
		day(18, 42, 10),
		day(23, 59, 59),
	}

	for i := 1; i < len(times); i++ {
		prev := timeSuffix(times[i-1])
		curr := timeSuffix(times[i])
		if !(prev < curr) {
			t.Errorf(
				"expected suffix for %v (%q) to sort before %v (%q)",
				times[i-1].Format("15:04:05"), prev,
				times[i].Format("15:04:05"), curr,
			)
		}
		if len(curr) != 4 {
			t.Errorf("expected fixed-width suffix of 4 chars, got %q", curr)
		}
	}
}
