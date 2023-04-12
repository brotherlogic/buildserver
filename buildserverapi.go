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

// Build a binary
func (s *Server) Build(ctx context.Context, req *pb.BuildRequest) (*pb.BuildResponse, error) {
	for _, b := range []string{"home", "fleet-infra", "gramophile"} {
		if req.GetJob().GetName() == b {
			return &pb.BuildResponse{}, nil
		}
	}

	if req.GetBitSize() != int32(s.Bits) {
		return nil, status.Errorf(codes.FailedPrecondition, "Unable to build for %v bits", req.GetBitSize())
	}

	s.CtxLog(ctx, fmt.Sprintf("Build request: %v", req))
	s.buildRequest++

	//Build the binary
	s.enqueue(req.GetJob(), req.ForceBuild)

	return &pb.BuildResponse{}, nil
}

// ReportCrash reports a crash
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
			s.scheduler.saveVersionFile(ctx, val)
			return &pb.CrashResponse{}, nil
		}
	}
	s.pathMapMutex.Unlock()

	s.BounceIssue(ctx, fmt.Sprintf("Unfound crash for %v", req.Job.Name), fmt.Sprintf("At %v on %v: %v", time.Now(), req.Origin, req.Crash.ErrorMessage), req.Job.Name)
	return &pb.CrashResponse{}, status.Errorf(codes.NotFound, "Version %v/%v not found for %v (%v) -> %v", req.Job.Name, req.Version, req.Origin, req.Crash.CrashType, req.Crash.ErrorMessage)
}

// GetVersions gets the versions
func (s *Server) GetVersions(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	if req.GetJob() == nil || req.GetJob().GetGoPath() == "" {
		return &pb.VersionResponse{}, fmt.Errorf("You sent an empty job for some reason, or an empty gopath: %v", req.GetJob())
	}

	if req.GetBitSize() == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "You must supply a bit size")
	}

	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	s.CtxLog(ctx, fmt.Sprintf("Loaded config: %v -> %v", config.GetLatest64Versions()[req.GetJob().GetName()], config.GetLatestVersions()[req.GetJob().GetName()]))
	var latest *pb.Version
	if req.GetBitSize() == 32 {
		latest = config.GetLatestVersions()[req.GetJob().GetName()]
		if latest == nil {
			go func() {
				ctx, cancel := utils.ManualContext("bsi", time.Minute*5)
				defer cancel()
				_, err := s.Build(ctx, &pb.BuildRequest{Job: req.GetJob(), Origin: "internal", BitSize: 32})
				s.CtxLog(ctx, fmt.Sprintf("internal build: %v", err))
			}()

			return nil, status.Errorf(codes.FailedPrecondition, "No builds found for %v", req.GetJob().GetName())
		}
	} else {
		latest = config.GetLatest64Versions()[req.GetJob().GetName()]
		if latest == nil {
			go func() {
				ctx, cancel := utils.ManualContext("bsi", time.Minute*5)
				defer cancel()
				_, err := s.Build(ctx, &pb.BuildRequest{Job: req.GetJob(), Origin: "internal", BitSize: 64})
				s.CtxLog(ctx, fmt.Sprintf("internal build: %v", err))
			}()

			return nil, status.Errorf(codes.FailedPrecondition, "No builds found for %v", req.GetJob().GetName())
		}
	}
	return &pb.VersionResponse{Versions: []*pb.Version{latest}}, nil
}
