package main

import (
	"context"
	"os"
	"testing"
	"time"

	dstore_client "github.com/brotherlogic/dstore/client"
	keystoreclient "github.com/brotherlogic/keystore/client"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pb "github.com/brotherlogic/buildserver/proto"
	pbd "github.com/brotherlogic/discovery/proto"
	pbds "github.com/brotherlogic/dstore/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

func TestGetVersion(t *testing.T) {
	v := getVersion("blah")
	if v != "NO VERSION FOUND" {
		t.Errorf("Bad version pickup: %v", v)
	}
}

func InitTestServer(f string) *Server {
	return CloneTestServer(f, true)
}

func CloneTestServer(f string, delete bool) *Server {
	wd, _ := os.Getwd()
	s := Init()
	if delete {
		os.RemoveAll(wd + "/" + f)
	}
	s.GoServer.KSclient = *keystoreclient.GetTestClient("./testing")
	s.scheduler.dir = wd + "/" + f
	s.dir = wd + "/" + f
	s.lister = &prodLister{dir: wd + "/" + f}
	s.SkipLog = true
	s.SkipIssue = true
	s.Registry = &pbd.RegistryEntry{Identifier: "blah"}
	s.runBuild = true
	s.testing = true
	s.dhclient = &dstore_client.DStoreClient{Test: true}
	s.Bits = 32

	config := &pb.Config{LatestVersions: map[string]*pb.Version{"blah": &pb.Version{}}}
	data, _ := proto.Marshal(config)
	s.dhclient.Write(context.Background(), &pbds.WriteRequest{Key: CONFIG_KEY, Value: &anypb.Any{Value: data}})

	// Run background queue processing
	go func() {
		s.dequeue()
	}()

	return s
}

func TestEmptyJob(t *testing.T) {
	s := InitTestServer("testcrashreport")
	_, err := s.GetVersions(context.Background(), &pb.VersionRequest{})

	if err == nil {
		t.Errorf("Empty request should be rejected")
	}
}

func TestCrashReport(t *testing.T) {
	s := InitTestServer("testcrashreport")
	s.ReportCrash(context.Background(), &pb.CrashRequest{Job: &pbgbs.Job{Name: "testing"}, Crash: &pb.Crash{ErrorMessage: "help"}})
}

func TestCrashReportWithEmpty(t *testing.T) {
	s := InitTestServer("testcrashreport")
	_, err := s.ReportCrash(context.Background(), &pb.CrashRequest{Job: &pbgbs.Job{Name: "testing"}, Crash: &pb.Crash{ErrorMessage: ""}})
	if err == nil {
		t.Errorf("Should have failed")
	}
}

func TestCrashReportWithUpdate(t *testing.T) {
	s := InitTestServer("testcrashreport")
	s.pathMap["blah"] = &pb.Version{Version: "1234", Job: &pbgbs.Job{Name: "testing"}}
	s.ReportCrash(context.Background(), &pb.CrashRequest{Job: &pbgbs.Job{Name: "testing"}, Version: "1234", Crash: &pb.Crash{ErrorMessage: "help"}})

	if len(s.pathMap["blah"].Crashes) != 1 {
		t.Errorf("Crash was not added")
	}
}

func TestBuildWithFailure(t *testing.T) {
	s := InitTestServer("buildwithhour")

	_, err := s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "blahblahblah", GoPath: "github.com/brotherlogic/blahblahblah"}})
	s.drainAndRestoreQueue(context.Background())

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/blahblahblah"}})
	if err == nil {
		t.Fatalf("Error in get versions: %v", resp)
	}
}

func TestList(t *testing.T) {
	s := InitTestServer("testlist")
	_, err := s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Error in build: %v", err)
	}
	s.drainAndRestoreQueue(context.Background())

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Get version failed: %v", err)
	}
	if len(resp.Versions) != 1 {
		t.Errorf("Not enough versions: %v -> %v", resp, s.latest)
	}
}

func TestListSingle(t *testing.T) {
	s := InitTestServer("testlistsingle")
	_, err := s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	s.drainAndRestoreQueue(context.Background())

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}, JustLatest: true})
	if err != nil {
		t.Fatalf("Get version failed: %v", err)
	}
	if len(resp.Versions) != 1 {
		t.Errorf("Not enough versions: %v", resp)
	}
}

func TestDoubleBuild(t *testing.T) {
	s := InitTestServer("testlistsingle")
	s.scheduler.waitTime = time.Second
	_, err := s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	s.drainAndRestoreQueue(context.Background())
	_, err = s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	s.drainAndRestoreQueue(context.Background())

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}, JustLatest: true})
	if err != nil {
		t.Fatalf("Get version failed: %v", err)
	}
	if len(resp.Versions) != 1 {
		t.Errorf("Not enough versions: %v", resp)
	}
}

func TestCrash(t *testing.T) {
	s := InitTestServer("testcrash")

	_, err := s.Build(context.Background(), &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Error in build: %v", err)
	}
	s.drainAndRestoreQueue(context.Background())

	resp, err := s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Fatalf("Error getting versions: %v", err)
	}

	if len(resp.GetVersions()) != 1 || len(resp.GetVersions()[0].GetCrashes()) != 0 {
		t.Fatalf("bad pull - not version or crashes: %v", resp)
	}

	s.ReportCrash(context.Background(), &pb.CrashRequest{Job: &pbgbs.Job{Name: "crasher"}, Version: resp.GetVersions()[0].GetVersion(), Crash: &pb.Crash{ErrorMessage: "help"}})

	resp, err = s.GetVersions(context.Background(), &pb.VersionRequest{BitSize: 32, JustLatest: true, Job: &pbgbs.Job{Name: "crasher", GoPath: "github.com/brotherlogic/crasher"}})
	if err != nil {
		t.Errorf("Error getting versions: %v", err)
	}

	// Get latest should not get crashes
	if len(resp.GetVersions()) != 1 || len(resp.GetVersions()[0].GetCrashes()) != 0 {
		t.Errorf("bad pull - not version or crashes: %v", resp)
	}
}
