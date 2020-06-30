package main

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

	//Build the binary
	s.enqueue(req.GetJob(), req.ForceBuild)

	return &pb.BuildResponse{}, nil
}

//ReportCrash reports a crash
func (s *Server) ReportCrash(ctx context.Context, req *pb.CrashRequest) (*pb.CrashResponse, error) {
	if len(req.Crash.ErrorMessage) == 0 {
		return nil, fmt.Errorf("Cannot submit an empty crash report")
	}
	s.crashes++
	s.pathMapMutex.Lock()
	for _, val := range s.pathMap {
		if val.Version == req.Version && val.Job.Name == req.Job.Name {
			s.BounceIssue(ctx, fmt.Sprintf("Crash for %v", val.Job.Name), fmt.Sprintf("on %v - %v", req.Origin, req.Crash.ErrorMessage), val.Job.Name)
			val.Crashes = append(val.Crashes, req.Crash)
			s.pathMapMutex.Unlock()
			s.scheduler.saveVersionFile(val)
			return &pb.CrashResponse{}, nil
		}
	}
	s.pathMapMutex.Unlock()

	s.BounceIssue(ctx, fmt.Sprintf("Unfound crash for %v", req.Job.Name), fmt.Sprintf("At %v on %v: %v", time.Now(), req.Origin, req.Crash.ErrorMessage), req.Job.Name)
	return &pb.CrashResponse{}, status.Errorf(codes.NotFound, "Version %v/%v not found for %v (%v) -> %v", req.Job.Name, req.Version, req.Origin, req.Crash.CrashType, req.Crash.ErrorMessage)
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	s.Log(fmt.Sprintf("GetVersions: %v", req))
	if req.GetJob() == nil {
		return &pb.VersionResponse{}, fmt.Errorf("You sent an empty job for some reason")
	}

	resp := &pb.VersionResponse{}
	resp.Versions = append(resp.Versions, s.latest[req.GetJob().GetName()])

	if req.JustLatest {
		return &pb.VersionResponse{Versions: []*pb.Version{s.latest[req.GetJob().GetName()]}}, nil
	}

	return resp, nil
}
