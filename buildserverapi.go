package main

import (
	"fmt"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/buildserver/proto"
)

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	return nil, fmt.Errorf("Not implemented")
}
