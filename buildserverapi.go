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

//Build a binary
func (s *Server) Build(ctx context.Context, req *pb.BuildRequest) (*pb.BuildResponse, error) {
	s.buildRequest++

	//Don't build blacklisted jobs
	s.blacklistMutex.Lock()
	defer s.blacklistMutex.Unlock()
	for _, blacklist := range s.nobuild {
		if blacklist == req.GetJob().Name {
			return &pb.BuildResponse{}, fmt.Errorf("Can't build %v due to blacklist", blacklist)
		}
	}

	//Build the binary
	s.enqueue(req.GetJob(), req.ForceBuild)

	return &pb.BuildResponse{}, nil
}

//ReportCrash reports a crash
func (s *Server) ReportCrash(ctx context.Context, req *pb.CrashRequest) (*pb.CrashResponse, error) {
	s.crashes++
	s.pathMapMutex.Lock()
	for _, val := range s.pathMap {
		if val.Version == req.Version && val.Job.Name == req.Job.Name {
			if req.Crash.CrashType != pb.Crash_MEMORY {
				s.RaiseIssue(ctx, fmt.Sprintf("Crash for %v", val.Job.Name), fmt.Sprintf("on %v - %v", req.Origin, req.Crash.ErrorMessage), false)
			}
			val.Crashes = append(val.Crashes, req.Crash)
			s.pathMapMutex.Unlock()
			s.scheduler.saveVersionFile(val)
			return &pb.CrashResponse{}, nil
		}
	}
	s.pathMapMutex.Unlock()

	if req.Crash.CrashType != pb.Crash_MEMORY {
		s.BounceIssue(ctx, fmt.Sprintf("Crash for %v", req.Job.Name), fmt.Sprintf("On %v: %v", req.Origin, req.Crash.ErrorMessage), req.Job.Name)
	}
	return &pb.CrashResponse{}, fmt.Errorf("Version %v not found for %v (%v)", req.Version, req.Origin, req.Crash.CrashType)
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	if req.GetJob() == nil {
		return &pb.VersionResponse{}, fmt.Errorf("You sent an empty job for some reason")
	}

	s.blacklistMutex.Lock()
	if s.blacklist[req.GetJob().Name] {
		s.enqueue(req.GetJob(), true)
		s.blacklistMutex.Unlock()
		return &pb.VersionResponse{}, fmt.Errorf("Job is blacklisted")
	}
	s.blacklistMutex.Unlock()

	resp := &pb.VersionResponse{}
	latest := make(map[string]*pb.Version)
	bestTime := make(map[string]int64)
	t := time.Now()
	s.pathMapMutex.Lock()
	d := time.Now().Sub(t)
	if d > s.lockTime {
		s.lockTime = d
	}
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
	s.pathMapMutex.Unlock()

	// Kick off an async build if we no versions
	if len(latest) == 0 {
		go s.Build(ctx, &pb.BuildRequest{Job: req.GetJob()})
	}

	if req.JustLatest {
		versions := []*pb.Version{}
		for _, l := range latest {
			versions = append(versions, l)
		}
		return &pb.VersionResponse{Versions: versions}, nil
	}

	return resp, nil
}
