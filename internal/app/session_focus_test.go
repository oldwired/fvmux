package app

import "testing"

func TestNearestRestoredIndexUsesOriginalWindowPositions(t *testing.T) {
	tests := []struct {
		name      string
		active    int
		total     int
		restored  []int
		wantIndex int
	}{
		{name: "exact", active: 3, total: 5, restored: []int{0, 3, 4}, wantIndex: 1},
		{name: "nearest before", active: 2, total: 6, restored: []int{0, 1, 5}, wantIndex: 1},
		{name: "nearest after", active: 4, total: 6, restored: []int{0, 5}, wantIndex: 1},
		{name: "tie prefers earlier original", active: 2, total: 5, restored: []int{1, 3}, wantIndex: 0},
		{name: "no survivors", active: 0, total: 1, restored: nil, wantIndex: -1},
		{name: "invalid negative", active: -1, total: 2, restored: []int{0}, wantIndex: -1},
		{name: "invalid high", active: 2, total: 2, restored: []int{0}, wantIndex: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nearestRestoredIndex(tt.active, tt.total, tt.restored); got != tt.wantIndex {
				t.Fatalf("nearestRestoredIndex(%d, %d, %v) = %d, want %d",
					tt.active, tt.total, tt.restored, got, tt.wantIndex)
			}
		})
	}
}
