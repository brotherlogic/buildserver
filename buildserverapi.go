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

//Build a binary
func (s *Server) Build(ctx context.Context, req *pb.BuildRequest) (*pb.BuildResponse, error) {
	s.buildRequest++

	//Don't build blacklisted jobs
	for _, blacklist := range s.blacklist {
		if blacklist == req.GetJob().Name {
			s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
			return &pb.BuildResponse{}, fmt.Errorf("Can't build %v due to blacklist", blacklist)
		}
	}

	//Build the binary
	s.enqueue(req.GetJob())

	return &pb.BuildResponse{}, nil
}

//ReportCrash reports a crash
func (s *Server) ReportCrash(ctx context.Context, req *pb.CrashRequest) (*pb.CrashResponse, error) {
	s.crashes++
	for _, val := range s.pathMap {
		if val.Version == req.Version && val.Job.Name == req.Job.Name {
			s.RaiseIssue(ctx, "Crash for "+val.Job.Name, fmt.Sprintf("%v", req.Crash.ErrorMessage), false)
			val.Crashes = append(val.Crashes, req.Crash)
			s.scheduler.saveVersionFile(val)
			return &pb.CrashResponse{}, nil
		}
	}

	s.BounceIssue(ctx, fmt.Sprintf("Crash for %v", req.Job.Name), fmt.Sprintf("%v", req.Crash.ErrorMessage), req.Job.Name)
	return &pb.CrashResponse{}, fmt.Errorf("Version not found")
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	ctx = s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_START_FUNCTION)

	s.jobsMutex.Lock()
	s.jobs[req.GetJob().Name] = req.GetJob()
	s.jobsMutex.Unlock()

	resp := &pb.VersionResponse{}
	latest := make(map[string]*pb.Version)
	bestTime := make(map[string]int64)
	for _, v := range s.pathMap {
		if req.GetJob().Name == "" || v.Job.Name == req.GetJob().Name {
			_, ok := bestTime[v.Job.Name]
			if !ok {
				bestTime[v.Job.Name] = 0
			}
			resp.Versions = append(resp.Versions, v)
			if v.VersionDate > bestTime[v.Job.Name] {
				latest[v.Job.Name] = v
				bestTime[v.Job.Name] = v.VersionDate
			}
		}
	}

	if req.JustLatest {
		versions := []*pb.Version{}
		for _, l := range latest {
			versions = append(versions, l)
		}
		s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
		return &pb.VersionResponse{Versions: versions}, nil
	}

	s.LogTrace(ctx, "GetVersions", time.Now(), pbt.Milestone_END_FUNCTION)
	return resp, nil
}
