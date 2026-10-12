package goxview

import (
	"fmt"
	"sync"
	"time"
)

// Event is an application event delivered by the runner.
type Event any

// EventModel receives renderer input and the default per-frame clock event.
type EventModel interface{ Update(Event) }

type ClockEvent struct {
	Time  time.Time
	Delta time.Duration
	Frame uint64
}

type KeyboardEvent struct {
	Key, Code string
	Repeat    bool
	Pressed   bool
}

type MouseEvent struct {
	X, Y    float64
	Button  int
	Pressed bool
	WheelX  float64
	WheelY  float64
}

// Runner joins the reconciler to a renderer and an application event model.
// Schedule accepts work from any goroutine; Frame executes queued work and
// events on the host goroutine, then flushes resulting state updates.
type Runner struct {
	Root     *Root
	model    EventModel
	wake     func()
	mu       sync.Mutex
	work     []func()
	events   []Event
	lastTime time.Time
	frame    uint64
}

func NewRunner(renderer Renderer, model EventModel, wake func()) *Runner {
	runner := &Runner{model: model, wake: wake}
	var commit func(Node)
	if renderer != nil {
		commit = renderer.Render
	}
	runner.Root = NewRoot(commit, wake)
	if interactive, ok := renderer.(InteractiveRenderer); ok {
		interactive.SetEventSink(runner.Dispatch)
	}
	return runner
}

func (r *Runner) Schedule(work func()) {
	if work == nil {
		return
	}
	r.mu.Lock()
	r.work = append(r.work, work)
	r.mu.Unlock()
	if r.wake != nil {
		r.wake()
	}
}

// Dispatch queues an event, normally called by renderer input adapters.
func (r *Runner) Dispatch(event Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	if r.wake != nil {
		r.wake()
	}
}

func (r *Runner) Frame(now time.Time) (err error) {
	r.mu.Lock()
	work, events := r.work, r.events
	r.work, r.events = nil, nil
	r.mu.Unlock()
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("goxview: scheduled work: %v", v)
		}
	}()
	for _, fn := range work {
		fn()
	}
	for _, event := range events {
		if r.model != nil {
			r.model.Update(event)
		}
	}
	r.frame++
	delta := time.Duration(0)
	if !r.lastTime.IsZero() {
		delta = now.Sub(r.lastTime)
	}
	r.lastTime = now
	if r.model != nil {
		r.model.Update(ClockEvent{Time: now, Delta: delta, Frame: r.frame})
	}
	return r.Root.Flush()
}
