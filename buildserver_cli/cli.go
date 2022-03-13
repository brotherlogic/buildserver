package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbgs "github.com/brotherlogic/goserver/proto"
	vtpb "github.com/brotherlogic/versiontracker/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func init() {
	resolver.Register(&utils.DiscoveryClientResolverBuilder{})
}

func main() {
	ctx, cancel := utils.ManualContext("buildserver-"+os.Args[1], time.Second*10)
	defer cancel()

	conn, err := utils.LFDialServer(ctx, "buildserver")
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewBuildServiceClient(conn)

	switch os.Args[1] {
	case "mote":
		ctx, cancel := utils.BuildContext("buildserver-"+os.Args[1], "buildserver")
		defer cancel()

		conn, err := grpc.Dial(fmt.Sprintf("%v:%v", os.Args[2], os.Args[3]), grpc.WithInsecure())
		client := pbgs.NewGoserverServiceClient(conn)
		a, err := client.Mote(ctx, &pbgs.MoteRequest{Master: false})
		fmt.Printf("%v and %v\n", a, err)

	case "spbuild":
		ctx, cancel := utils.BuildContext("buildserver-"+os.Args[1], "buildserver")
		defer cancel()

		entries, err := utils.ResolveAll("buildserver")
		if err != nil {
			log.Fatalf("Unable to reach organiser: %v", err)
		}
		for _, entry := range entries {
			if entry.Identifier == os.Args[2] {
				conn, err := grpc.Dial(entry.Ip+":"+strconv.Itoa(int(entry.Port)), grpc.WithInsecure())
				defer conn.Close()

				if err != nil {
					log.Fatalf("Unable to dial: %v", err)
				}

				client := pb.NewBuildServiceClient(conn)

				_, err = client.Build(ctx, &pb.BuildRequest{Job: &pbgbs.Job{Name: os.Args[3], GoPath: "github.com/brotherlogic/" + os.Args[3]}, ForceBuild: true})
				if err != nil {
					log.Fatalf("Error on build: %v", err)
				}
			}
		}
	case "build":
		_, err := client.Build(ctx, &pb.BuildRequest{BitSize: 32, Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, ForceBuild: len(os.Args) > 3})
		if err != nil {
			log.Printf("Error on build: %v", err)
		}
	case "build64":
		_, err := client.Build(ctx, &pb.BuildRequest{BitSize: 64, Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, ForceBuild: len(os.Args) > 3})
		if err != nil {
			log.Printf("Error on build: %v", err)
		}
	case "alllatest":
		entries, err := utils.ResolveAll("buildserver")
		if err != nil {
			log.Fatalf("Unable to reach organiser: %v", err)
		}
		for _, entry := range entries {
			ctx, cancel := utils.BuildContext("buildserver-"+os.Args[1], "buildserver")
			defer cancel()

			conn, err := grpc.Dial(entry.Ip+":"+strconv.Itoa(int(entry.Port)), grpc.WithInsecure())
			defer conn.Close()

			if err != nil {
				log.Fatalf("Unable to dial: %v", err)
			}

			client := pb.NewBuildServiceClient(conn)

			res, err := client.GetVersions(ctx, &pb.VersionRequest{Origin: "cli-alllatest", Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true})
			if err != nil {
				log.Fatalf("Error on build: %v", err)
			}
			if len(res.Versions) > 0 {
				fmt.Printf("(%v) %v - %v (%v)\n", entry.Identifier, res.Versions[0].Version, time.Unix(res.Versions[0].VersionDate, 0), len(res.Versions[0].Crashes))
			} else {
				fmt.Printf("(%v) no versions\n", entry.Identifier)
			}
		}
	case "latest":
		var err error
		ctx, cancel := utils.ManualContext("buildserver-"+os.Args[1], time.Second*10)
		defer cancel()

		req := &pb.VersionRequest{Origin: "cli-latest", Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true, BitSize: 32}
		res, err := client.GetVersions(ctx, req)
		if err == nil {
			fmt.Printf("32: %v - %v (%v) %v\n", res.Versions[0].Version, time.Unix(res.Versions[0].VersionDate, 0), len(res.Versions[0].Crashes),
				res.Versions[0].Server)
		}

		if err != nil {
			log.Fatalf("Error on build: %v", err)
		}

		req = &pb.VersionRequest{Origin: "cli-latest", Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true, BitSize: 64}
		res, err = client.GetVersions(ctx, req)
		if err == nil {
			fmt.Printf("64: %v - %v (%v) %v\n", res.Versions[0].Version, time.Unix(res.Versions[0].VersionDate, 0), len(res.Versions[0].Crashes),
				res.Versions[0].Server)
		}

		if err != nil {
			log.Fatalf("Error on build: %v", err)
		}
	case "trigger":
		var err error
		ctx, cancel := utils.ManualContext("buildserver-"+os.Args[1], time.Second*10)
		defer cancel()

		req := &pb.VersionRequest{Origin: "cli-latest", Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true}
		res, err := client.GetVersions(ctx, req)
		if err == nil {
			fmt.Printf("%v - %v (%v)\n", res.Versions[0].Version, time.Unix(res.Versions[0].VersionDate, 0), len(res.Versions[0].Crashes))
		}

		if err != nil {
			log.Fatalf("Error on build: %v", err)
		}

		conn, err := utils.LFDialSpecificServer(ctx, "versiontracker", "dev")
		if err != nil {
			log.Fatalf("Booo: %v", err)
		}
		vtclient := vtpb.NewVersionTrackerServiceClient(conn)
		a, err := vtclient.NewJob(ctx, &vtpb.NewJobRequest{Version: res.Versions[0]})
		log.Printf("Triggered: %v, %v", a, err)

	case "crash":
		file, err := ioutil.ReadFile(os.Args[4])
		if err != nil {
			log.Fatalf("Error reading file: %v", err)
		}

		origin := getLocalIP()
		_, err = client.ReportCrash(ctx, &pb.CrashRequest{Origin: origin, Job: &pbgbs.Job{Name: os.Args[2]}, Version: os.Args[3], Crash: &pb.Crash{ErrorMessage: string(file)}})
		if err != nil {
			log.Fatalf("Error reporting: %v", err)
		}
	}
}

func getLocalIP() string {
	ifaces, _ := net.Interfaces()

	var ip net.IP
	for _, i := range ifaces {
		addrs, _ := i.Addrs()

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ip = ipnet.IP
				}
			}
		}
	}

	return ip.String()
}
