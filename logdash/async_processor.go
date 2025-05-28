package logdash

import (
	"context"
	"errors"
	"sync"
)

// asyncProcessor is a generic processor for handling asynchronous operations.
type asyncProcessor[T any] struct {
	processChan    chan T
	stoppedChan    chan struct{}
	processChanMu  sync.RWMutex
	overflowPolicy OverflowPolicy
	processFunc    func(T) error
	errorHandler   func(error)
}

// errChannelOverflow is returned when the channel is full and the overflow policy is set to drop.
var errChannelOverflow = errors.New("channel overflow")

// newAsyncProcessor creates a new async processor instance.
func newAsyncProcessor[T any](bufferSize int, processFunc func(T) error, errorHandler func(error)) *asyncProcessor[T] {
	processor := &asyncProcessor[T]{
		processChan:    make(chan T, bufferSize),
		stoppedChan:    make(chan struct{}),
		overflowPolicy: OverflowPolicyBlock, // Default to blocking
		processFunc:    processFunc,
		errorHandler:   errorHandler,
	}

	// Start background worker
	go processor.process(processor.processChan)

	return processor
}

// process handles the background processing of items
func (p *asyncProcessor[T]) process(ch chan T) {
	defer close(p.stoppedChan)
	for item := range ch {
		if err := p.processFunc(item); err != nil {
			p.errorHandler(err)
		}
	}
}

// send sends an item to be processed asynchronously
func (p *asyncProcessor[T]) send(item T) {
	p.processChanMu.RLock()
	defer p.processChanMu.RUnlock()
	select {
	case p.processChan <- item:
		// Item sent to channel
	default:
		// Channel is full
		if p.overflowPolicy == OverflowPolicyDrop {
			p.errorHandler(errChannelOverflow)
			return
		}
		// Block until there's space in the channel
		p.processChan <- item
	}
}

// Close stops the background worker immediately.
func (p *asyncProcessor[T]) Close() error {
	p.processChanMu.Lock()
	defer p.processChanMu.Unlock()

	return p.safeClear()
}

func (p *asyncProcessor[T]) safeClear() error {
	// already in shutdown mode or closed
	if p.processChan == nil {
		return ErrAlreadyClosed
	}

	close(p.processChan)
	p.processChan = nil
	return nil
}

// Shutdown stops the background worker after items in the channel are processed.
func (p *asyncProcessor[T]) Shutdown(ctx context.Context) error {
	p.processChanMu.Lock()
	if err := p.safeClear(); err != nil {
		return err
	}
	p.processChanMu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stoppedChan:
		return nil
	}
}

// SetOverflowPolicy sets the overflow policy for the processor
func (p *asyncProcessor[T]) SetOverflowPolicy(policy OverflowPolicy) {
	p.overflowPolicy = policy
}
