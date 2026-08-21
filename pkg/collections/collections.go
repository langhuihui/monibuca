// Package collections provides generic functional helpers for slices and maps,
// removing the need for repetitive hand-written loops across the codebase.
package collections

// Map transforms each element of src using f and returns a new slice.
func Map[T, U any](src []T, f func(T) U) []U {
	out := make([]U, len(src))
	for i, v := range src {
		out[i] = f(v)
	}
	return out
}

// Filter returns a new slice containing only the elements of src for which
// keep returns true.
func Filter[T any](src []T, keep func(T) bool) []T {
	out := make([]T, 0, len(src))
	for _, v := range src {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// Reduce folds src into a single value by repeatedly calling f(accumulator, element).
func Reduce[T, U any](src []T, initial U, f func(U, T) U) U {
	acc := initial
	for _, v := range src {
		acc = f(acc, v)
	}
	return acc
}

// GroupBy partitions src into a map keyed by the result of key(element).
func GroupBy[T any, K comparable](src []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range src {
		k := key(v)
		out[k] = append(out[k], v)
	}
	return out
}

// IndexBy builds a map from src where the map key is derived from each element
// via key. When multiple elements share a key the last one wins.
func IndexBy[T any, K comparable](src []T, key func(T) K) map[K]T {
	out := make(map[K]T, len(src))
	for _, v := range src {
		out[key(v)] = v
	}
	return out
}

// Keys returns the keys of m in unspecified order.
func Keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Values returns the values of m in unspecified order.
func Values[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// Contains reports whether any element of src satisfies pred.
func Contains[T any](src []T, pred func(T) bool) bool {
	for _, v := range src {
		if pred(v) {
			return true
		}
	}
	return false
}

// Find returns the first element that satisfies pred and true, or the zero
// value and false if none is found.
func Find[T any](src []T, pred func(T) bool) (T, bool) {
	for _, v := range src {
		if pred(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// Unique returns a new slice with duplicates removed, preserving order.
// Equality is determined by the key function.
func Unique[T any, K comparable](src []T, key func(T) K) []T {
	seen := make(map[K]struct{}, len(src))
	out := make([]T, 0, len(src))
	for _, v := range src {
		k := key(v)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// MapKeys transforms the keys of a map using f, producing a new map.
func MapKeys[K1, K2 comparable, V any](m map[K1]V, f func(K1) K2) map[K2]V {
	out := make(map[K2]V, len(m))
	for k, v := range m {
		out[f(k)] = v
	}
	return out
}

// MapValues transforms the values of a map using f, producing a new map.
func MapValues[K comparable, V1, V2 any](m map[K]V1, f func(V1) V2) map[K]V2 {
	out := make(map[K]V2, len(m))
	for k, v := range m {
		out[k] = f(v)
	}
	return out
}

// Flatten concatenates a slice of slices into a single slice.
func Flatten[T any](src [][]T) []T {
	n := 0
	for _, s := range src {
		n += len(s)
	}
	out := make([]T, 0, n)
	for _, s := range src {
		out = append(out, s...)
	}
	return out
}

// Chunk splits src into consecutive sub-slices of at most size elements.
// The returned sub-slices are views into src's backing array; mutating them
// will mutate the original slice. Copy the sub-slices if independent data
// is required.
func Chunk[T any](src []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	out := make([][]T, 0, (len(src)+size-1)/size)
	for len(src) > 0 {
		n := size
		if len(src) < n {
			n = len(src)
		}
		out = append(out, src[:n])
		src = src[n:]
	}
	return out
}

// Zip pairs elements from two slices. The returned slice has length equal to
// the shorter of a and b.
func Zip[A, B any](a []A, b []B) []Pair[A, B] {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]Pair[A, B], n)
	for i := range n {
		out[i] = Pair[A, B]{First: a[i], Second: b[i]}
	}
	return out
}

// Pair holds two values of possibly different types.
type Pair[A, B any] struct {
	First  A
	Second B
}
