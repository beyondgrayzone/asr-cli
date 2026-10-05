package main

import (
	"os/exec"
	"strings"
)

type CommandRunner func(name string, arg ...string) ([]byte, error)

var RealRunner CommandRunner = func(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).Output()
}

func getLinuxActiveDeviceInfo(runner CommandRunner) (string, string) {
	defaultSrcBuf, err := runner("pactl", "get-default-source")
	if err != nil {
		return "", ""
	}

	listSourcesBuf, err := runner("pactl", "list", "sources")
	if err != nil {
		return strings.TrimSpace(string(defaultSrcBuf)), ""
	}

	return parseDeviceInfo(string(defaultSrcBuf), string(listSourcesBuf))
}

func parseDeviceInfo(defaultSourceOut, listSourcesOut string) (sourceName string, portDesc string) {
	sourceName = strings.TrimSpace(defaultSourceOut)
	if sourceName == "" {
		return "", ""
	}

	lines := strings.Split(listSourcesOut, "\n")
	isTargetSource := false
	activePortID := ""
	portMap := make(map[string]string)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(line, "Source #") {
			isTargetSource = false
		}
		if strings.Contains(line, "Name: "+sourceName) {
			isTargetSource = true
		}

		if isTargetSource {
			if strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "Active Port") && !strings.Contains(trimmed, "Properties") {
				parts := strings.SplitN(trimmed, ":", 2)
				id := strings.TrimSpace(parts[0])
				desc := strings.Split(parts[1], "(")[0]
				portMap[id] = strings.TrimSpace(desc)
			}

			if strings.HasPrefix(trimmed, "Active Port:") {
				parts := strings.Split(trimmed, ":")
				if len(parts) > 1 {
					activePortID = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	return sourceName, portMap[activePortID]
}
