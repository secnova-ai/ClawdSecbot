package core

import "testing"

func TestParseProcNetTCPPorts_ReturnsListeningPorts(t *testing.T) {
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:4955 00000000:0000 0A 00000000:00000000 00:00000000 00000000   100        0 111 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 00000000:0000 01 00000000:00000000 00:00000000 00000000   100        0 112 1 0000000000000000 100 0 0 10 0
   2: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000   100        0 113 1 0000000000000000 100 0 0 10 0
`

	ports := parseProcNetTCPPorts(content)

	if !intSliceContains(ports, 18773) {
		t.Fatalf("ports = %v, want 18773", ports)
	}
	if !intSliceContains(ports, 22) {
		t.Fatalf("ports = %v, want 22", ports)
	}
	if intSliceContains(ports, 8080) {
		t.Fatalf("ports = %v, should skip non-listening port 8080", ports)
	}
}

func TestBuildProcSystemProcess_NormalizesCmdline(t *testing.T) {
	proc := buildProcSystemProcess(42, "node\n", []byte("/usr/bin/node\x00/app/openclaw/dist/index.js\x00gateway\x00"), "/usr/bin/node")

	if proc.Pid != 42 {
		t.Fatalf("Pid = %d, want 42", proc.Pid)
	}
	if proc.Name != "node" {
		t.Fatalf("Name = %q, want node", proc.Name)
	}
	if proc.Cmd != "42 node /usr/bin/node /app/openclaw/dist/index.js gateway" {
		t.Fatalf("Cmd = %q", proc.Cmd)
	}
	if proc.Path != "/usr/bin/node" {
		t.Fatalf("Path = %q, want /usr/bin/node", proc.Path)
	}
}

func intSliceContains(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
