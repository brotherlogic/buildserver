package main

import (
	"log"
	"strings"
	"time"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/buildserver/proto"
)

func getVersion(f string) string {
	fs := strings.Split(f, "-")
	if len(fs) == 2 {
		return fs[1]
	}
	return "NO VERSION FOUND"
}

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
	log.Printf("BUILD NEEDED: %v", buildNeeded)
	if buildNeeded {
		go s.scheduler.build(req.GetJob())
	}

	files := s.lister.listFiles(req.GetJob())

	resp := &pb.VersionResponse{}
	for _, f := range files {
		resp.Versions = append(resp.Versions,
			&pb.Version{
				Job:     req.GetJob(),
				Version: getVersion(f),
			})
	}

	return resp, nil
}
