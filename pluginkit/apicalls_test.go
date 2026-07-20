package pluginkit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAPICallCounter_NilReportsZero(t *testing.T) {
	var counter *APICallCounter
	if got := counter.APICallCount(); got != 0 {
		t.Errorf("expected a nil counter to report 0, got %d", got)
	}
}

func TestAPICallCounter_ZeroValueIsUsable(t *testing.T) {
	counter := &APICallCounter{}
	if got := counter.APICallCount(); got != 0 {
		t.Errorf("expected a fresh counter to report 0, got %d", got)
	}
}

func TestAPICallCounter_CountsRoundTrips(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	counter := &APICallCounter{}
	client := counter.WrapClient(&http.Client{})

	for range 3 {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp.Body.Close()
	}

	if got := counter.APICallCount(); got != 3 {
		t.Errorf("expected 3 counted round trips, got %d", got)
	}
}

// TestAPICallCounter_PayloadPromotesMethod is the documented usage: the payload
// embeds the counter and satisfies APICallReporter without declaring a method.
func TestAPICallCounter_PayloadPromotesMethod(t *testing.T) {
	type payload struct {
		Repo string
		*APICallCounter
	}

	counter := &APICallCounter{}
	p := payload{Repo: "x", APICallCounter: counter}

	var reporter APICallReporter = p
	counter.n.Add(7)

	if got := reporter.APICallCount(); got != 7 {
		t.Errorf("expected the promoted method to report 7, got %d", got)
	}

	// copying the payload by value must still observe the same tally, which is
	// the reason the counter is embedded as a pointer
	clone := p
	counter.n.Add(1)
	if got := clone.APICallCount(); got != 8 {
		t.Errorf("expected a payload copy to share the tally, got %d", got)
	}
}

func TestAPICallCounter_WrapPreservesBaseAndDefaults(t *testing.T) {
	counter := &APICallCounter{}

	if got := counter.Wrap(nil); got == nil {
		t.Error("expected a nil base to be replaced with the default transport")
	}

	var nilCounter *APICallCounter
	base := http.DefaultTransport
	if got := nilCounter.Wrap(base); got == nil {
		t.Error("expected a nil counter to return the base transport undecorated")
	}

	// WrapClient on a nil client must still produce a usable client
	if client := counter.WrapClient(nil); client == nil || client.Transport == nil {
		t.Error("expected WrapClient(nil) to yield a usable client")
	}
}

func TestAPICallCounter_ConcurrentRoundTrips(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	counter := &APICallCounter{}
	client := counter.WrapClient(&http.Client{})

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(server.URL)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()

	if got := counter.APICallCount(); got != 50 {
		t.Errorf("expected 50 counted round trips, got %d", got)
	}
}
