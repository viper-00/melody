package melody

import "sync"

type hub struct {
	sessions   map[*Session]bool
	broadcast  chan *envelope
	register   chan *Session
	unregister chan *Session
	exit       chan *envelope
	open       bool
	rwmutex    *sync.RWMutex
}

func newHub() *hub {
	return &hub{
		sessions:   make(map[*Session]bool),
		broadcast:  make(chan *envelope),
		register:   make(chan *Session),
		unregister: make(chan *Session),
		exit:       make(chan *envelope),
		open:       true,
		rwmutex:    &sync.RWMutex{},
	}
}

func (hub *hub) run() {
loop:
	for {
		select {
		case s := <-hub.register:
			hub.rwmutex.Lock()
			hub.sessions[s] = true
			hub.rwmutex.Unlock()
		case s := <-hub.unregister:
			if _, ok := hub.sessions[s]; ok {
				hub.rwmutex.Lock()
				delete(hub.sessions, s)
				hub.rwmutex.Unlock()
			}
		case message := <-hub.broadcast:
			hub.rwmutex.Lock()
			for s := range hub.sessions {
				if message.filter != nil {
					if message.filter(s) {
						s.writeMessage(message)
					}
				} else {
					s.writeMessage(message)
				}
			}
			hub.rwmutex.Unlock()
		case m := <-hub.exit:
			hub.rwmutex.Lock()
			for s := range hub.sessions {
				s.writeMessage(m)
				delete(hub.sessions, s)
				s.Close()
			}
			hub.open = false
			hub.rwmutex.Unlock()
			break loop
		}
	}
}
