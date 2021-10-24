package main

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/brotherlogic/buildserver/proto"
	"github.com/brotherlogic/goserver/utils"
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
	s.Log(fmt.Sprintf("Build request: %v", req))
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
			s.BounceIssue(fmt.Sprintf("Crash for %v", val.Job.Name), fmt.Sprintf("on %v - %v", req.Origin, req.Crash.ErrorMessage), val.Job.Name)
			val.Crashes = append(val.Crashes, req.Crash)
			s.pathMapMutex.Unlock()
			s.scheduler.saveVersionFile(val)
			return &pb.CrashResponse{}, nil
		}
	}
	s.pathMapMutex.Unlock()

	s.BounceIssue(fmt.Sprintf("Unfound crash for %v", req.Job.Name), fmt.Sprintf("At %v on %v: %v", time.Now(), req.Origin, req.Crash.ErrorMessage), req.Job.Name)
	return &pb.CrashResponse{}, status.Errorf(codes.NotFound, "Version %v/%v not found for %v (%v) -> %v", req.Job.Name, req.Version, req.Origin, req.Crash.CrashType, req.Crash.ErrorMessage)
}

//GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	if req.GetJob() == nil {
		return &pb.VersionResponse{}, fmt.Errorf("You sent an empty job for some reason")
	}

	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	s.Log(fmt.Sprintf("Loaded config: %v -> %v", config, config.GetLatestVersions()[req.GetJob().GetName()]))

	latest := config.GetLatestVersions()[req.GetJob().GetName()]
	if latest == nil || time.Now().Sub(time.Unix(latest.GetVersionDate(), 0)) > time.Hour*4 {
		go func() {
			ctx, cancel := utils.ManualContext("bsi", time.Minute*5)
			defer cancel()
			_, err := s.Build(ctx, &pb.BuildRequest{Job: req.GetJob(), Origin: "internal"})
			s.Log(fmt.Sprintf("internal build: %v", err))
		}()

		return nil, fmt.Errorf("No builds found for %v", req.GetJob().GetName())
	}
	return &pb.VersionResponse{Versions: []*pb.Version{latest}}, nil
}
