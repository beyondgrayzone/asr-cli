package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/gen2brain/malgo"
)

func printMicrophones(ctx AudioSystem, out io.Writer) {
	devices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		log.Fatal(err)
	}
	sysSource, sysPort := getLinuxActiveDeviceInfo(RealRunner)
	fmt.Fprintln(out, "Available Microphones:")
	for _, dev := range devices {
		isActive := false
		decodedID, _ := hex.DecodeString(dev.ID.String())
		if string(decodedID) == sysSource {
			isActive = true
		}
		if !isActive && dev.IsDefault > 0 && !strings.Contains(strings.ToLower(dev.Name()), "monitor") {
			isActive = true
		}
		status := ""
		if isActive {
			if sysPort != "" {
				status = fmt.Sprintf(" [ACTIVE: %s]", sysPort)
			} else {
				status = " [ACTIVE/DEFAULT]"
			}
		}
		fmt.Fprintf(out, "- %s%s\n  ID: %v\n", dev.Name(), status, dev.ID.String())
	}
}

func initAudioDevice(ctx *malgo.AllocatedContext, audioChan chan []byte) (*malgo.Device, error) {
	onRecvFrames := func(pOutputSample, pInputSamples []byte, framecount uint32) {
		buf := make([]byte, len(pInputSamples))
		copy(buf, pInputSamples)
		select {
		case audioChan <- buf:
		default:
		}
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatF32
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = 16000
	deviceConfig.Alsa.NoMMap = 1

	return malgo.InitDevice(ctx.Context, deviceConfig, malgo.DeviceCallbacks{Data: onRecvFrames})
}
