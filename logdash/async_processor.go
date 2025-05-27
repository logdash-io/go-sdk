package logdash

import "errors"

// asyncProcessor is a generic processor for handling asynchronous operations.
type asyncProcessor[T any] struct {
	processChan    chan T
	overflowPolicy OverflowPolicy
	stopChan       chan struct{}
	processFunc    func(T) error
	errorHandler   func(error)
}

// errChannelOverflow is returned when the channel is full and the overflow policy is set to drop.
var errChannelOverflow = errors.New("channel overflow")

// newAsyncProcessor creates a new async processor instance.
func newAsyncProcessor[T any](bufferSize int, processFunc func(T) error, errorHandler func(error)) *asyncProcessor[T] {
	processor := &asyncProcessor[T]{
		processChan:    make(chan T, bufferSize),
		overflowPolicy: OverflowPolicyBlock, // Default to blocking
		stopChan:       make(chan struct{}),
		processFunc:    processFunc,
		errorHandler:   errorHandler,
	}

	// Start background worker
	go processor.process()

	return processor
}

// process handles the background processing of items
func (p *asyncProcessor[T]) process() {
	for {
		select {
		case item := <-p.processChan:
			if err := p.processFunc(item); err != nil {
				p.errorHandler(err)
			}
		case <-p.stopChan:
			return
		}
	}
}

// send sends an item to be processed asynchronously
func (p *asyncProcessor[T]) send(item T) {
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

// Close stops the background worker and closes the processor
func (p *asyncProcessor[T]) Close() {
	close(p.stopChan)
	close(p.processChan)
}

// SetOverflowPolicy sets the overflow policy for the processor
func (p *asyncProcessor[T]) SetOverflowPolicy(policy OverflowPolicy) {
	p.overflowPolicy = policy
}
