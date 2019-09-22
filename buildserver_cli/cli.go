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

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func main() {
	ctx, cancel := utils.BuildContext("buildserver-"+os.Args[1], "buildserver")
	defer cancel()

	host, port, err := utils.Resolve("buildserver", "buildserver-cli")
	if err != nil {
		log.Fatalf("Unable to reach organiser: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewBuildServiceClient(conn)

	switch os.Args[1] {
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
		_, err := client.Build(ctx, &pb.BuildRequest{Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, ForceBuild: len(os.Args) > 3})
		if err != nil {
			log.Fatalf("Error on build: %v", err)
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

			res, err := client.GetVersions(ctx, &pb.VersionRequest{Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true})
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
		res, err := client.GetVersions(ctx, &pb.VersionRequest{Job: &pbgbs.Job{Name: os.Args[2], GoPath: "github.com/brotherlogic/" + os.Args[2]}, JustLatest: true})
		if err != nil {
			log.Fatalf("Error on build: %v", err)
		}

		fmt.Printf("%v - %v (%v)\n", res.Versions[0].Version, time.Unix(res.Versions[0].VersionDate, 0), len(res.Versions[0].Crashes))
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
