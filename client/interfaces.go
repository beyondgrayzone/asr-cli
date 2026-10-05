//go:generate mockgen -typed -source=interfaces.go -destination=mocks_test.go -package=main

package main

import (
	"context"
	"io"

	"github.com/gen2brain/malgo"
	"github.com/go-vgo/robotgo"
)

type Connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteJSON(v any) error
	Close() error
}

type AudioSystem interface {
	Devices(kind malgo.DeviceType) ([]malgo.DeviceInfo, error)
}

type ServerConnector interface {
	Connect(ctx context.Context, url string, mode string, lang string, targetLang string, out io.Writer) (Connection, error)
}

type Typer interface {
	Type(text string)
}

type WSConnector struct{}

func (w *WSConnector) Connect(ctx context.Context, url string, mode string, lang string, targetLang string, out io.Writer) (Connection, error) {
	return connectToServer(ctx, url, mode, lang, targetLang, out)
}

type RobotTyper struct{}

func (r *RobotTyper) Type(text string) {
	if text != "" {
		robotgo.TypeStr(text)
	}
}

type NullTyper struct{}

func (n *NullTyper) Type(text string) {}
