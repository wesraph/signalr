package signalr

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type invokeClient struct {
	mx                 sync.Mutex
	resultChans        map[string]invokeResult
	chanReceiveTimeout time.Duration
}

func newInvokeClient(chanReceiveTimeout time.Duration) *invokeClient {
	return &invokeClient{
		mx:                 sync.Mutex{},
		resultChans:        make(map[string]invokeResult),
		chanReceiveTimeout: chanReceiveTimeout,
	}
}

type invokeResult struct {
	resultChan chan interface{}
	errChan    chan error
}

func (i *invokeClient) newInvocation(id string) (chan interface{}, chan error) {
	i.mx.Lock()
	r := invokeResult{
		resultChan: make(chan interface{}, 1),
		errChan:    make(chan error, 1),
	}
	i.resultChans[id] = r
	i.mx.Unlock()
	return r.resultChan, r.errChan
}

func (i *invokeClient) deleteInvocation(id string) {
	i.mx.Lock()
	if r, ok := i.resultChans[id]; ok {
		delete(i.resultChans, id)
		close(r.resultChan)
		close(r.errChan)
	}
	i.mx.Unlock()
}

func (i *invokeClient) cancelAllInvokes() {
	i.mx.Lock()
	for _, r := range i.resultChans {
		close(r.resultChan)
		go func(errChan chan error) {
			errChan <- errors.New("message loop ended")
			close(errChan)
		}(r.errChan)
	}
	// Clear map
	i.resultChans = make(map[string]invokeResult)
	i.mx.Unlock()
}

func (i *invokeClient) handlesInvocationID(invocationID string) bool {
	i.mx.Lock()
	_, ok := i.resultChans[invocationID]
	i.mx.Unlock()
	return ok
}

func (i *invokeClient) receiveCompletionItem(completion completionMessage) error {
	i.mx.Lock()
	ir, ok := i.resultChans[completion.InvocationID]
	i.mx.Unlock()
	if ok {
		if completion.Error != "" {
			done := make(chan struct{})
			go func() {
				ir.errChan <- errors.New(completion.Error)
				done <- struct{}{}
			}()
			select {
			case <-done:
				return nil
			case <-time.After(i.chanReceiveTimeout):
				return &hubChanTimeoutError{fmt.Sprintf("timeout (%v) waiting for hub to receive client sent error", i.chanReceiveTimeout)}
			}
		}
		if completion.Result != nil {
			done := make(chan struct{})
			go func() {
				ir.resultChan <- completion.Result
				done <- struct{}{}
			}()
			select {
			case <-done:
				return nil
			case <-time.After(i.chanReceiveTimeout):
				return &hubChanTimeoutError{fmt.Sprintf("timeout (%v) waiting for hub to receive client sent value", i.chanReceiveTimeout)}
			}
		}
		return nil
	}
	return fmt.Errorf(`unknown completion id "%v"`, completion.InvocationID)
}
