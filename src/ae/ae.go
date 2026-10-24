// original redis code
// - ae.c
// - ae.h

package ae

type BeforeSleepProc func(*EventLoop)

/* State of an event based program */
type EventLoop struct {
	// TODO(EVENT_LOOP): define EventLoop fields (maxfd, events, fired, timeEventHead, etc.)
}

func (eventLoop *EventLoop) beforeSleep() {
	// TODO(EVENT_LOOP): call registered beforeSleep callback
}

func (eventLoop *EventLoop) Main() {
	// TODO(EVENT_LOOP): implement event processing loop (aeProcessEvents)
}

func (eventLoop *EventLoop) DeleteEventLoop() {
	// TODO(EVENT_LOOP): free event loop resources
}
