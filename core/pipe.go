package core

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"gopkg.in/sensorbee/sensorbee.v0/data"
)

func newPipe(inputName string, capacity int) (*pipeReceiver, *pipeSender) {
	p := make(chan *Tuple, capacity) // TODO: the type should be chan []*Tuple

	r := &pipeReceiver{
		in: p,
	}

	s := &pipeSender{
		inputName: inputName,
		out:       p,
	}
	r.sender = s
	return r, s
}

type pipeReceiver struct {
	in     <-chan *Tuple
	sender *pipeSender
}

// close closes the channel from the receiver side. It doesn't directly close
// the channel. Instead, it sends a signal to the sender so that sender can
// close the channel.
func (r *pipeReceiver) close() {
	// Close the sender because close(r.in) isn't safe. This method uses a
	// goroutine because it might be dead-locked when the channel is full.
	// In that case pipeSender.Write blocks at s.out <- t having RLock.
	// Then, calling sender.close() is blocked until someone reads a tuple
	// from r.in.
	go func() {
		r.sender.close()
	}()
}

// QueueDropMode is a mode which controls the behavior of dropping tuples from
// a pipe when an output tuple is written to its queue that is full. This mode
// doesn't care about precision of computation results. When a Box connected to
// an output queue supports sophisticated load shedding algorithm, specify
// DropNone.
type QueueDropMode int

const (
	// DropNone is one of QueueDropMode that a Source or Box doesn't drop any
	// tuple when its output queue is full. This is the default mode.
	DropNone QueueDropMode = iota

	// DropLatest is one of QueueDropMode that a Source and a Box drops the
	// latest tuple (i.e. the tuple which is being sent) when its output queue
	// is full.
	DropLatest

	// DropOldest is one of QueueDropMode that a Source and a Box drops the
	// oldest tuple being queued when its output queue is full.
	DropOldest
)

// pipeSender represents a pipe sender. An object of this struct must be
// placed in a global variable or in memory allocated from the heap.
// Using an array or a slice of pipeSender may cause panic even if it is
// global or allocated from the heap. Read dataDestination godoc or
// https://github.com/golang/go/issues/9959 for details.
type pipeSender struct {
	// cnt is the number of tuples written to this pipe. This value may not
	// be accurate when the sender is registered to multiple destinations.
	// This can be solved by sharing the same chan with multiple pipe instances.
	// To support it, we should create a sharedChan which manages the chan with
	// reference counting. registeredDsts won't have to be a slice after
	// applying this change.
	// cnt is the first field of this struct for 64-bit alignment.
	cnt int64

	inputName string
	out       chan *Tuple
	dropMode  QueueDropMode

	// rwm protects out from write-close conflicts.
	rwm sync.RWMutex

	registeredDsts []struct {
		registeredName string
		dst            *dataDestinations
	}
	closed bool
}

// Write outputs the given tuple to the pipe. This method only returns
// errPipeClosed and never panics.
func (s *pipeSender) Write(ctx *Context, t *Tuple) error {
	// A benchmark result has shown that passing a closure here is 10% faster
	// than passing a function which does nothing.
	return s.write(ctx, t, func(*Tuple) {})
}

func (s *pipeSender) write(ctx *Context, in *Tuple, droppedTuple func(*Tuple)) error {
	s.rwm.RLock()
	defer s.rwm.RUnlock()

	if s.closed {
		return errPipeClosed
	}

	t := in
	if t.Flags.IsSet(TFShared) {
		t = in.ShallowCopy()
	}
	t.InputName = s.inputName

	if s.dropMode == DropNone {
		s.out <- t
	} else {
	sendLoop:
		for {
			select {
			case s.out <- t:
				break sendLoop
			default:
				if s.dropMode == DropLatest {
					droppedTuple(t)
					return nil
				}

				// The mode is DropOldest, so it takes the oldest one and try
				// again in the next iteration. This loop can cause starvation.
				select {
				case dropped := <-s.out:
					droppedTuple(dropped)
				default: // Another thread may drop it before this thread does.
				}
			}
		}
	}
	atomic.AddInt64(&s.cnt, 1)
	return nil
}

// Close closes a channel. When multiple goroutines try to close the channel,
// only one goroutine can actually close it. Other goroutines don't wait until
// the channel is actually closed. Close never fails.
func (s *pipeSender) Close(ctx *Context) error {
	// Close method is provided to support WriteCloser interface.
	s.close()
	return nil
}

