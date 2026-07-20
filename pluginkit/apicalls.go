package pluginkit

import (
	"net/http"
	"sync/atomic"
)

// APICallCounter tallies outbound HTTP requests, letting a plugin's payload
// satisfy APICallReporter without hand-rolling a RoundTripper. Embed a
// *APICallCounter in the payload and APICallCount is promoted onto it:
//
//	type Payload struct {
//	    // ...
//	    *pluginkit.APICallCounter
//	}
//
//	counter := &pluginkit.APICallCounter{}
//	httpClient := counter.WrapClient(oauth2.NewClient(ctx, src))
//	return Payload{APICallCounter: counter /* ... */}
//
// Embed the pointer, not the value: payloads are commonly used by value, and a
// value-embedded counter would be copied so the tallies diverge. Copying is a
// `go vet` copylocks error rather than a silent miscount, because the tally is
// an atomic.Int64.
//
// The tally counts HTTP round trips, which is a close proxy for API calls but
// not identical: retries and redirects each increment it, while one request
// batching several logical queries (a GraphQL document, say) counts once. It is
// a rate-limit budget, not an exact call ledger. All hosts share one tally; a
// plugin calling two rate-limited services sees their sum.
//
// The zero value is ready to use, and a nil *APICallCounter reports zero.
type APICallCounter struct {
	n atomic.Int64
}

// APICallCount implements APICallReporter. It is safe on a nil receiver so a
// payload built without a counter reports zero rather than panicking.
func (c *APICallCounter) APICallCount() int {
	if c == nil {
		return 0
	}
	return int(c.n.Load())
}

// Wrap returns base decorated to count every round trip through it. A nil base
// means http.DefaultTransport, matching http.Client's own behavior. A nil
// counter returns base undecorated.
func (c *APICallCounter) Wrap(base http.RoundTripper) http.RoundTripper {
	if c == nil {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &countingTransport{base: base, counter: c}
}

// WrapClient decorates client's transport in place and returns client, so it
// composes with a client an auth library already built. A nil client yields a
// new one.
func (c *APICallCounter) WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	client.Transport = c.Wrap(client.Transport)
	return client
}

// countingTransport increments its counter once per round trip.
type countingTransport struct {
	base    http.RoundTripper
	counter *APICallCounter
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.counter.n.Add(1)
	return t.base.RoundTrip(req)
}
