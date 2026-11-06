package ports

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// listenEnv makes the test binary, started again, a process that listens
// on a port until it is ended.
const listenEnv = "OMT_TEST_LISTEN"

func TestMain(m *testing.M) {
	if os.Getenv(listenEnv) != "" {
		listen()
		return
	}
	os.Exit(m.Run())
}

func listen() {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString(strconv.Itoa(l.Addr().(*net.TCPAddr).Port) + "\n")
	time.Sleep(time.Minute)
}

// TestRiskMatchesBehaviour starts a process that listens on a port: the
// plan, declared read, finds it and leaves it running; the apply, declared
// process, ends it.
func TestRiskMatchesBehaviour(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), listenEnv+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	port, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	in := tool.Input{fieldPort: port[:len(port)-1]}
	p, err := New().Plan(context.Background(), in)
	if err != nil || p.ApplyLabel == "" {
		t.Skipf("the system did not list the test's own port (%v); nothing to end", err)
	}
	if cmd.ProcessState != nil {
		t.Fatal("the plan ended the process")
	}
	if _, err := New().Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("the apply did not end the process")
	}
	for _, s := range New().Specs() {
		if s.Name == tool.ActionApply && s.Risk != tool.RiskProcess {
			t.Errorf("apply risk %q, but it ends processes", s.Risk)
		}
	}
}
