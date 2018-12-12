package main

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func LogTest(text string) {
	log.Printf(text)
}

func load(v *pb.Version) {
	//Pass
}

func TestAppendRun(t *testing.T) {
	os.Unsetenv("GOBIN")
	os.Unsetenv("GOPATH")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}
	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}

	rc := &rCommand{command: exec.Command("ls")}
	err = s.run(rc)
	if err != nil {
		t.Fatalf("Error running command: %v", err)
	}
}

func TestRunNoCommand(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}
	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}

	rc := &rCommand{
		command: exec.Command("thisdoesnothing"),
	}

	err = s.run(rc)

	if err == nil {
		t.Errorf("bad command, no error")
	}
}

func TestRunBadCommand(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}
	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}

	rc := &rCommand{
		command: exec.Command("ls", "/madeupdirectory"),
	}

	err = s.run(rc)
	time.Sleep(time.Millisecond * 500)

	if rc.err == nil {
		t.Errorf("bad command, no error: %v", rc.output)
	}
}

func TestBuidlRun(t *testing.T) {
	os.Setenv("GOBIN", "blah")
	os.Setenv("GOPATH", "wha")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}
	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}

	hash, err := s.build(queueEntry{job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")

	f, err := os.Open(wd + "/buildtest/builds/github.com/brotherlogic/crasher/crasher-" + hash)
	if err != nil {
		t.Fatalf("Can't open file: %v", err)
	}

	_, err = f.Stat()
	if err != nil {
		t.Errorf("Failure to get file info: %v", err)
	}
}

func TestBuildRunError(t *testing.T) {
	os.Setenv("GOBIN", "blah")
	os.Setenv("GOPATH", "wha")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}
	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}
	s.lastBuild["crasher"] = time.Now()

	hash, err := s.build(queueEntry{job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")
	if err == nil {
		t.Errorf("Should have errored here: %v", hash)
	}
}

func TestEmptyJobName(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Pah, %v", err)
	}

	s := &Scheduler{
		wd + "/buildtest",
		&sync.Mutex{},
		make(map[string]*sync.Mutex),
		LogTest,
		"md5sum",
		load,
		&sync.Mutex{},
		make(map[string]time.Time),
		"",
		time.Minute * 2,
	}
	hash, err := s.build(queueEntry{job: &pbgbs.Job{GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")
	if err == nil {
		t.Errorf("Empty job name did not fail build: %v", hash)
	}
}
