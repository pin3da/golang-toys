package ratelimiter_test

import (
	"testing"

	ratelimiter "github.com/pin3da/golang-toys/rate-limiter"
	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
	"github.com/pin3da/golang-toys/rate-limiter/leakybucket"
	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
	"github.com/pin3da/golang-toys/rate-limiter/tokenbucket"
)

// TestLimitersSatisfyInterface ensures every implementation is usable through
// the common Limiter interface. It does not exercise Allow; per-algorithm
// behavior tests live alongside each implementation.
func TestLimitersSatisfyInterface(t *testing.T) {
	limiters := []ratelimiter.Limiter{
		tokenbucket.New(10, 1),
		leakybucket.New(10, 1),
		fixedwindow.New(10, 0),
		slidinglog.New(10, 0),
		slidingwindow.New(10, 0),
	}
	for _, l := range limiters {
		if l.Name() == "" {
			t.Errorf("%T: Name() returned empty string", l)
		}
	}
}
