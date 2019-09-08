/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// custom CLI loading spinner for kind
var spinnerFrames = []string{
	"⠈⠁",
	"⠈⠑",
	"⠈⠱",
	"⠈⡱",
	"⢀⡱",
	"⢄⡱",
	"⢄⡱",
	"⢆⡱",
	"⢎⡱",
	"⢎⡰",
	"⢎⡠",
	"⢎⡀",
	"⢎⠁",
	"⠎⠁",
	"⠊⠁",
}

// Spinner is a simple and efficient CLI loading spinner used by kind
// It is simplistic and assumes that the line length will not change.
type Spinner struct {
	stop    chan struct{} // signals writer goroutine to stop from Stop()
	stopped chan struct{} // signals Stop() that the writer goroutine stopped
	mu      *sync.Mutex   // protects the mutable bits
	// below are protected by mu
	running bool
	writer  io.Writer
	ticker  *time.Ticker // signals that it is time to write a frame
	prefix  string
	suffix  string
}

// spinner implements writer
var _ io.Writer = &Spinner{}

// NewSpinner initializes and returns a new Spinner that will write to w
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{
		stop:    make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
		mu:      &sync.Mutex{},
		writer:  w,
	}
}

// SetPrefix sets the prefix to print before the spinner
func (s *Spinner) SetPrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefix = prefix
}

// SetSuffix sets the suffix to print after the spinner
func (s *Spinner) SetSuffix(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suffix = suffix
}

// Start starts the spinner running
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// don't start if we've already started, also flag that we've started
	if s.running {
		return
	}
	s.running = true
	// start / create a frame ticker
	s.ticker = time.NewTicker(time.Millisecond * 100)
	// spin in the background
	go func() {
		// write frames forever (until signaled to stop)
		for {
			for _, frame := range spinnerFrames {
				select {
				// prefer stopping, select this signal first
				case <-s.stop:
					s.stopped <- struct{}{} // signal that we stopped
					return                  // ... and stop
				// otherwise continue and write one frame
				case <-s.ticker.C:
					func() {
						s.mu.Lock()
						defer s.mu.Unlock()
						fmt.Fprintf(s.writer, "\r%s%s%s", s.prefix, frame, s.suffix)
					}()
				}
			}
		}
	}()
}

// Stop signals the spinner to stop
func (s *Spinner) Stop() {
	s.mu.Lock()
	// if there's nothing to stop, return early
	if !s.running {
		s.mu.Unlock()
		return
	}
	// otherwise: mark not running, send the stop signal and release the ticker
	s.stop <- struct{}{}
	s.ticker.Stop()
	s.running = false
	// unlock after sending the signal to avoid deadlock
	s.mu.Unlock()
	// and wait for the reply
	<-s.stopped
}

// Write implements io.Writer, interrupting the spinner and writing to
// the inner writer
func (s *Spinner) Write(p []byte) (n int, err error) {
	// lock first, so nothing else can start writing until we are done
	s.mu.Lock()
	defer s.mu.Unlock()
	// it the spinner is not running, just write directly
	if !s.running {
		return s.writer.Write(p)
	}
	// otherwise: we will rewrite the line first
	if _, err := s.writer.Write([]byte("\r")); err != nil {
		return 0, err
	}
	return s.writer.Write(p)
}
