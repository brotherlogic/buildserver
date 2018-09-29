package main

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/brotherlogic/keystore/client"

	pb "github.com/brotherlogic/buildserver/proto"
	pbd "github.com/brotherlogic/discovery/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func TestGetVersion(t *testing.T) {
	v := getVersion("blah")
	if v != "NO VERSION FOUND" {
		t.Errorf("Bad version pickup: %v", v)
	}
}

func InitTestServer(f string) *Server {
	wd, _ := os.Getwd()
	s := Init()
	os.RemoveAll(wd + "/" + f)
	s.GoServer.KSclient = *keystoreclient.GetTestClient("./testing")
	s.scheduler.dir = wd + "/" + f
	s.dir = wd + "/" + f
	s.lister = &prodLister{dir: wd + "/" + f}
	s.SkipLog = true
	s.Registry = &pbd.RegistryEntry{Identifier: "blah"}
	return s
}

func TestBuildWithHour(t *testing.T) {
	s := InitTestServer("buildwithhour")
	s.builds["crasher"] = time.Now().AddDate(-1, 0, 0)

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})

	if err != nil {
		t.Fatalf("Error in get versions: %v", err)
	}
	time.Sleep(time.Second)
	s.scheduler.wait()

	if len(resp.Versions) != 0 {
		t.Errorf("Get versions did not fail: %v", resp)
	}

	resp, err = s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Error in get versions: %v", err)
	}

	if len(resp.Versions) != 1 {
		t.Errorf("Get versions did not fail: %v", resp)
	}

}

func TestBuildWithFailure(t *testing.T) {
	s := InitTestServer("buildwithhour")

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "blahblahblah", GoPath: "github.com/brotherlogic/blahblahblah"}})

	if err != nil {
		t.Fatalf("Error in get versions: %v", err)
	}
	time.Sleep(time.Second)
	s.scheduler.wait()

	if len(resp.Versions) != 0 {
		t.Errorf("Get versions did not fail: %v", resp)
	}

	resp, err = s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Error in get versions: %v", err)
	}

	if len(resp.Versions) != 0 {
		t.Errorf("Get versions did not fail: %v", resp)
	}

}

func TestList(t *testing.T) {
	log.Printf("TEST LIST")
	s := InitTestServer("testlist")
	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if len(resp.Versions) != 0 {
		t.Fatalf("Get versions did not fail: %v", resp)
	}
	time.Sleep(time.Second)
	s.scheduler.wait()

	resp, err = s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Get version failed: %v", err)
	}
	if len(resp.Versions) != 1 {
		t.Errorf("Not enough versions: %v", resp)
	}
}

func TestListFail(t *testing.T) {
	s := InitTestServer("testlistfail")
	s.lister = &prodLister{fail: true}
	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err == nil {
		t.Errorf("Should have failed: %v", resp)
	}
}
