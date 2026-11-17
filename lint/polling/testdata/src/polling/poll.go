package polling

import (
	"context"
	"time"
)

// ready reports whether the awaited state holds.
func ready() bool { return false }

// SleepPoll waits for ready by sleeping between reads.
func SleepPoll() {
	for !ready() {
		time.Sleep(time.Millisecond) // want `time.Sleep polls for a state change; watch the component`
	}
}

// TickerPoll waits for ready on a ticker.
func TickerPoll(ctx context.Context) {
	ticker := time.NewTicker(time.Second) // want `time.NewTicker polls`
	defer ticker.Stop()
	for _, tick := range []<-chan time.Time{time.Tick(time.Second)} { // want `time.Tick polls`
		<-tick
	}
	<-ctx.Done()
}

// AfterPoll waits for ready with a fresh timer each iteration.
func AfterPoll(ctx context.Context) {
	for !ready() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second): // want `time.After inside a loop polls`
		}
	}
}

// Deadline waits once for a known expiry, which is not a poll.
func Deadline(ctx context.Context, done <-chan struct{}) {
	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(time.Minute):
	}
}

// Backoff retries a failed attempt with a marked delay.
func Backoff(ctx context.Context, attempt func() error) {
	delay := time.Millisecond
	for attempt() != nil {
		select {
		case <-ctx.Done():
			return
		//gostyle:timer backoff
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// External bounds a poll of a system that offers no watch.
func External() {
	for !ready() {
		time.Sleep(time.Millisecond) //gostyle:timer external
	}
}

// LoopedGoroutine starts work whose own body holds no loop.
func LoopedGoroutine(ctx context.Context, n int) {
	for range n {
		go func() {
			<-time.After(time.Second)
		}()
	}
}

// Unknown names a kind the directive does not allow.
func Unknown() {
	//gostyle:timer settle
	time.Sleep(time.Second) // want `timer kind "settle" is not deadline, backoff or external`
}
