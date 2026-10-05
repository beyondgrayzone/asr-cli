package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/gorilla/websocket"
	"go.uber.org/mock/gomock"
)

func TestPrintMicrophones_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAudio := NewMockAudioSystem(ctrl)

	dev1 := malgo.DeviceInfo{IsDefault: 1}
	dev2 := malgo.DeviceInfo{IsDefault: 0}

	mockAudio.EXPECT().Devices(malgo.Capture).Return([]malgo.DeviceInfo{dev1, dev2}, nil)

	oldRunner := RealRunner
	RealRunner = func(name string, arg ...string) ([]byte, error) {
		return []byte("test-source"), nil
	}
	defer func() { RealRunner = oldRunner }()

	printMicrophones(mockAudio, io.Discard)
}

func TestHandleReadLoop_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockConnection(ctrl)
	mockTyper := NewMockTyper(ctrl)
	disconnectChan := make(chan struct{}, 1)
	buf := new(bytes.Buffer)

	mockConn.EXPECT().ReadMessage().Return(websocket.TextMessage, []byte(`{"text":"hello world"}`), nil)
	mockTyper.EXPECT().Type("hello world")
	mockConn.EXPECT().ReadMessage().Return(0, nil, io.EOF)

	handleReadLoop(mockConn, false, disconnectChan, buf, mockTyper)

	if !bytes.Contains(buf.Bytes(), []byte("hello world")) {
		t.Errorf("output missing expected text, got: %s", buf.String())
	}

	select {
	case <-disconnectChan:
	default:
		t.Fatal("expected disconnect signal")
	}
}

func TestRunCaptureLoop_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConnector := NewMockServerConnector(ctrl)
	mockConn := NewMockConnection(ctrl)
	mockTyper := NewMockTyper(ctrl)
	audioChan := make(chan []byte, 1)

	mockConnector.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockConn, nil).AnyTimes()
	mockConn.EXPECT().ReadMessage().Return(0, nil, errors.New("disconnect")).AnyTimes()

	audioData := []byte{0x01, 0x02}
	mockConn.EXPECT().WriteMessage(websocket.BinaryMessage, audioData).Return(nil).AnyTimes()
	mockConn.EXPECT().Close().Return(nil).AnyTimes()

	audioChan <- audioData

	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(os.Interrupt)
	}()

	runCaptureLoop("ws://localhost:9999", "asr", "en", "", false, audioChan, mockConnector, mockTyper, io.Discard)
}

func TestRunCaptureLoop_ConnectionFailure_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConnector := NewMockServerConnector(ctrl)
	mockTyper := NewMockTyper(ctrl)
	audioChan := make(chan []byte)

	mockConnector.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("refused")).AnyTimes()

	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(os.Interrupt)
	}()

	runCaptureLoop("ws://fail", "asr", "en", "", false, audioChan, mockConnector, mockTyper, io.Discard)
}
