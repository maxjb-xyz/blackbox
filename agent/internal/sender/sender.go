package sender

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/shared/types"
)

const (
	mainBufSize  = 2048
	retryBufSize = 64
	maxRetries   = 10
	maxBackoff   = 30 * time.Second
)

type Sender struct {
	client  *client.Client
	events  chan types.Entry
	retries chan types.Entry
	done    chan struct{}
}

func New(c *client.Client) *Sender {
	return &Sender{
		client:  c,
		events:  make(chan types.Entry, mainBufSize),
		retries: make(chan types.Entry, retryBufSize),
		done:    make(chan struct{}),
	}
}

func (s *Sender) Chan() chan<- types.Entry {
	return s.events
}

func (s *Sender) Done() <-chan struct{} {
	return s.done
}

func (s *Sender) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.retryWorker(ctx)
	}()
	go func() {
		defer wg.Done()
		s.sendLoop(ctx)
	}()
	wg.Wait()
	close(s.done)
}

func (s *Sender) sendLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case entry := <-s.events:
					if err := s.client.Send(entry); err != nil {
						log.Printf("sender: drop on shutdown: %v", err)
					}
				case entry := <-s.retries:
					if err := s.client.Send(entry); err != nil {
						log.Printf("sender: drop on shutdown: %v", err)
					}
				default:
					return
				}
			}
		case entry := <-s.events:
			if err := s.client.Send(entry); err != nil {
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

func (s *Sender) retryWorker(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-s.retries:
			attempts := 0
			for {
				select {
				case <-ctx.Done():
					if err := s.client.Send(entry); err != nil {
						log.Printf("sender: drop on shutdown: %v", err)
					}
					return
				case <-time.After(backoff):
				}
				attempts++
				if err := s.client.Send(entry); err != nil {
					var permErr *client.PermanentError
					if errors.As(err, &permErr) {
						log.Printf("sender: permanent error, dropping entry id=%s: %v", entry.ID, err)
						backoff = time.Second
						break
					}
					if attempts >= maxRetries {
						log.Printf("sender: max retries (%d) exceeded, dropping entry id=%s: %v", maxRetries, entry.ID, err)
						backoff = time.Second
						break
					}
					log.Printf("sender: retry %d/%d failed (backoff %s): %v", attempts, maxRetries, backoff, err)
					if backoff < maxBackoff {
						backoff *= 2
						if backoff > maxBackoff {
							backoff = maxBackoff
						}
					}
				} else {
					log.Printf("sender: retry succeeded after %d attempt(s), resetting backoff", attempts)
					backoff = time.Second
					break
				}
			}
		}
	}
}