func (s *pipeSender) close() {
	s.rwm.Lock()
	defer s.rwm.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.out)

	// Remove the sender from all destinations to notify owners of
	// dataDestinations that a sender is removed from them. Without this,
	// a notification won't be sent until someone write a tuple to
	// dataDestination.
	for _, d := range s.registeredDsts {
		// There can be a circular recursive call like pipeSender.close ->
		// dataDestinations.remove -> pipeSender.close. So, this should be
		// called via goroutine.
		go d.dst.remove(d.registeredName)
	}
	s.registeredDsts = nil
}

func (s *pipeSender) registered(name string, dst *dataDestinations) {
	s.rwm.Lock()
	defer s.rwm.Unlock()
	s.registeredDsts = append(s.registeredDsts, struct {
		registeredName string
		dst            *dataDestinations
	}{name, dst})
}

func (s *pipeSender) count() int64 {
	return atomic.LoadInt64(&s.cnt)
}

func (s *pipeSender) queueStatus() (int, int) {
	s.rwm.RLock()
	defer s.rwm.RUnlock()
	if s.closed {
		return 0, 0
	}
	return len(s.out), cap(s.out)
}

func (s *pipeSender) isClosed() bool {
	s.rwm.RLock()
	defer s.rwm.RUnlock()
	return s.closed
}

// dataSources represents data sources. This struct has atomic integers
// and golang does not offer alignment attributes, so it is a user's
// responsibility to align them. A user must neither allocate dataSources
// on a stack nor use an array or slice of dataSources.
// Read godoc for dataDestinations or https://github.com/golang/go/issues/9959
// for details.
type dataSources struct {
	// numReceived and numErrors must be here for 64-bit alignment.
	// See godoc for this struct.
	numReceived int64
	numErrors   int64

	nodeType NodeType
	nodeName string

	// m protects state, recvs, and msgChs.
	m     sync.RWMutex
	state *topologyStateHolder

	recvs map[string]*pipeReceiver

	// msgChs is a slice of channels which are connected to goroutines
	// pouring tuples. They receive controlling messages through this channel.
	msgChs []chan<- *dataSourcesMessage
}

func newDataSources(nodeType NodeType, nodeName string) *dataSources {
	s := &dataSources{
		nodeType: nodeType,
		nodeName: nodeName,
		recvs:    map[string]*pipeReceiver{},
	}
	s.state = newTopologyStateHolder(&s.m)
	return s
}

type dataSourcesMessage struct {
	cmd dataSourcesCommand
	v   interface{}
}

type dataSourcesCommand int

const (
	ddscAddReceiver dataSourcesCommand = iota
	ddscStop
	ddscToggleGracefulStop
	ddscStopOnDisconnect
)

func (s *dataSources) add(name string, r *pipeReceiver) error {
	// Because dataSources is used internally and shouldn't return error
	// in most cases, there's no need to check s.recvs with RLock before
	// actually acquiring Lock.
	s.m.Lock()
	defer s.m.Unlock()
	if s.state.getWithoutLock() >= TSStopping {
		return fmt.Errorf("node '%v' already closed its input", s.nodeName)
	}

	// It's safe even if pouringThread panics while executing this method
	// because pour method will drain all channels in s.recvs. So, the sender
	// won't block even if pouringThread doesn't receive the new pipe receiver.
	if _, ok := s.recvs[name]; ok {
		return fmt.Errorf("node '%v' is already receiving tuples from '%v'", s.nodeName, name)
	}
	s.recvs[name] = r
	// It is not necessary to send messages before pour() call.
	if len(s.msgChs) > 0 {
		s.sendMessageWithoutLock(&dataSourcesMessage{
			cmd: ddscAddReceiver,
			v:   r,
		})
	}
	return nil
}

func (s *dataSources) sendMessage(msg *dataSourcesMessage) {
	s.m.Lock()
	defer s.m.Unlock()
	s.sendMessageWithoutLock(msg)
}

func (s *dataSources) sendMessageWithoutLock(msg *dataSourcesMessage) {
	// wait for pour().
	// Note that it is the user's responsibility to avoid deadlock.
	// Users must call a function which modify s.state to TSRunning or greater
	// (e.g. pour()) before call of this function or in another goroutine.
	s.state.waitWithoutLock(TSRunning)
	// Applying range-based for to a nil slice is valid, so this loop is safe
	// even when s.state is TSStopping or TSStopped.
	for _, ch := range s.msgChs {
		ch <- msg
	}
}

