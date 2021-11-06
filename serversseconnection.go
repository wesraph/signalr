package signalr

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type serverSSEConnection struct {
	ConnectionBase
	mx            sync.Mutex
	postWriting   bool
	postWriter    io.Writer
	postReader    io.Reader
	jobChan       chan []byte
	jobResultChan chan sseJobResult
}

type sseJobResult struct {
	n   int
	err error
}

func newServerSSEConnection(ctx context.Context, connectionID string) (*serverSSEConnection, <-chan []byte, chan sseJobResult, error) {
	s := serverSSEConnection{
		ConnectionBase: ConnectionBase{
			ctx:          ctx,
			connectionID: connectionID,
		},
		jobChan:       make(chan []byte, 1),
		jobResultChan: make(chan sseJobResult, 1),
	}
	s.postReader, s.postWriter = io.Pipe()
	go func() {
		<-s.Context().Done()
		s.mx.Lock()
		close(s.jobChan)
		s.mx.Unlock()
	}()
	return &s, s.jobChan, s.jobResultChan, nil
}

func (s *serverSSEConnection) consumeRequest(request *http.Request) int {
	if err := s.Context().Err(); err != nil {
		return http.StatusGone // 410
	}
	s.mx.Lock()
	if s.postWriting {
		s.mx.Unlock()
		return http.StatusConflict // 409
	}
	s.postWriting = true
	s.mx.Unlock()
	defer func() {
		_ = request.Body.Close()
	}()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return http.StatusBadRequest // 400
	} else if _, err := s.postWriter.Write(body); err != nil {
		return http.StatusInternalServerError // 500
	}
	s.mx.Lock()
	s.postWriting = false
	s.mx.Unlock()
	<-time.After(50 * time.Millisecond)
	return http.StatusOK // 200
}

func (s *serverSSEConnection) Read(p []byte) (n int, err error) {
	if err := s.Context().Err(); err != nil {
		return 0, fmt.Errorf("serverSSEConnection canceled: %w", s.ctx.Err())
	}
	return s.postReader.Read(p)
}

func (s *serverSSEConnection) Write(p []byte) (n int, err error) {
	if err := s.Context().Err(); err != nil {
		return 0, fmt.Errorf("serverSSEConnection canceled: %w", s.ctx.Err())
	}
	payload := ""
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		payload = payload + "data: " + line + "\n"
	}
	// prevent race with goroutine closing the jobChan
	s.mx.Lock()
	if s.Context().Err() == nil {
		s.jobChan <- []byte(payload + "\n")
	} else {
		return 0, fmt.Errorf("serverSSEConnection canceled: %w", s.ctx.Err())
	}
	s.mx.Unlock()
	r := <-s.jobResultChan
	return r.n, r.err
}
