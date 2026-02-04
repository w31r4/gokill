package why

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

type envCaptureErrorKind string

const (
	envCaptureTimeout   envCaptureErrorKind = "timeout"
	envCaptureTruncated envCaptureErrorKind = "truncated"
)

type envCaptureError struct {
	kind   envCaptureErrorKind
	detail string
}

func (e envCaptureError) Error() string {
	if e.detail == "" {
		return string(e.kind)
	}
	return fmt.Sprintf("%s: %s", e.kind, e.detail)
}

func readEnvironFromReaderWithContext(ctx context.Context, r io.Reader, maxBytes, maxVars int) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return nil, fmt.Errorf("nil reader")
	}

	if maxBytes <= 0 {
		maxBytes = defaultEnvMaxBytes
	}
	if maxVars <= 0 {
		maxVars = defaultEnvMaxVars
	}

	buf := make([]byte, 0, minInt(maxBytes, 4096))
	tmp := make([]byte, 4096)

	bytesTruncated := false
	for {
		if err := ctx.Err(); err != nil {
			env := parseEnvironBytes(trimToLastNull(buf))
			env, varsTruncated := limitEnvVars(env, maxVars)
			errOut := err
			if errors.Is(err, context.DeadlineExceeded) {
				errOut = envCaptureError{kind: envCaptureTimeout}
			}
			if varsTruncated || bytesTruncated {
				// Preserve the timeout/cancel signal; truncation details are secondary.
			}
			return env, errOut
		}

		if len(buf) >= maxBytes {
			bytesTruncated = true
			break
		}

		remain := maxBytes - len(buf)
		want := minInt(len(tmp), remain)
		n, err := r.Read(tmp[:want])
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			env := parseEnvironBytes(trimToLastNull(buf))
			env, _ = limitEnvVars(env, maxVars)
			if len(env) == 0 {
				return nil, err
			}
			return env, err
		}
	}

	env := parseEnvironBytes(trimToLastNull(buf))
	env, varsTruncated := limitEnvVars(env, maxVars)

	if bytesTruncated {
		return env, envCaptureError{kind: envCaptureTruncated, detail: fmt.Sprintf("maxBytes=%d", maxBytes)}
	}
	if varsTruncated {
		return env, envCaptureError{kind: envCaptureTruncated, detail: fmt.Sprintf("maxVars=%d", maxVars)}
	}

	return env, nil
}

func trimToLastNull(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	idx := bytes.LastIndexByte(data, 0)
	if idx == -1 {
		return nil
	}
	return data[:idx]
}

func limitEnvVars(env []string, maxVars int) ([]string, bool) {
	if maxVars <= 0 || len(env) <= maxVars {
		return env, false
	}
	return env[:maxVars], true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
