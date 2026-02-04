package why

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestReadEnvironFromReaderWithContext_OK(t *testing.T) {
	data := []byte("A=1\x00B=2\x00")
	env, err := readEnvironFromReaderWithContext(context.Background(), bytes.NewReader(data), 1024, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(env) != 2 || env[0] != "A=1" || env[1] != "B=2" {
		t.Fatalf("unexpected env: %#v", env)
	}
}

func TestReadEnvironFromReaderWithContext_TruncatesBytesDropsIncompleteTail(t *testing.T) {
	data := []byte("A=1\x00B=2\x00C=3\x00")
	env, err := readEnvironFromReaderWithContext(context.Background(), bytes.NewReader(data), 9, 10)
	if err == nil {
		t.Fatalf("expected truncation error, got nil")
	}
	var ce envCaptureError
	if !errors.As(err, &ce) || ce.kind != envCaptureTruncated {
		t.Fatalf("expected envCaptureTruncated, got %T: %v", err, err)
	}
	if len(env) != 2 {
		t.Fatalf("expected 2 vars after truncation, got %#v", env)
	}
	for _, e := range env {
		if e == "C=3" || e == "C" {
			t.Fatalf("expected incomplete tail to be dropped, got env=%#v", env)
		}
	}
}

func TestReadEnvironFromReaderWithContext_TruncatesVars(t *testing.T) {
	data := []byte("A=1\x00B=2\x00C=3\x00")
	env, err := readEnvironFromReaderWithContext(context.Background(), bytes.NewReader(data), 1024, 2)
	if err == nil {
		t.Fatalf("expected truncation error, got nil")
	}
	var ce envCaptureError
	if !errors.As(err, &ce) || ce.kind != envCaptureTruncated {
		t.Fatalf("expected envCaptureTruncated, got %T: %v", err, err)
	}
	if len(env) != 2 || env[0] != "A=1" || env[1] != "B=2" {
		t.Fatalf("unexpected env after var truncation: %#v", env)
	}
}

func TestReadEnvironFromReaderWithContext_Timeout(t *testing.T) {
	data := []byte("A=1\x00B=2\x00")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	env, err := readEnvironFromReaderWithContext(ctx, bytes.NewReader(data), 1024, 10)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	var ce envCaptureError
	if !errors.As(err, &ce) || ce.kind != envCaptureTimeout {
		t.Fatalf("expected envCaptureTimeout, got %T: %v", err, err)
	}
	if len(env) != 0 {
		t.Fatalf("expected no env entries on immediate timeout, got %#v", env)
	}
}
