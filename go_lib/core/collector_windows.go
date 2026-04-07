package core

import (
	"bufio"
	"strconv"
	"strings"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// getOpenPorts uses netstat to get open TCP listening ports on Windows
func (c *platformCollector) getOpenPorts() ([]int, error) {
	logging.Debug("Running netstat to get open ports...")
	cmd := cmdutil.Command("netstat", "-ano", "-p", "TCP")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("netstat command failed: %v", err)
		return nil, err
	}
	logging.Debug("netstat output length: %d bytes", len(output))

	portMap := make(map[int]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		// netstat -ano output: Proto  Local Address  Foreign Address  State  PID
		// e.g.: TCP    127.0.0.1:8080    0.0.0.0:0    LISTENING    1234
		if len(fields) < 4 {
			continue
		}
		if !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}

		localAddr := fields[1]
		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon < 0 || lastColon == len(localAddr)-1 {
			continue
		}
		portStr := localAddr[lastColon+1:]
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}
		portMap[port] = true
	}

	var ports []int
	for p := range portMap {
		ports = append(ports, p)
	}
	logging.Debug("Found %d ports: %v", len(ports), ports)
	return ports, nil
}

// getRunningProcesses uses tasklist to get running processes on Windows
func (c *platformCollector) getRunningProcesses() ([]SystemProcess, error) {
	logging.Debug("Running tasklist to get processes...")
	// /FO CSV /NH: CSV format, no header
	cmd := cmdutil.Command("tasklist", "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("tasklist command failed: %v", err)
		return nil, err
	}
	logging.Debug("tasklist output length: %d bytes", len(output))

	var procs []SystemProcess
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// CSV format: "name.exe","PID","Session Name","Session#","Mem Usage"
		fields := parseCSVLine(line)
		if len(fields) < 2 {
			continue
		}

		name := strings.Trim(fields[0], "\"")
		pidStr := strings.Trim(fields[1], "\"")
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err != nil {
			continue
		}

		procs = append(procs, SystemProcess{
			Pid:  int32(pid),
			Name: name,
			Cmd:  name,
			Path: name,
		})
	}
	logging.Debug("Found %d processes", len(procs))
	return procs, nil
}

// getServices uses sc query to get Windows services
func (c *platformCollector) getServices() ([]string, error) {
	logging.Debug("Running sc query to get services...")
	cmd := cmdutil.Command("sc", "query", "type=", "service", "state=", "all")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("sc query command failed: %v", err)
		return nil, err
	}
	logging.Debug("sc query output length: %d bytes", len(output))

	var services []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// SERVICE_NAME: ServiceName
		if strings.HasPrefix(line, "SERVICE_NAME:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "SERVICE_NAME:"))
			if name != "" {
				services = append(services, name)
			}
		}
	}
	logging.Debug("Found %d services", len(services))
	return services, nil
}

// parseCSVLine splits a simple CSV line (handles quoted fields)
func parseCSVLine(line string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false

	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}
