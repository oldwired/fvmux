package ring

import (
	"reflect"
	"testing"
)

func TestRing_Empty(t *testing.T) {
	r := New[int](4)
	if r.Len() != 0 {
		t.Fatalf("Len() = %d; want 0", r.Len())
	}
	if r.Cap() != 4 {
		t.Fatalf("Cap() = %d; want 4", r.Cap())
	}
	if got := r.Items(); len(got) != 0 {
		t.Fatalf("Items() = %v; want empty", got)
	}
}

func TestRing_FillsWithoutWrap(t *testing.T) {
	r := New[int](3)
	r.Push(1)
	r.Push(2)
	if r.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", r.Len())
	}
	if got, want := r.Items(), []int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() = %v; want %v", got, want)
	}
}

func TestRing_ExactCapacity(t *testing.T) {
	r := New[int](3)
	r.Push(1)
	r.Push(2)
	r.Push(3)
	if r.Len() != 3 {
		t.Fatalf("Len() = %d; want 3", r.Len())
	}
	if got, want := r.Items(), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() = %v; want %v", got, want)
	}
}

func TestRing_WrapAroundKeepsNewestOldestFirst(t *testing.T) {
	r := New[int](3)
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	// Capacity 3, pushed 1..5 → oldest surviving is 3.
	if r.Len() != 3 {
		t.Fatalf("Len() = %d; want 3", r.Len())
	}
	if got, want := r.Items(), []int{3, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() = %v; want %v", got, want)
	}
	// One more push slides the window forward again.
	r.Push(6)
	if got, want := r.Items(), []int{4, 5, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() after extra push = %v; want %v", got, want)
	}
}

func TestRing_CapacityOne(t *testing.T) {
	r := New[string](1)
	r.Push("a")
	r.Push("b")
	if got, want := r.Items(), []string{"b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() = %v; want %v", got, want)
	}
}

func TestRing_ClampsNonPositiveCapacity(t *testing.T) {
	r := New[int](0)
	if r.Cap() != 1 {
		t.Fatalf("Cap() = %d; want 1 (clamped)", r.Cap())
	}
	r.Push(7)
	r.Push(8)
	if got, want := r.Items(), []int{8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Items() = %v; want %v", got, want)
	}
}
