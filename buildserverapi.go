package main

import (
	"fmt"
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
	s.buildRequest++
	s.jobs[req.GetJob().Name] = req.GetJob()

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
		s.Log(fmt.Sprintf("Build needed for %v with path %v", req.GetJob().Name, req.GetJob().GoPath))
		go func(ictx context.Context) {
			if s.runBuild {
				_, err := s.scheduler.build(req.GetJob())
				if err != nil {
					s.RaiseIssue(ictx, "Build Failure", fmt.Sprintf("Build failed for %v: %v", req.GetJob().Name, err), false)
				}
			}
		}(ctx)
		s.builds[req.GetJob().Name] = time.Now()
	}

	files, err := s.lister.listFiles(req.GetJob())
	if err != nil {
		return &pb.VersionResponse{}, err
	}

	resp := &pb.VersionResponse{}
	var latest *pb.Version
	bestTime := int64(0)
	for _, f := range files {
		ver := &pb.Version{
			Job:         req.GetJob(),
			Version:     getVersion(f.path),
			Path:        f.path,
			Server:      s.Registry.Identifier,
			VersionDate: f.date,
		}
		resp.Versions = append(resp.Versions, ver)
		if ver.VersionDate > bestTime {
			latest = ver
			bestTime = ver.VersionDate
		}
	}

	if req.JustLatest && latest != nil {
		return &pb.VersionResponse{Versions: []*pb.Version{latest}}, nil
	}

	return resp, nil
}
