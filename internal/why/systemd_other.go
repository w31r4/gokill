//go:build !linux

package why

import "context"

func resolveSystemdUnit(ctx context.Context, pid int) string {
	return ""
}

