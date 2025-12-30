package why

import (
	"context"
)

const (
	// maxAncestryDepth limits the depth of ancestry traversal to prevent infinite loops
	maxAncestryDepth = 50
)

// buildAncestry constructs the process ancestry chain from init to the target PID.
// Returns the chain in order from root (init/systemd) to target.
func buildAncestry(ctx context.Context, pid int) ([]ProcessInfo, error) {
	var chain []ProcessInfo
	seen := make(map[int]bool)
	current := pid

	for i := 0; i < maxAncestryDepth && current > 0; i++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			// Return partial chain on timeout
			return reverseChain(chain), ctx.Err()
		default:
		}

		// Prevent infinite loops from circular PPID references
		if seen[current] {
			break
		}
		seen[current] = true

		// Read process information
		info, err := readProcessInfo(ctx, current)
		if err != nil {
			// Stop traversal on error but return what we have
			break
		}

		chain = append(chain, info)

		// Stop at init/systemd (PID 1) or if PPID is 0
		if current == 1 || info.PPID == 0 {
			break
		}

		current = info.PPID
	}

	// Reverse to get root-to-target order
	return reverseChain(chain), nil
}

// reverseChain reverses the process chain in place.
func reverseChain(chain []ProcessInfo) []ProcessInfo {
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// formatAncestryChain formats the ancestry as a readable string.
// Example: "systemd (pid 1) → pm2 (pid 5034) → node (pid 14233)"
func FormatAncestryChain(ancestry []ProcessInfo) string {
	if len(ancestry) == 0 {
		return "(unknown)"
	}

	result := ""
	for i, p := range ancestry {
		if i > 0 {
			result += " → "
		}
		result += formatProcess(p)
	}
	return result
}

// formatProcess formats a single process for display.
func formatProcess(p ProcessInfo) string {
	return p.Command + " (pid " + itoa(p.PID) + ")"
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if negative {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}