func (s *dataSources) remove(name string) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.state.getWithoutLock() >= TSStopping {
		return
	}

	// Removing receiver here can result in a dead-lock when pouringThread
	// panics while the channel of receiver is full. In that case, there's no
	// reader who reads a tuple and unblock the sender. To avoid this problem,
	// pouringThread returns all input channels it had at the time the failure
	// occurred and pour method will read tuples from those channels.
	r, ok := s.recvs[name]
	if !ok {
		return
	}
	delete(s.recvs, name)
	r.close() // This eventually closes the channel.
}

// pour pours out tuples for the target Writer. The target must directly be
// connected to a Box or a Sink.
func (s *dataSources) pour(ctx *Context, w Writer, parallelism int) error {
	if parallelism == 0 {
		parallelism = 1
	}

	var (
		wg            sync.WaitGroup
		logOnce       sync.Once
		collectInputs sync.Once
		inputs        []reflect.SelectCase
		threadErr     error
	)

	err := func() error {
		s.m.Lock()
		defer s.m.Unlock()
		if st, err := s.state.checkAndPrepareForRunningWithoutLock(); err != nil {
			switch st {
			case TSRunning, TSPaused:
				return fmt.Errorf("'%v' already started to receive tuples", s.nodeName)
			case TSStopped:
				return fmt.Errorf("'%v' already stopped receiving tuples", s.nodeName)
			default:
				return fmt.Errorf("'%v' has invalid state: %v", s.nodeName, st)
			}
		}

		genCases := func(msgCh <-chan *dataSourcesMessage) []reflect.SelectCase {
			cs := make([]reflect.SelectCase, 0, len(s.recvs)+2)
			cs = append(cs, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(msgCh),
			})

			// This case is used as a default case after stopCh is closed.
			// Currently, it only has recv with nil channel so that
			// reflect.Select does nothing on it.
			cs = append(cs, reflect.SelectCase{
				Dir: reflect.SelectRecv,
			})

			for _, r := range s.recvs {
				cs = append(cs, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(r.in),
				})
			}
			return cs
		}

		// ensureLocked ensures proper lock for s. Removing this introduces
		// race conditions because genCases requires locked s and genCases is
		// called in goroutines.
		var ensureLocked sync.WaitGroup
		for i := 0; i < parallelism; i++ {
			msgCh := make(chan *dataSourcesMessage)
			s.msgChs = append(s.msgChs, msgCh)

			wg.Add(1)
			ensureLocked.Add(1)
			go func() {
				defer wg.Done()
				needDone := true
				defer func() {
					if needDone {
						ensureLocked.Done()
					}
				}()
				cs := genCases(msgCh)
				ensureLocked.Done()
				needDone = false
				ins, err := s.pouringThread(ctx, w, cs)
				collectInputs.Do(func() {
					// It's sufficient to collect input only once. The only
					// problem which might happen is that ins has old receivers.
					// However, they will simply be removed when calling
					// reflect.Select. There might be a case that only one
					// pouringThread has a newly added receiver but it isn't
					// assigned to inputs. To solve that problem, pour method
					// also reads tuples from s.recvs.
					//
					// In addition, when pouringThread panics while remove
					// method is called, s.recvs might not have receivers
					// which pour method should read tuples. However, inputs
					// returned from pouringThread has them. If the returned
					// inputs doesn't have them, that means they were already
					// removed successfully.
					//
					// In conclusion, by combining s.recvs and inputs, all
					// inputs can be drained and no sender will be blocked.
					inputs = ins
				})
				if err != nil {
					logOnce.Do(func() {
						threadErr = err // return only one error
						ctx.ErrLog(err).WithFields(nodeLogFields(s.nodeType, s.nodeName)).
							Error("the node stopped with a fatal error")
					})
				}
			}()
		}
		ensureLocked.Wait()
		return nil
	}()
	if err != nil {
		return err
	}

	s.state.Set(TSRunning)
	wg.Wait()

	s.m.Lock()
	defer func() {
		s.state.setWithoutLock(TSStopped)
		s.m.Unlock()
	}()

	drainTargets := make([]reflect.SelectCase, 0, len(s.recvs)+len(inputs))
	drainTargets = append(drainTargets, inputs...)
	for _, r := range s.recvs {
		drainTargets = append(drainTargets, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(r.in),
		})
		r.close()
	}
	s.recvs = nil

	go func() {
		// drainTargets might have duplicated channels but it doesn't cause
		// a problem.
		for len(drainTargets) != 0 {
			i, _, ok := reflect.Select(drainTargets)
			if ok {
				continue
			}
			drainTargets[i] = drainTargets[len(drainTargets)-1]
			drainTargets = drainTargets[:len(drainTargets)-1]
		}
	}()

	for _, ch := range s.msgChs {
		close(ch)
	}
	s.msgChs = nil
	return threadErr
}

