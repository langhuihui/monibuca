package collections_test

import (
	"fmt"
	"sort"
	"testing"

	"m7s.live/v5/pkg/collections"
)

func TestMap(t *testing.T) {
	src := []int{1, 2, 3}
	got := collections.Map(src, func(v int) string { return fmt.Sprintf("%d", v) })
	want := []string{"1", "2", "3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Map[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFilter(t *testing.T) {
	src := []int{1, 2, 3, 4, 5}
	got := collections.Filter(src, func(v int) bool { return v%2 == 0 })
	if len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("Filter = %v, want [2 4]", got)
	}
}

func TestReduce(t *testing.T) {
	src := []int{1, 2, 3, 4}
	sum := collections.Reduce(src, 0, func(acc, v int) int { return acc + v })
	if sum != 10 {
		t.Fatalf("Reduce = %d, want 10", sum)
	}
}

func TestGroupBy(t *testing.T) {
	src := []string{"apple", "avocado", "banana", "blueberry", "cherry"}
	groups := collections.GroupBy(src, func(s string) byte { return s[0] })
	if len(groups['a']) != 2 {
		t.Fatalf("GroupBy 'a' = %v, want 2 items", groups['a'])
	}
	if len(groups['b']) != 2 {
		t.Fatalf("GroupBy 'b' = %v, want 2 items", groups['b'])
	}
}

func TestIndexBy(t *testing.T) {
	type Item struct{ ID int }
	src := []Item{{1}, {2}, {3}}
	m := collections.IndexBy(src, func(i Item) int { return i.ID })
	if m[2].ID != 2 {
		t.Fatalf("IndexBy[2] = %v, want {2}", m[2])
	}
}

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	keys := collections.Keys(m)
	sort.Strings(keys)
	if keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("Keys = %v", keys)
	}
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	vals := collections.Values(m)
	sort.Ints(vals)
	if vals[0] != 1 || vals[1] != 2 {
		t.Fatalf("Values = %v", vals)
	}
}

func TestContains(t *testing.T) {
	src := []int{1, 2, 3}
	if !collections.Contains(src, func(v int) bool { return v == 2 }) {
		t.Fatal("Contains should be true for 2")
	}
	if collections.Contains(src, func(v int) bool { return v == 99 }) {
		t.Fatal("Contains should be false for 99")
	}
}

func TestFind(t *testing.T) {
	src := []int{1, 2, 3}
	v, ok := collections.Find(src, func(v int) bool { return v > 1 })
	if !ok || v != 2 {
		t.Fatalf("Find = %d %v, want 2 true", v, ok)
	}
	_, ok = collections.Find(src, func(v int) bool { return v > 99 })
	if ok {
		t.Fatal("Find should return false for missing element")
	}
}

func TestUnique(t *testing.T) {
	src := []int{1, 2, 2, 3, 1}
	got := collections.Unique(src, func(v int) int { return v })
	if len(got) != 3 {
		t.Fatalf("Unique = %v, want 3 elements", got)
	}
}

func TestFlatten(t *testing.T) {
	src := [][]int{{1, 2}, {3}, {4, 5}}
	got := collections.Flatten(src)
	if len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Fatalf("Flatten = %v", got)
	}
}

func TestChunk(t *testing.T) {
	src := []int{1, 2, 3, 4, 5}
	got := collections.Chunk(src, 2)
	if len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 {
		t.Fatalf("Chunk = %v", got)
	}
}

func TestZip(t *testing.T) {
	a := []int{1, 2, 3}
	b := []string{"a", "b"}
	got := collections.Zip(a, b)
	if len(got) != 2 || got[0].First != 1 || got[0].Second != "a" {
		t.Fatalf("Zip = %v", got)
	}
}
