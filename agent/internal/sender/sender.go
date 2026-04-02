package sender

import (
	"context"
	"errors"
	"log"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/shared/types"
)

const (
	mainBufSize      = 2048
	retryBufSize     = 64
	maxBackoff       = 30 * time.Second
	maxRetryAttempts = 8
)

type Sender struct {
	client    *client.Client
	events    chan types.Entry
	retries   chan types.Entry
	done      chan struct{}
	retryDone chan struct{}
}

func New(c *client.Client) *Sender {
	return &Sender{
		client:    c,
		events:    make(chan types.Entry, mainBufSize),
		retries:   make(chan types.Entry, retryBufSize),
		done:      make(chan struct{}),
		retryDone: make(chan struct{}),
	}
}

func (s *Sender) Chan() chan<- types.Entry {
	return s.events
}

func (s *Sender) Done() <-chan struct{} {
	return s.done
}

func (s *Sender) Start(ctx context.Context) {
	go s.retryWorker(ctx)
	s.sendLoop(ctx)
}

func (s *Sender) sendLoop(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			s.drainEvents()
			<-s.retryDone
			return
		case entry := <-s.events:
			if err := s.client.Send(entry); err != nil {
				var permErr *client.PermanentError
				if errors.As(err, &permErr) {
					log.Printf("sender: permanent error, dropping entry id=%s: %v", entry.ID, err)
				} else {
					log.Printf("sender: delivery failed, queuing retry: %v", err)
					select {
					case s.retries <- entry:
					default:
						log.Printf("sender: retry buffer full, dropping entry id=%s", entry.ID)
					}
				}
			}
		}
	}
}

func (s *Sender) drainEvents() {
	for {
		select {
		case entry := <-s.events:
			if err := s.client.Send(entry); err != nil {
				log.Printf("sender: drop on shutdown: %v", err)
			}
		default:
			return
		}
	}
}

func (s *Sender) retryWorker(ctx context.Context) {
	defer close(s.retryDone)
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			s.drainRetriesOnShutdown()
			return
		case entry := <-s.retries:
			backoff = s.retryWithBackoff(ctx, entry, backoff)
		}
	}
}

func (s *Sender) retryWithBackoff(ctx context.Context, entry types.Entry, backoff time.Duration) time.Duration {
	attempts := 0
	for {
		if attempts >= maxRetryAttempts {
			log.Printf("sender: max retry attempts reached, dropping entry id=%s", entry.ID)
			return time.Second
		}

		select {
		case <-ctx.Done():
			s.drainSingleRetry(entry)
			s.drainRetriesOnShutdown()
			return time.Second
		case <-time.After(backoff):
		}

		attempts++
		if err := s.client.Send(entry); err != nil {
			var permErr *client.PermanentError
			if errors.As(err, &permErr) {
				log.Printf("sender: permanent error on retry, dropping entry id=%s: %v", entry.ID, err)
				return time.Second
			}

			log.Printf("sender: retry failed (attempt %d/%d, backoff %s): %v", attempts, maxRetryAttempts, backoff, err)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		log.Printf("sender: retry succeeded, resetting backoff")
		return time.Second
	}
}

func (s *Sender) drainRetriesOnShutdown() {
	for {
		select {
		case entry := <-s.retries:
			s.drainSingleRetry(entry)
		default:
			return
		}
	}
}

func (s *Sender) drainSingleRetry(entry types.Entry) {
	if err := s.client.Send(entry); err != nil {
		log.Printf("sender: drop retry on shutdown id=%s: %v", entry.ID, err)
	}
}
