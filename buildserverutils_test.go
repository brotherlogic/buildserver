package main

import (
	"context"
	"testing"
	"time"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func TestLoadVersion(t *testing.T) {
	s := InitTestServer("testloadversion")
	s.builds["crasher"] = time.Now().AddDate(-1, 0, 0)

	_, err := s.Build(context.Background(), &pb.BuildRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})

	if err != nil {
		t.Fatalf("Error in get versions: %v", err)
	}
	s.drainQueue(context.Background())

	s2 := CloneTestServer("testloadversion", false)
	if len(s2.pathMap) != 0 {
		t.Fatalf("Error in init path map: %v", s2.pathMap)
	}
	s2.preloadInfo()
	if len(s2.pathMap) == 1 {
		t.Errorf("Error in loaded path map: %v", s2.pathMap)
	}

	//Second preload to trigger job list
	s2.preloadInfo()
	if len(s2.pathMap) == 1 {
		t.Errorf("Error in loaded path map: %v", s2.pathMap)
	}

}
