package ordnung

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestImage_GenerateNewName(t *testing.T) {
	d := time.Now().UTC()
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
			fmt.Sprintf("%v_0000.jpg", d.Format("2006-01-02")),
		},
		{
			"YYYY/MM-DD seen",
			Image{"sample.jpg", "", d, true},
			"YYYY/MM-DD",
			&map[string]int{d.Format("2006/01-02"): 1},
			fmt.Sprintf("%v_0002.jpg", d.Format("2006/01-02")),
		},
		{
			"unsupported pattern",
			Image{"sample.jpg", "", d, true},
			"MM-DD",
			&map[string]int{},
			fmt.Sprintf("%v_0000.jpg", d.Format("2006-01-02")),
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
