package main

import (
	"context"
	"testing"

	"github.com/brotherlogic/keystore/client"

	pb "github.com/brotherlogic/buildserver/proto"
)

func InitTestServer() *Server {
	s := Init()
	s.GoServer.KSclient = *keystoreclient.GetTestClient("./testing")
	return s
}

func TestPass(t *testing.T) {
	s := InitTestServer()

	_, err := s.GetVersions(context.Background(), &pb.VersionRequest{})

	if err == nil {
		t.Errorf("Get Versions did not fail - add some proper tests!")
	}
}
