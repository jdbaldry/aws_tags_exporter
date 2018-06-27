package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var (
	binary, _ = filepath.Abs("_output/bin/aws_tags_exporter.go")
	C         = &myCmd{cmd: exec.Command(binary, "--web.port", address, "--aws.region", region)}
)

const (
	address = "60099"
	region  = "eu-west-1"
)

type myCmd struct {
	cmd *exec.Cmd
}

func TestHandlingOfMetrics(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("aws_tags_exporter binary not available, try to run `make build` first: %s", err)
	}

	test := func(_ int) error {
		return queryExporter(address)
	}

	if err := runCommandAndTests(address, test); err != nil {
		t.Error(err)
	}
	killCmd()
}

func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/metrics", address))
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("%s", b)
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func runCommandAndTests(address string, fn func(pid int) error) error {
	if err := C.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 10; i++ {
		if err := queryExporter(address); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if C.cmd.Process == nil || i == 9 {
			return fmt.Errorf("can't start command")
		}
	}

	errc := make(chan error)
	go func(pid int) {
		errc <- fn(pid)
	}(C.cmd.Process.Pid)

	err := <-errc
	return err
}

func killCmd() {
	if C.cmd.Process != nil {
		fmt.Println("Killing proc")
		C.cmd.Process.Kill()
	}
}
