package sender

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/agent/internal/queue"
	"blackbox/shared/types"
)

const (
	// eventBufSize is the in-memory transit buffer between collectors and queueWriter.
	// It is intentionally small (256 vs the old 2048) because SQLite is the primary
	// buffer — this channel only needs to absorb bursts between queueWriter ticks.
	eventBufSize = 256
	flushBatch   = 50
	maxBackoff   = 30 * time.Second
)

// Sender persists events to a local queue and flushes them to the server in
// batches. It survives agent restarts and network outages without data loss.
type Sender struct {
	client        *client.Client
	queue         *queue.Queue
	events        chan types.Entry
	done          chan struct{}
	flushInterval time.Duration
}

// New creates a Sender with a 1-second flush interval.
func New(c *client.Client, q *queue.Queue) *Sender {
	return newWithFlushInterval(c, q, time.Second)
}

func newWithFlushInterval(c *client.Client, q *queue.Queue, interval time.Duration) *Sender {
	return &Sender{
		client:        c,
		queue:         q,
		events:        make(chan types.Entry, eventBufSize),
		done:          make(chan struct{}),
		flushInterval: interval,
	}
}

// Chan returns the channel collectors write events to.
func (s *Sender) Chan() chan<- types.Entry {
	return s.events
}

// Done is closed when the sender has fully shut down.
func (s *Sender) Done() <-chan struct{} {
	return s.done
}

// Start runs the sender until ctx is cancelled, then performs a final flush.
func (s *Sender) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.queueWriter(ctx)
	}()
	go func() {
		defer wg.Done()
		s.flushLoop(ctx)
	}()
	wg.Wait()
	close(s.done)
}

// queueWriter drains the inbound channel and persists each event to SQLite.
// If Push fails (e.g. disk full) the entry is logged and dropped — this is
// the only intentional drop point in the system.
func (s *Sender) queueWriter(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case entry := <-s.events:
					if err := s.queue.Push(entry); err != nil {
						log.Printf("sender: queue push failed on shutdown, dropping id=%s: %v", entry.ID, err)
					}
				default:
					return
				}
			}
		case entry := <-s.events:
			if err := s.queue.Push(entry); err != nil {
				log.Printf("sender: queue push failed, dropping id=%s: %v", entry.ID, err)
			}
		}
	}
}

// flushLoop ticks at flushInterval, reads up to flushBatch rows, sends them,
// and deletes accepted rows. On failure it backs off up to maxBackoff.
func (s *Sender) flushLoop(ctx context.Context) {
	backoff := s.flushInterval

	doFlush := func(ctx context.Context) {
		entries, err := s.queue.Flush(flushBatch)
		if err != nil {
			log.Printf("sender: flush read failed: %v", err)
			return
		}
		if len(entries) == 0 {
			return
		}

		flushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		accepted, failed, err := s.client.SendBatch(flushCtx, entries)
		if err != nil {
			log.Printf("sender: batch send failed (backoff %s): %v", backoff, err)
			// Permanent errors (4xx) mean the whole batch is rejected; delete to
			// avoid retrying entries the server will always refuse.
			var permErr *client.PermanentError
			if errors.As(err, &permErr) {
				ids := make([]string, len(entries))
				for i, e := range entries {
					ids[i] = e.ID
				}
				if delErr := s.queue.Delete(ids); delErr != nil {
					log.Printf("sender: failed to delete permanently rejected batch: %v", delErr)
				}
				backoff = s.flushInterval
			} else if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			return
		}
		for _, f := range failed {
			log.Printf("sender: server rejected entry id=%s: %s", f.ID, f.Reason)
		}
		// Delete accepted entries and any per-entry permanent failures.
		toDelete := accepted
		for _, f := range failed {
			toDelete = append(toDelete, f.ID)
		}
		if len(toDelete) > 0 {
			if err := s.queue.Delete(toDelete); err != nil {
				log.Printf("sender: failed to delete sent entries: %v", err)
			}
		}
		backoff = s.flushInterval
	}

	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for {
				entries, err := s.queue.Flush(flushBatch)
				if err != nil || len(entries) == 0 {
					break
				}
				doFlush(drainCtx)
			}
			return
		case <-timer.C:
			doFlush(ctx)
			// timer.C was drained by the select receive, so Reset is safe without draining first.
			timer.Reset(backoff)
		}
	}
}
