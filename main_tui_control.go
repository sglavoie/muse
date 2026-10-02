package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/notify"
	"github.com/sglavoie/muse/internal/tui"
)

// tuiOptions carries the command-line choices runTUI acts on.
type tuiOptions struct {
	// ControlPath is where --control serves the hosted-session socket. Empty
	// means muse runs unhosted.
	ControlPath string
}

// hosted reports whether a host app drives this session over --control.
func (o tuiOptions) hosted() bool {
	return o.ControlPath != ""
}

// checkPrereqsFor is the prerequisite check for o. A hosted session shows its
// banners through the host, so a missing alerter is not worth a warning there.
func checkPrereqsFor(o tuiOptions) func(context.Context) ([]string, error) {
	opts := config.PrereqOptions{SkipNotifier: o.hosted()}
	return func(ctx context.Context) ([]string, error) {
		return config.CheckPrereqsWith(ctx, opts)
	}
}

// listenControl binds the --control socket and wires its drain and close into
// shutdown. It runs after app.Run, so "another instance is running" still wins
// over a bind error, and before the model is built, so the model can publish to
// it. It returns a nil server when unhosted.
func listenControl(o tuiOptions, shutdownDeps *app.ShutdownDeps) (*control.Server, error) {
	if !o.hosted() {
		return nil, nil
	}
	srv, err := control.Listen(o.ControlPath)
	if err != nil {
		return nil, fmt.Errorf("control socket: %w", err)
	}
	shutdownDeps.DrainControl = srv.Drain
	shutdownDeps.CloseControl = func() { _ = srv.Close() }
	return srv, nil
}

// serveControl introduces this muse to the host and starts answering
// requests, which reach the TUI through send. It is a no-op when srv is nil.
func serveControl(srv *control.Server, send func(tea.Msg)) error {
	if srv == nil {
		return nil
	}
	srv.SetHello(control.Hello{
		PID:      os.Getpid(),
		Version:  resolveVersion(),
		Protocol: control.ProtocolVersion,
	})
	if err := srv.Serve(tui.ControlDispatch(send)); err != nil {
		return fmt.Errorf("control socket: %w", err)
	}
	return nil
}

// trackNotifier mirrors the TUI's notifier port so main can choose one.
type trackNotifier interface {
	NotifyTrack(title, artist, album string) error
}

// notifierFor picks where track-change banners go: through the host when
// hosted, otherwise to the desktop backend. The TUI still decides whether to
// notify at all.
func notifierFor(srv *control.Server) trackNotifier {
	if srv == nil {
		return desktopNotifier{}
	}
	return controlNotifier{pub: srv}
}

// controlModelOptions returns the TUI options a hosted session adds. It never
// hands the TUI a typed-nil publisher.
func controlModelOptions(srv *control.Server) []tui.Option {
	if srv == nil {
		return nil
	}
	return []tui.Option{tui.WithControlPublisher(srv)}
}

// eventPublisher is the part of *control.Server controlNotifier uses.
type eventPublisher interface {
	Publish(event control.Event)
}

// controlNotifier hands track-change banners to the host as notify events. It
// never runs a notification backend itself, so a hosted session shows one
// banner from the host app rather than a second one from alerter.
type controlNotifier struct {
	pub eventPublisher
}

func (n controlNotifier) NotifyTrack(title, artist, album string) error {
	n.pub.Publish(control.Event{
		Event: control.EventNotify,
		Notify: &control.Notification{
			Title:    title,
			Subtitle: artist,
			Body:     notify.Track{Title: title, Artist: artist, Album: album}.Message(),
		},
	})
	return nil
}
