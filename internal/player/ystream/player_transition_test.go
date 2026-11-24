package ystream

import "testing"

func TestStartIgnoresStaleOrRemovedQueueEntry(t *testing.T) {
	p := &Player{gen: 2}
	// A queued start can run after the queue has been replaced or cleared.
	p.start(1, 0)
	p.start(2, 0)
	p.start(2, -1)
}