func (s *dataSources) pouringThread(ctx *Context, w Writer, cs []reflect.SelectCase) (inputs []reflect.SelectCase, retErr error) {
	const (
		message = iota
		defaultCase

		// maxControlIndex has the max index of special channels used to
		// control this method.
		maxControlIndex = defaultCase
	)

	defer func() {
		if e := recover(); e != nil {
			if err, ok := e.(error); ok {
				if !IsFatalError(err) {
					err = FatalError(err)
				}
				retErr = err
			} else {
				retErr = fmt.Errorf("'%v' got an unknown error through panic: %v", s.nodeName, e)
			}
		}

		if len(cs) > maxControlIndex+1 {
			// Return all inputs this method still has. pour method will read
			// tuples from those input channels so that senders don't block.
			inputs = cs[maxControlIndex+1:]
		}

		// drain channels specific to pouringThread
		go func() {
			cs = cs[:1] // exclude defaultCase
			for len(cs) != 0 {
				i, _, ok := reflect.Select(cs)
				if ok {
					continue
				}
				cs[i] = cs[len(cs)-1]
				cs = cs[:len(cs)-1]
			}
		}()
	}()

	gracefulStopEnabled := false
	stopOnDisconnect := false

	reportDT := func(t *Tuple, err error) {
		ctx.droppedTuple(t, s.nodeType, s.nodeName, ETInput, err)
	}

receiveLoop:
	for {
		if stopOnDisconnect && len(cs) == maxControlIndex+1 {
			// When stopOnDisconnect is enabled, this loop breaks if the data
			// source doesn't have any input channel. Otherwise, it keeps
			// running because a new input could dynamically be added.
			break
		}

		i, v, ok := reflect.Select(cs) // all cases are receive direction
		if !ok && i != defaultCase {
			if i <= maxControlIndex {
				retErr = FatalError(fmt.Errorf("a controlling channel (%v) of '%v' has been closed", i, s.nodeName))
				return
			}

			// remove the closed channel by swapping it with the last element.
			cs[i], cs[len(cs)-1] = cs[len(cs)-1], cs[i]
			cs = cs[:len(cs)-1]
			continue
		}

		switch i {
		case message:
			msg, ok := v.Interface().(*dataSourcesMessage)
			if !ok {
				ctx.Log().WithFields(nodeLogFields(s.nodeType, s.nodeName)).
					Warnf("Received an invalid control message in dataSources: %v", v.Interface())
				continue
			}

			switch msg.cmd {
			case ddscAddReceiver:
				c, ok := msg.v.(*pipeReceiver)
				if !ok {
					ctx.Log().WithFields(nodeLogFields(s.nodeType, s.nodeName)).
						Warn("Cannot add a new receiver due to a type error")
					break
				}
				cs = append(cs, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(c.in),
				})

			case ddscStop:
				if !gracefulStopEnabled {
					break receiveLoop
				}
				cs[defaultCase].Dir = reflect.SelectDefault // activate the default case

			case ddscToggleGracefulStop:
				gracefulStopEnabled = true

			case ddscStopOnDisconnect:
				stopOnDisconnect = true
			}

		case defaultCase:
			// stop has been called and there's no additional input.
			break receiveLoop

		default:
			atomic.AddInt64(&s.numReceived, 1)
			t, ok := v.Interface().(*Tuple)
			if !ok {
				atomic.AddInt64(&s.numErrors, 1)
				ctx.Log().WithFields(nodeLogFields(s.nodeType, s.nodeName)).
					Error("Cannot receive a tuple from a receiver due to a type error")
				break
			}

			err := w.Write(ctx, t)
			if err == nil {
				break
			}

			atomic.AddInt64(&s.numErrors, 1)
			switch {
			case IsFatalError(err):
				// logging is done by pour method
				retErr = err
				reportDT(t, err)
				return

			case IsTemporaryError(err):
				// TODO: retry
				reportDT(t, err) // TODO: don't write a tuple until retry fails

			default:
				// Skip this tuple
				reportDT(t, err)
			}
		}
	}
	return // return values will be set by the deferred function.
}

