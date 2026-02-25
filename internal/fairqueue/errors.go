package fairqueue

import "errors"

var (
	ErrQueueClosed      = errors.New("queue is closed")
	ErrQueueFull        = errors.New("tenant queue depth limit exceeded")
	ErrQueueEmpty       = errors.New("queue is empty")
	ErrInflightLimit    = errors.New("tenant inflight limit exceeded")
	ErrDeadlineExceeded = errors.New("deadline already exceeded")
)
