package main

import (
	"fmt"
	"io"
	"log"
	"os/exec"
)

func startExternalServer(out io.Writer) *exec.Cmd {
	fmt.Fprintln(out, "Starting Rust WebSocket server...")
	cmd := exec.Command("cargo", "run", "--release")
	cmd.Dir = "../server"
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	return cmd
}
