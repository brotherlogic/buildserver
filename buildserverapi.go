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

//ReportCrash reports a crash
func (s *Server) ReportCrash(ctx context.Context, req *pb.CrashRequest) (*pb.CrashResponse, error) {
	return &pb.CrashResponse{}, fmt.Errorf("Not implemented yet")
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	s.buildRequest++

	//Don't build blacklisted jobs
	for _, blacklist := range s.blacklist {
		if blacklist == req.GetJob().Name {
			return &pb.VersionResponse{}, fmt.Errorf("Can't build %v due to blacklist", blacklist)
		}
	}

	s.jobs[req.GetJob().Name] = req.GetJob()

	// Schedule a build if it's been 1 hour since the last call
	buildNeeded := false
	if val, ok := s.builds[req.GetJob().Name]; ok {
		if time.Now().Sub(val) > time.Minute*5 {
			buildNeeded = true
		}
	} else {
		buildNeeded = true
	}
	if buildNeeded {
		s.Log(fmt.Sprintf("Build needed for %v with path %v", req.GetJob().Name, req.GetJob().GoPath))
		s.enqueue(req.GetJob())
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
		if !strings.HasSuffix(f.path, ".version") {
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
	}

	if req.JustLatest && latest != nil {
		return &pb.VersionResponse{Versions: []*pb.Version{latest}}, nil
	}

	return resp, nil
}
