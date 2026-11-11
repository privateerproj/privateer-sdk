package shared

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
		{"Aborted beats TestPass", TestPass, Aborted, Aborted},
		{"Aborted beats TestFail", TestFail, Aborted, Aborted},
		{"BadUsage beats Aborted", Aborted, BadUsage, BadUsage},
		{"NoTests beats TestPass", TestPass, NoTests, NoTests},
		{"TestFail beats NoTests", NoTests, TestFail, TestFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeExitCode(tt.prev, tt.next); got != tt.want {
				t.Errorf("MergeExitCode(%d, %d) = %d, want %d", tt.prev, tt.next, got, tt.want)
			}
		})
	}
}

// TestExitSeverityCoversEveryExitCode fails when a code is added without its
// own exitSeverity entry, which would otherwise merge as TestPass. Severities
// must also be unique, since MergeExitCode keeps the earlier code on a tie.
func TestExitSeverityCoversEveryExitCode(t *testing.T) {
	if len(exitSeverity) != exitCodeCount {
		t.Errorf("exitSeverity has %d entries for %d exit codes", len(exitSeverity), exitCodeCount)
	}
	codeBySeverity := map[int]int{}
	for code := range exitCodeCount {
		severity, ok := exitSeverity[code]
		if !ok {
			t.Errorf("exit code %d has no exitSeverity entry", code)
			continue
		}
		if otherCode, taken := codeBySeverity[severity]; taken {
			t.Errorf("exit codes %d and %d share severity %d", otherCode, code, severity)
		}
		codeBySeverity[severity] = code
	}
}
