package multiplex

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	READY = iota
	STARTING
	RUNNING
	DEAD

	IN_ERROR
)

type service struct {
	id     string
	status status

	ctx context.Context
	mtx sync.Mutex
	out io.Writer

	stateChangeHandler func(status)

	cmd      command
	tearDown command
}

// note: thread-unsafe!
func (s *service) changeState(status status) {
	s.status = status
	s.stateChangeHandler(s.status)
}

func (s *service) Start(stableCb func()) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.status == STARTING {
		return errors.New("service is already starting")
	}

	// note: does it make sense considering `stateChangeHandler`?
	// service already stable
	if s.status == RUNNING {
		stableCb()
		return nil
	}

	if s.status == DEAD || s.status == IN_ERROR {
		// note: restore command context based on the service one
		s.cmd.ctx, s.cmd.cancelCtx = context.WithCancel(s.ctx)
		s.changeState(STARTING)
	}

	completed := make(chan error)
	timeGate := make(chan struct{})
	go func() {
		// todo: complete
		if err := s.cmd.Run(); err != nil {
			completed <- err
		} else {
			completed <- nil
		}
	}()
	go func() {
		time.Sleep(time.Duration(5 * time.Second))
		timeGate <- struct{}{}
	}()

	select {
	case <-timeGate:
		s.changeState(RUNNING)
		stableCb()
	case <-completed:
		s.changeState(READY)
	}
	<-timeGate

	return nil
}

func (s *service) TearDown(done func()) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return nil
}

func NewService(ctx context.Context, id string, out io.Writer, path, cmd, tearDown string) *service {
	return &service{
		id:       id,
		out:      out,
		ctx:      ctx,
		mtx:      sync.Mutex{},
		status:   READY,
		cmd:      NewCommand(ctx, path, cmd),
		tearDown: NewCommand(ctx, path, tearDown),
	}
}
