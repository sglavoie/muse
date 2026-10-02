package player

// BeginShutdown tells the restart supervisor that the process is exiting, so an
// mpv that dies from here on is left dead rather than relaunched. A signal that
// reaches the whole process group (SIGHUP from a closing terminal) kills mpv
// before muse's own Shutdown runs Close; without this flag the supervisor would
// race that Close to start a fresh mpv. It is safe to call more than once and
// from any goroutine, and it does not stop playback itself — Close does that.
func (m *Manager) BeginShutdown() {
	m.shuttingDown.Store(true)
}

// shutdownBegun reports whether BeginShutdown has been called.
func (m *Manager) shutdownBegun() bool {
	return m.shuttingDown.Load()
}
