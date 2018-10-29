package main

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/buildserver/proto"
	pbt "github.com/brotherlogic/tracer/proto"
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
	s.crashes++
	for _, val := range s.pathMap {
		if val.Version == req.Version && val.Job.Name == req.Job.Name {
			s.RaiseIssue(ctx, "Crash for "+val.Job.Name, req.Crash.ErrorMessage, false)
			val.Crashes = append(val.Crashes, req.Crash)
			s.scheduler.saveVersionFile(val)
			return &pb.CrashResponse{}, nil
		}
	}

	s.RaiseIssue(ctx, fmt.Sprintf("Could not find version for %v", req.Job.Name), req.Crash.ErrorMessage, false)
	return &pb.CrashResponse{}, fmt.Errorf("Version not found")
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	ctx = s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_START_FUNCTION)
	s.buildRequest++

	//Don't build blacklisted jobs
	for _, blacklist := range s.blacklist {
		if blacklist == req.GetJob().Name {
			s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
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

	resp := &pb.VersionResponse{}
	var latest *pb.Version
	bestTime := int64(0)
	for _, v := range s.pathMap {
		if v.Job.Name == req.GetJob().Name {
			resp.Versions = append(resp.Versions, v)
			if v.VersionDate > bestTime {
				latest = v
				bestTime = v.VersionDate
			}
		}
	}

	if req.JustLatest && latest != nil {
		s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
		return &pb.VersionResponse{Versions: []*pb.Version{latest}}, nil
	}

	s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
	return resp, nil
}
