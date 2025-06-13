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
	"golang.org/x/net/context"
)

func LogTest(ctx context.Context, text string) {
	log.Printf("%v", text)
}

func load(ctx context.Context, v *pb.Version) {
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
	}

	hash, _, err := s.build(context.Background(), queueEntry{job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")
	log.Printf("%v and %v", hash, err)

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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
	}
	s.lastBuild["crasher"] = time.Now()

	hash, _, err := s.build(context.Background(), queueEntry{job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
	}
	hash, _, err := s.build(context.Background(), queueEntry{job: &pbgbs.Job{GoPath: "github.com/brotherlogic/crasher"}}, "madeup", "blah")
	if err == nil {
		t.Errorf("Empty job name did not fail build: %v", hash)
	}
}

func TestSaveVersion(t *testing.T) {
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
		int64(0),
		int64(0),
		int64(0),
		int32(0),
	}

	v := s.saveVersionInfo(context.Background(), nil, "madeuppath", "blah", "blah")
	if v != nil {
		t.Errorf("Did not fail: %v", v)
	}
}
