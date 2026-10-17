// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
	"unsafe"
)

func TestChildJobTerminatesOwnedProcess(t *testing.T) {
	if unsafe.Sizeof(jobExtendedLimits{}) != 144 {
		t.Fatal("incorrect Windows x64 job ABI")
	}
	job, err := newChildJob()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()
	child := exec.Command(os.Args[0], "-test.run=^TestJobChildHarness$")
	child.Env = append(os.Environ(), "YINGXIA_LAUNCHER_TEST_CHILD=1")
	configureChild(child)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	if err := job.Assign(child.Process.Pid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(output).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "JOB_CHILD_READY\n" {
			t.Fatal("child fixture did not initialize", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child fixture did not initialize")
	}
	job.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("owned child survived closing the launcher job")
	}
}

func TestJobChildHarness(t *testing.T) {
	if os.Getenv("YINGXIA_LAUNCHER_TEST_CHILD") != "1" {
		t.Skip("explicit launcher child fixture")
	}
	fmt.Println("JOB_CHILD_READY")
	for {
		time.Sleep(time.Second)
	}
}
