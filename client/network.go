package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gorilla/websocket"
)

func connectToServer(ctx context.Context, url string, mode string, lang string, targetLang string, out io.Writer) (Connection, error) {
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	cfg := ConfigMsg{
		Mode:     mode,
		Language: lang,
	}
	if targetLang != "" {
		cfg.TargetLang = &targetLang
	}

	if err := c.WriteJSON(cfg); err != nil {
		return nil, err
	}

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("handshake failed: %v", err)
		}

		msgStr := string(message)
		switch msgStr {
		case "loading":
			fmt.Fprintln(os.Stdout, "[Status] Server is preparing models (downloading/loading to memory). This may take a moment...")
		case "ready":
			fmt.Fprintln(os.Stdout, "[Status] Server is ready, You may now start speaking")
			return c, nil
		default:
			if len(msgStr) > 6 && msgStr[:6] == "error:" {
				return nil, fmt.Errorf("server error: %s", msgStr[6:])
			}
			return nil, fmt.Errorf("unexpected server response: %s", msgStr)
		}
	}
}

func handleReadLoop(conn Connection, timestamps bool, disconnectChan chan struct{}, out io.Writer, typer Typer) {
	lastSpeaker := -1
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Fprintln(out, "\n[Warning] Connection lost while reading.")
			select {
			case disconnectChan <- struct{}{}:
			default:
			}
			return
		}
		var msg OutputMsg
		if err := json.Unmarshal(message, &msg); err == nil {
			formatOutput(out, msg, timestamps, &lastSpeaker)
			if !timestamps {
				typer.Type(msg.Text)
			}
		}
	}
}
