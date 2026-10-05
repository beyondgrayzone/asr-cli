package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/gorilla/websocket"
)

func runCaptureLoop(url string, mode string, lang string, targetLang string, timestamps bool, audioChan chan []byte, connector ServerConnector, typer Typer, out io.Writer) {
	disconnectChan := make(chan struct{}, 1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var conn Connection
	fmt.Fprintln(out, "Listening to microphone... Press Ctrl+C to stop.")
	for {
		if conn == nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Fprintf(out, "\n[Status] Connecting to server at %s...\n", url)
			// Long timeout: the first connection triggers a multi-GB model download,
			// so the handshake itself can take many minutes.
			dialCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)

			newConn, err := connector.Connect(dialCtx, url, mode, lang, targetLang, out)
			// Connect has returned, so the handshake deadline has served its
			// purpose: release the timer (not `defer`, this is inside a loop).
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				fmt.Fprintf(out, "[Warning] Connection failed: %v. Retrying in 2s...\n", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}
			conn = newConn
			fmt.Fprintf(out, "[Status] Connected successfully in %s mode.\n", mode)
			go handleReadLoop(conn, timestamps, disconnectChan, out, typer)
		}

		select {
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}
			return
		case <-disconnectChan:
			if conn != nil {
				conn.Close()
				conn = nil
			}
		case audioData := <-audioChan:
			if conn != nil {
				if err := conn.WriteMessage(websocket.BinaryMessage, audioData); err != nil {
					fmt.Fprintln(out, "\n[Warning] Failed to send audio. Reconnecting...")
					conn.Close()
					conn = nil
				}
			}
		}
	}
}

func main() {
	printMics := flag.Bool("print-mics", false, "Print available microphones and exit")
	start := flag.Bool("start", false, "Start capturing and streaming")
	startServer := flag.Bool("start-server", false, "Start the rust websocket server automatically")
	timestamps := flag.Bool("timestamps", false, "Show word level timestamps")
	shouldType := flag.Bool("type", false, "Type transcribed text at cursor position")
	stdout := flag.Bool("stdout", false, "Print terminal output")
	host := flag.String("host", "localhost:9393", "Websocket server address")
	mode := flag.String("mode", "asr", "Engine mode: asr (Nemotron) or multitalker")
	lang := flag.String("lang", "en", "Model weights: en (English Specialist) or all (Multilingual 3.5)")
	targetLang := flag.String("target-lang", "", "Target language code for 'all' mode (e.g. tr-TR, ja-JP)")
	source := flag.String("source", "mic", "Input source: 'mic' (hardware) or 'speaker' (stdin pipe)")
	flag.Parse()

	var out io.Writer = io.Discard
	if *stdout {
		out = os.Stdout
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	if *printMics {
		printMicrophones(ctx, out)
		return
	}

	if *startServer {
		cmd := startExternalServer(out)
		defer func() {
			fmt.Fprintln(out, "\nShutting down Rust server...")
			_ = cmd.Process.Kill()
		}()
	}

	if *start {
		audioChan := make(chan []byte, 100)
		var typer Typer = &NullTyper{}
		if *shouldType {
			typer = &RobotTyper{}
		}

		if *source == "speaker" {
			fmt.Fprintln(out, "[Status] Reading audio from stdin...")
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := os.Stdin.Read(buf)
					if n > 0 {
						chunk := make([]byte, n)
						copy(chunk, buf[:n])
						audioChan <- chunk
					}
					if err != nil {
						if err != io.EOF {
							fmt.Fprintf(os.Stderr, "[Error] Stdin read error: %v\n", err)
						}
						break
					}
				}
			}()
		} else {
			device, err := initAudioDevice(ctx, audioChan)
			if err != nil {
				log.Fatal(err)
			}
			defer device.Uninit()

			if err := device.Start(); err != nil {
				log.Fatal(err)
			}
			defer device.Stop()
		}

		runCaptureLoop(fmt.Sprintf("ws://%s", *host), *mode, *lang, *targetLang, *timestamps, audioChan, &WSConnector{}, typer, out)
	}
}
