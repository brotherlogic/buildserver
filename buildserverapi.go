package main

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/buildserver/proto"
)

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	// Schedule a build if it's been 1 hour since the last call
	buildNeeded := false
	if val, ok := s.builds[req.GetJob().Name]; ok {
		if time.Now().Sub(val) > time.Hour {
			buildNeeded = true
		}
	} else {
		buildNeeded = true
	}
	if buildNeeded {
		go s.scheduler.build(req.GetJob())
	}
	return nil, fmt.Errorf("Not implemented")
}
