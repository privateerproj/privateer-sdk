package command

import "testing"

func TestMergeExitCode(t *testing.T) {
	tests := []struct {
		name       string
		prev, next int
		want       int
	}{
		{"TestPass over TestPass keeps TestPass", TestPass, TestPass, TestPass},
		{"TestFail beats TestPass", TestPass, TestFail, TestFail},
		{"TestPass after TestFail keeps TestFail", TestFail, TestPass, TestFail},
		{"BadUsage beats TestFail", TestFail, BadUsage, BadUsage},
		{"TestFail does not downgrade BadUsage", BadUsage, TestFail, BadUsage},
		{"InternalError beats BadUsage", BadUsage, InternalError, InternalError},
		{"BadUsage does not downgrade InternalError", InternalError, BadUsage, InternalError},
		{"InternalError beats TestFail", TestFail, InternalError, InternalError},
		{"InternalError beats TestPass", TestPass, InternalError, InternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeExitCode(tt.prev, tt.next); got != tt.want {
				t.Errorf("mergeExitCode(%d, %d) = %d, want %d", tt.prev, tt.next, got, tt.want)
			}
		})
	}
}
