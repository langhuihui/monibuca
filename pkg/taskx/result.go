// Package taskx provides generic task execution utilities and a Result type
// that wraps a value/error pair, reducing boilerplate in async task chains.
package taskx

import "context"

// Result bundles a typed value with an optional error so that task functions
// can return both in a single, type-safe structure.
type Result[T any] struct {
	Value T
	Err   error
}

// Ok returns a successful Result holding v.
func Ok[T any](v T) Result[T] {
	return Result[T]{Value: v}
}

// Fail returns a failed Result holding err.
func Fail[T any](err error) Result[T] {
	return Result[T]{Err: err}
}

// Unwrap returns the value and error stored in r.
func (r Result[T]) Unwrap() (T, error) {
	return r.Value, r.Err
}

// IsOk reports whether the result carries no error.
func (r Result[T]) IsOk() bool {
	return r.Err == nil
}

// Runner is the constraint for types that can be executed to produce a T.
type Runner[T any] interface {
	Run(context.Context) (T, error)
}

// Execute runs r and wraps the outcome in a Result.
func Execute[T any](ctx context.Context, r Runner[T]) Result[T] {
	v, err := r.Run(ctx)
	if err != nil {
		return Fail[T](err)
	}
	return Ok(v)
}

// RunFunc is an adapter that lets a plain function satisfy Runner[T].
type RunFunc[T any] func(context.Context) (T, error)

func (f RunFunc[T]) Run(ctx context.Context) (T, error) { return f(ctx) }

// ExecuteFunc is a shorthand for executing a function directly without
// wrapping it in a named type.
func ExecuteFunc[T any](ctx context.Context, fn func(context.Context) (T, error)) Result[T] {
	return Execute[T](ctx, RunFunc[T](fn))
}
