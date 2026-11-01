package mpris

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// Start connects to the session bus, exports the player, and claims
// org.mpris.MediaPlayer2.muse, falling back to a per-process name if another
// muse holds it. Hand the service to the TUI as a state observer, call Serve
// once requests can reach the TUI, and Close it at shutdown. It returns
// ErrUnavailable when there is no session bus.
func Start() (*Service, error) {
	addr := sessionBusAddress(os.Getenv, fileExists)
	if addr == "" {
		return nil, ErrUnavailable
	}
	conn, err := dbus.Connect(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	s := newService(nil, func(name string, values ...any) error {
		return conn.Emit(objectPath, name, values...)
	}, time.Now)
	s.closer = conn.Close
	if err := export(conn, s); err != nil {
		_ = conn.Close()
		return nil, err
	}
	name, err := claimName(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	s.busName = name
	return s, nil
}

// sessionBusAddress finds the running session bus without ever starting one:
// godbus's own lookup falls back to dbus-launch, which would leave a stray
// daemon behind on a headless or SSH session.
func sessionBusAddress(getenv func(string) string, exists func(string) bool) string {
	if addr := getenv("DBUS_SESSION_BUS_ADDRESS"); addr != "" {
		return addr
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		if path := filepath.Join(dir, "bus"); exists(path) {
			return "unix:path=" + path
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func export(conn *dbus.Conn, s *Service) error {
	exports := []struct {
		v       any
		mapping map[string]string
		iface   string
	}{
		{rootIface{s}, nil, ifaceRoot},
		{playerIface{s}, map[string]string{"SeekBy": "Seek"}, ifacePlayer},
		{propsIface{s}, nil, ifaceProps},
		{introspect.Introspectable(introspection), nil, "org.freedesktop.DBus.Introspectable"},
	}
	for _, e := range exports {
		if err := conn.ExportWithMap(e.v, e.mapping, objectPath, e.iface); err != nil {
			return fmt.Errorf("mpris: export %s: %w", e.iface, err)
		}
	}
	return nil
}

// claimName takes the well-known name, or a per-process one when another
// player already holds it, as the spec suggests for multiple instances.
func claimName(conn *dbus.Conn) (string, error) {
	for _, name := range []string{
		busNamePrefix + identity,
		fmt.Sprintf("%s%s.instance%d", busNamePrefix, identity, os.Getpid()),
	} {
		reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
		if err != nil {
			return "", fmt.Errorf("mpris: request %s: %w", name, err)
		}
		if reply == dbus.RequestNameReplyPrimaryOwner {
			return name, nil
		}
	}
	return "", fmt.Errorf("mpris: bus names for %s are taken", identity)
}

// introspection describes the exported object for clients that ask.
const introspection = introspect.IntrospectDeclarationString + `<node>
  <interface name="org.mpris.MediaPlayer2">
    <method name="Raise"/>
    <method name="Quit"/>
    <property name="CanQuit" type="b" access="read"/>
    <property name="CanRaise" type="b" access="read"/>
    <property name="HasTrackList" type="b" access="read"/>
    <property name="Identity" type="s" access="read"/>
    <property name="SupportedUriSchemes" type="as" access="read"/>
    <property name="SupportedMimeTypes" type="as" access="read"/>
  </interface>
  <interface name="org.mpris.MediaPlayer2.Player">
    <method name="Next"/>
    <method name="Previous"/>
    <method name="Pause"/>
    <method name="PlayPause"/>
    <method name="Stop"/>
    <method name="Play"/>
    <method name="Seek"><arg name="Offset" type="x" direction="in"/></method>
    <method name="SetPosition">
      <arg name="TrackId" type="o" direction="in"/>
      <arg name="Position" type="x" direction="in"/>
    </method>
    <method name="OpenUri"><arg name="Uri" type="s" direction="in"/></method>
    <signal name="Seeked"><arg name="Position" type="x"/></signal>
    <property name="PlaybackStatus" type="s" access="read"/>
    <property name="Rate" type="d" access="read"/>
    <property name="Metadata" type="a{sv}" access="read"/>
    <property name="Position" type="x" access="read"/>
    <property name="MinimumRate" type="d" access="read"/>
    <property name="MaximumRate" type="d" access="read"/>
    <property name="CanGoNext" type="b" access="read"/>
    <property name="CanGoPrevious" type="b" access="read"/>
    <property name="CanPlay" type="b" access="read"/>
    <property name="CanPause" type="b" access="read"/>
    <property name="CanSeek" type="b" access="read"/>
    <property name="CanControl" type="b" access="read"/>
  </interface>
  <interface name="org.freedesktop.DBus.Properties">
    <method name="Get">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="out"/>
    </method>
    <method name="GetAll">
      <arg name="interface" type="s" direction="in"/>
      <arg name="properties" type="a{sv}" direction="out"/>
    </method>
    <method name="Set">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="in"/>
    </method>
    <signal name="PropertiesChanged">
      <arg name="interface" type="s"/>
      <arg name="changed" type="a{sv}"/>
      <arg name="invalidated" type="as"/>
    </signal>
  </interface>
</node>`
