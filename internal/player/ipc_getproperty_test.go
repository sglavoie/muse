package player

import (
	"encoding/json"
	"errors"
	"testing"
)

// newHandlerClient wires a client to a fake mpv answering with handler.
func newHandlerClient(t *testing.T, name string, handler responseHandler) *IPCClient {
	t.Helper()
	sock := shortSockPath(t, name)
	fakeServerWithHandler(t, sock, handler)

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// dataHandler replies successfully with the given raw JSON data.
func dataHandler(data string) responseHandler {
	return func(req ipcTestRequest) ipcTestResponse {
		resp := successHandler(req)
		resp.Data = json.RawMessage(data)
		return resp
	}
}

func TestGetPropertyStringValue(t *testing.T) {
	client := newHandlerClient(t, "getprop-string", dataHandler(`"/music/a.flac"`))

	value, ok, err := client.GetPropertyString(propPath)
	if err != nil {
		t.Fatalf("GetPropertyString: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if value != "/music/a.flac" {
		t.Fatalf("value = %q, want %q", value, "/music/a.flac")
	}
}

func TestGetPropertyStringNull(t *testing.T) {
	client := newHandlerClient(t, "getprop-null", dataHandler(`null`))

	value, ok, err := client.GetPropertyString(propPath)
	if err != nil {
		t.Fatalf("GetPropertyString: %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false for null data")
	}
	if value != "" {
		t.Fatalf("value = %q, want empty", value)
	}
}

func TestGetPropertyStringUnavailable(t *testing.T) {
	client := newHandlerClient(t, "getprop-unavail", errorHandler("property unavailable"))

	_, ok, err := client.GetPropertyString(propPath)
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("err = %v, want ErrPlaybackFailed", err)
	}
	if ok {
		t.Fatal("ok = true, want false on error")
	}
}

func TestGetPropertyBoolRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
		ok   bool
	}{
		{"true", `true`, true, true},
		{"false", `false`, false, true},
		{"null", `null`, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newHandlerClient(t, "getprop-bool-"+tt.name, dataHandler(tt.data))

			value, ok, err := client.GetPropertyBool(propPause)
			if err != nil {
				t.Fatalf("GetPropertyBool: %v", err)
			}
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if value != tt.want {
				t.Fatalf("value = %v, want %v", value, tt.want)
			}
		})
	}
}

func TestGetPropertyBoolWrongType(t *testing.T) {
	client := newHandlerClient(t, "getprop-bool-wrong", dataHandler(`"yes"`))

	if _, _, err := client.GetPropertyBool(propPause); err == nil {
		t.Fatal("GetPropertyBool: want error for non-bool data, got nil")
	}
}

func TestGetPropertyBoolUnavailable(t *testing.T) {
	client := newHandlerClient(t, "getprop-bool-unavail", errorHandler("property unavailable"))

	if _, _, err := client.GetPropertyBool(propPause); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("err = %v, want ErrPlaybackFailed", err)
	}
}
