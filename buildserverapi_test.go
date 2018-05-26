package main

import (
	"context"
	"testing"
	"time"

	"github.com/brotherlogic/keystore/client"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func InitTestServer() *Server {
	s := Init()
	s.GoServer.KSclient = *keystoreclient.GetTestClient("./testing")
	s.scheduler.dir = ""
	return s
}

func TestBuildWithHour(t *testing.T) {
	s := InitTestServer()
	s.builds["madeup"] = time.Now().AddDate(-1, 0, 0)

	_, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "madeup", GoPath: "github.com/brotherlogic/crasher"}})

	if err == nil {
		t.Errorf("Get versions did not fail")
	}
}

func TestPass(t *testing.T) {
	s := InitTestServer()

	_, err := s.GetVersions(context.Background(), &pb.VersionRequest{Job: &pbgbs.Job{Name: "madeup", GoPath: "github.com/brotherlogic/crasher"}})

	if err == nil {
		t.Errorf("Get Versions did not fail - add some proper tests!")
	}
}
