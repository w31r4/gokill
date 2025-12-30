package why

import "fmt"

const restartWarningThreshold = 5

func restartCountFromAncestry(ancestry []ProcessInfo) int {
	if len(ancestry) == 0 {
		return 0
	}
	restartCount := 0
	lastCmd := ""
	for _, proc := range ancestry {
		if proc.Command == lastCmd {
			restartCount++
		}
		lastCmd = proc.Command
	}
	return restartCount
}

func shouldWarnRestart(count int) bool {
	return count > restartWarningThreshold
}

func restartWarning(count int) string {
	return fmt.Sprintf("Process or ancestor restarted more than %d times (count=%d)", restartWarningThreshold, count)
}