// enableGracefulStop enables graceful stop mode. If the mode is enabled, the
// source automatically stops when it doesn't receive any input after stop is
// called.
func (s *dataSources) enableGracefulStop() {
	// Perhaps this function should be something like 'toggle', but it wasn't
	// necessary at the time of this writing.
	s.sendMessage(&dataSourcesMessage{
		cmd: ddscToggleGracefulStop,
	})
}

// stopOnDisconnect activates automatic stop when the dataSources has
// no receiver.
func (s *dataSources) stopOnDisconnect() {
	s.sendMessage(&dataSourcesMessage{
		cmd: ddscStopOnDisconnect,
	})
}

// stop stops the source after processing tuples which it currently has.
func (s *dataSources) stop(ctx *Context) {
	s.m.Lock()
	defer s.m.Unlock()
	if stopped, err := s.state.checkAndPrepareForStoppingWithoutLock(false); stopped || err != nil {
		return
	}

	for _, r := range s.recvs {
		// This eventually closes the channels and remove edges from
		// data sources.
		r.close()
	}
	s.recvs = nil

	s.sendMessageWithoutLock(&dataSourcesMessage{
		cmd: ddscStop,
	})
	s.state.waitWithoutLock(TSStopped)
}

func (s *dataSources) status() data.Map {
	// mutex of the dataSources doesn't block reading tuples from a channel.
	s.m.Lock()
	defer s.m.Unlock()

	st := data.Map{}
	st["num_received_total"] = data.Int(atomic.LoadInt64(&s.numReceived))
	st["num_errors"] = data.Int(atomic.LoadInt64(&s.numErrors))
	// TODO: Add num_temporary_errors and num_retries.

	m := make(data.Map, len(s.recvs))
	for name, recv := range s.recvs {
		if recv.sender.isClosed() {
			delete(s.recvs, name)
			continue
		}

		l, c := recv.sender.queueStatus()
		m[name] = data.Map{
			"num_received": data.Int(recv.sender.count() - int64(l)),
			"queue_size":   data.Int(c),
			"num_queued":   data.Int(l),
		}
	}
	st["inputs"] = m
	return st
}

// dataDestinations have writers connected to multiple destination nodes and
// distributes tuples to them. It is the user's responsibility to store an object
// of this struct in 64-bit aligned memory.
// A global variable and an first address of allocated memory from heap are
// guaranteed to have 64-bit alignment. Memory allocated on stack has no such guarantee.
// Use of a slice or an array of dataDestinations must be avoided because the second
// element may not have 64-bit alignment. []*dataDestinations is okay if all pointed
// objects have 64-bit alignment. This restriction is introduced to access int64
// fields atomically on 32-bit machines.
// See: https://github.com/golang/go/issues/9959
type dataDestinations struct {
	// numSent and numDropped must be here for 64-bit alignment.
	// See godoc for this struct.
	numSent    int64
	numDropped int64

	nodeType NodeType

	// nodeName is the name of the node which writes tuples to
	// destinations (i.e. the name of a Source or a Box).
	nodeName string
	rwm      sync.RWMutex
	cond     *sync.Cond
	dsts     map[string]*pipeSender
	paused   bool

	callback func(ddEvent)
}

type ddEvent int

const (
	// ddeNewConn is sent when a new connection is added. When this event is
	// sent, the callback must not call any method of dataDestinations.
	ddeNewConn ddEvent = iota

	// ddeDisconnect is sent when all connections are closed. When this event
	// is sent, the callback can safely call other methods of dataDestinations.
	ddeDisconnect
)

func newDataDestinations(nodeType NodeType, nodeName string) *dataDestinations {
	d := &dataDestinations{
		nodeType: nodeType,
		nodeName: nodeName,
		dsts:     map[string]*pipeSender{},
	}
	d.cond = sync.NewCond(&d.rwm)
	return d
}

func (d *dataDestinations) add(name string, s *pipeSender) error {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	if d.dsts == nil {
		return fmt.Errorf("node '%v' already closed its output", d.nodeName)
	}

	if _, ok := d.dsts[name]; ok {
		return fmt.Errorf("node '%v' already has the destination '%v'", d.nodeName, name)
	}
	d.dsts[name] = s
	s.registered(name, d)
	if d.callback != nil {
		// This isn't called via goroutine because calling it via goroutine
		// might result in inconsistent ordering (e.g. ddeDisconnect can be
		// sent before the first ddeNewConn). As a consequence, callbacks
		// receiving ddeNewConn must not call any method of dataDestinations
		// to avoid deadlocks.
		d.callback(ddeNewConn)
	}
	return nil
}

