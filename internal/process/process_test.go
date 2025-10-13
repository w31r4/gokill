package process

import "testing"

func BenchmarkGetProcesses(b *testing.B) {
	// b.N is a variable that the testing framework manages.
	// It will increase the number of iterations until the benchmark
	// result is statistically stable.
	for i := 0; i < b.N; i++ {
		_, err := GetProcesses()
		if err != nil {
			b.Fatal(err)
		}
	}
}
