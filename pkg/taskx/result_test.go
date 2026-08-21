package taskx_test

import (
	"context"
	"errors"
	"testing"

	"m7s.live/v5/pkg/taskx"
)

func TestOk(t *testing.T) {
	r := taskx.Ok(42)
	if !r.IsOk() {
		t.Fatal("Ok result should have no error")
	}
	v, err := r.Unwrap()
	if err != nil || v != 42 {
		t.Fatalf("Unwrap() = %d %v, want 42 nil", v, err)
	}
}

func TestFail(t *testing.T) {
	sentinel := errors.New("boom")
	r := taskx.Fail[int](sentinel)
	if r.IsOk() {
		t.Fatal("Fail result should carry an error")
	}
	_, err := r.Unwrap()
	if !errors.Is(err, sentinel) {
		t.Fatalf("Unwrap err = %v, want sentinel", err)
	}
}

type doubler struct{}

func (doubler) Run(_ context.Context) (int, error) { return 2 * 21, nil }

func TestExecute(t *testing.T) {
	r := taskx.Execute[int](context.Background(), doubler{})
	if !r.IsOk() || r.Value != 42 {
		t.Fatalf("Execute = %+v, want {42 nil}", r)
	}
}

func TestExecuteFunc(t *testing.T) {
	r := taskx.ExecuteFunc(context.Background(), func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if !r.IsOk() || r.Value != "hello" {
		t.Fatalf("ExecuteFunc = %+v", r)
	}
}

func TestExecuteFunc_Error(t *testing.T) {
	sentinel := errors.New("fail")
	r := taskx.ExecuteFunc(context.Background(), func(ctx context.Context) (int, error) {
		return 0, sentinel
	})
	if r.IsOk() {
		t.Fatal("expected error result")
	}
	if !errors.Is(r.Err, sentinel) {
		t.Fatalf("unexpected error: %v", r.Err)
	}
}

func TestRunFunc(t *testing.T) {
	fn := taskx.RunFunc[bool](func(_ context.Context) (bool, error) { return true, nil })
	r := taskx.Execute[bool](context.Background(), fn)
	if !r.IsOk() || !r.Value {
		t.Fatalf("RunFunc result = %+v", r)
	}
}