func (d *dataDestinations) remove(name string) {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	if d.dsts == nil {
		return
	}

	dst, ok := d.dsts[name]
	if !ok {
		return
	}
	delete(d.dsts, name)
	dst.close()
	if len(d.dsts) == 0 && d.callback != nil {
		// This is called by a goroutine so that callback can call other methods
		// of this dataDestinations without being deadlocked.
		go d.callback(ddeDisconnect)
	}
}

func (d *dataDestinations) len() int {
	d.rwm.RLock()
	defer d.rwm.RUnlock()
	return len(d.dsts)
}

// Write writes tuples to destinations. It doesn't return any error including
// errPipeClosed.
func (d *dataDestinations) Write(ctx *Context, t *Tuple) error {
	d.rwm.RLock()
	shouldUnlock := true
	defer func() {
		if shouldUnlock {
			d.rwm.RUnlock()
		}
	}()

	// RLock will be acquired again by the end of the loop.
	for d.paused {
		d.rwm.RUnlock()

		// assuming d.cond.Wait doesn't panic.
		d.rwm.Lock()
		for d.paused {
			d.cond.Wait()
		}
		d.rwm.Unlock()

		// Because paused can be changed in this Unlock -> RLock interval,
		// its value has to be checked again in the next iteration.

		d.rwm.RLock()
	}
	// It's safe even if Close method is called while waiting in the loop above.

	if len(d.dsts) == 0 {
		atomic.AddInt64(&d.numDropped, 1)
		if ctx.Flags.DestinationlessTupleLog.Enabled() {
			ctx.droppedTuple(t, d.nodeType, d.nodeName, ETOutput, errors.New("no output destination is connected"))
		}
		return nil
	}

	reportFunc := func(dropped *Tuple) {
		ctx.droppedTuple(t, d.nodeType, d.nodeName, ETOutput, errors.New("the output queue is full"))
	}

	if len(d.dsts) > 1 {
		// If TFShared is already set, it doesn't have to be set again and
		// setting the flag again is also safe (as long as any Box doesn't
		// clear the flag). If the flag isn't set, it means that this Write
		// method can safely modify the tuple, so the flag can be set here.
		// Therefore, just setting TFShared here works fine with any condition.
		t.Flags.Set(TFShared)
	}
	var closed []string
	for name, dst := range d.dsts {
		// TODO: recovering from panic here instead of using RWLock in
		// pipeSender might be faster.

		if err := dst.write(ctx, t, reportFunc); err != nil { // never panics
			// err is always errPipeClosed when it isn't nil.
			// Because the closed destination doesn't do anything harmful,
			// it'll be removed later for performance reason.

			closed = append(closed, name)
		}
	}

	if closed != nil {
		shouldUnlock = false
		d.rwm.RUnlock()
		d.rwm.Lock()
		defer d.rwm.Unlock()
		for _, n := range closed {
			delete(d.dsts, n)
		}
		if len(d.dsts) == 0 && d.callback != nil {
			// This has to be called asynchronously because Write may be called
			// from dataSources.pour and callback would be able to call
			// dataSources.stop, which might end up with a dead-lock.
			go d.callback(ddeDisconnect)
		}
	}
	atomic.AddInt64(&d.numSent, 1)
	return nil
}

func (d *dataDestinations) pause() {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	d.setPaused(true)
}

func (d *dataDestinations) resume() {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	d.setPaused(false)
}

func (d *dataDestinations) setPaused(p bool) {
	if d.paused == p {
		return
	}
	d.paused = p
	d.cond.Broadcast()
}

func (d *dataDestinations) Close(ctx *Context) error {
	d.rwm.Lock()
	defer d.rwm.Unlock()
	for _, dst := range d.dsts {
		dst.close()
	}
	d.dsts = nil
	d.setPaused(false)
	return nil
}

func (d *dataDestinations) status() data.Map {
	d.rwm.RLock()
	defer d.rwm.RUnlock()

	st := data.Map{}
	st["num_sent_total"] = data.Int(atomic.LoadInt64(&d.numSent))
	st["num_dropped"] = data.Int(atomic.LoadInt64(&d.numDropped))

	m := make(data.Map, len(d.dsts))
	for name, dst := range d.dsts {
		l, c := dst.queueStatus()
		m[name] = data.Map{
			"num_sent":   data.Int(dst.count()),
			"queue_size": data.Int(c),
			"num_queued": data.Int(l),
		}
	}
	st["outputs"] = m
	return st
}
