package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRobotTyper_NullTyper(t *testing.T) {
	// Simple sanity test for Typer implementations
	var _ Typer = &RobotTyper{}
	var _ Typer = &NullTyper{}
}

func TestParseDeviceInfo(t *testing.T) {
	mockDefaultSource := "alsa_input.pci-0000_00_1f.3.analog-stereo"
	mockListSources := `
Source #1
	Name: other_source
	Active Port: some-port

Source #2
	Name: alsa_input.pci-0000_00_1f.3.analog-stereo
	Ports:
		analog-input-mic: Microphone (priority: 8700, latency offset: 0 usec)
		analog-input-internal-mic: Internal Microphone (priority: 8900)
	Active Port: analog-input-mic
`
	tests := []struct {
		name           string
		defaultIn      string
		listIn         string
		wantSourceName string
		wantPortDesc   string
	}{
		{"Standard", mockDefaultSource, mockListSources, "alsa_input.pci-0000_00_1f.3.analog-stereo", "Microphone"},
		{"NoPorts", "empty", "Source #1\nName: empty\nActive Port: none", "empty", ""},
		{"Empty", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSrc, gotPort := parseDeviceInfo(tt.defaultIn, tt.listIn)
			if gotSrc != tt.wantSourceName || gotPort != tt.wantPortDesc {
				t.Errorf("got %v, %v; want %v, %v", gotSrc, gotPort, tt.wantSourceName, tt.wantPortDesc)
			}
		})
	}
}

func TestGetLinuxActiveDeviceInfo(t *testing.T) {
	runner := func(name string, arg ...string) ([]byte, error) {
		if arg[0] == "get-default-source" {
			return []byte("test-source"), nil
		}
		return []byte("Source #1\nName: test-source\nActive Port: p1\np1: Mic"), nil
	}
	src, port := getLinuxActiveDeviceInfo(runner)
	if src != "test-source" || port != "Mic" {
		t.Errorf("expected test-source/Mic, got %s/%s", src, port)
	}

	failRunner := func(name string, arg ...string) ([]byte, error) {
		return nil, errors.New("fail")
	}
	src, port = getLinuxActiveDeviceInfo(failRunner)
	if src != "" || port != "" {
		t.Errorf("expected empty on failure")
	}
}

func TestFormatOutput(t *testing.T) {
	speaker := 1
	tests := []struct {
		name       string
		out        OutputMsg
		timestamps bool
		lastSpk    *int
		want       string
	}{
		{
			"Simple Text",
			OutputMsg{Text: "hello"},
			false,
			new(int),
			"hello",
		},
		{
			"Speaker Change",
			OutputMsg{Text: "hello", SpeakerID: &speaker},
			false,
			new(int),
			"\n[Speaker 1] hello",
		},
		{
			"Timestamps",
			OutputMsg{Words: []WordTimestampMsg{{Word: "hi", StartSecs: 0, EndSecs: 1}}},
			true,
			new(int),
			"[0.00s - 1.00s] hi\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			formatOutput(buf, tt.out, tt.timestamps, tt.lastSpk)
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestConnectToServer(t *testing.T) {
	upgrader := websocket.Upgrader{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := upgrader.Upgrade(w, r, nil)
		var cfg ConfigMsg
		_ = c.ReadJSON(&cfg)
		_ = c.WriteMessage(websocket.TextMessage, []byte("ready"))
		c.Close()
	}))
	defer s.Close()

	conn, err := connectToServer(context.Background(), "ws"+s.URL[4:], "asr", "en", "", io.Discard)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	conn.Close()
}

func TestHandleReadLoop_WithWriter(t *testing.T) {
	upgrader := websocket.Upgrader{}
	done := make(chan bool)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := upgrader.Upgrade(w, r, nil)
		msg, _ := json.Marshal(OutputMsg{Text: "test_output"})
		_ = c.WriteMessage(websocket.TextMessage, msg)
		c.Close()
		done <- true
	}))
	defer s.Close()

	u := "ws" + s.URL[4:]
	conn, _, _ := websocket.DefaultDialer.Dial(u, nil)

	buf := new(bytes.Buffer)
	disconnect := make(chan struct{})

	go handleReadLoop(conn, false, disconnect, buf, &NullTyper{})

	<-done
	<-disconnect

	if !strings.Contains(buf.String(), "test_output") {
		t.Errorf("expected test_output in buffer, got %s", buf.String())
	}
}
