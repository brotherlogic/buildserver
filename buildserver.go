package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/keystore/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbg "github.com/brotherlogic/goserver/proto"
)

//Server main server type
type Server struct {
	*goserver.GoServer
	scheduler *Scheduler
	builds    map[string]time.Time
	dir       string
	lister    lister
	jobs      map[string]*pbgbs.Job
}

type lister interface {
	listFiles(job *pbgbs.Job) ([]string, error)
}

type prodLister struct {
	dir string
}

func (s *Server) backgroundBuilder(ctx context.Context) {
	for _, j := range s.jobs {
		go s.scheduler.build(j)
	}
}

func (p *prodLister) listFiles(job *pbgbs.Job) ([]string, error) {
	vals := make([]string, 0)
	files, err := ioutil.ReadDir(p.dir + "/builds/" + job.GoPath + "/")
	if err != nil {
		return vals, err
	}

	for _, f := range files {
		vals = append(vals, f.Name())
	}

	return vals, nil
}

// Init builds the server
func Init() *Server {
	s := &Server{
		&goserver.GoServer{},
		&Scheduler{
			dir:         "/media/scratch/buildserver",
			masterMutex: &sync.Mutex{},
			mMap:        make(map[string]*sync.Mutex),
		},
		make(map[string]time.Time),
		"/media/scratch/buildserver",
		&prodLister{dir: "/media/scratch/buildserver"},
		make(map[string]*pbgbs.Job),
	}

	return s
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {
	pb.RegisterBuildServiceServer(server, s)
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

// Mote promotes/demotes this server
func (s *Server) Mote(master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	return []*pbg.State{}
}

func main() {
	var quiet = flag.Bool("quiet", false, "Show all output")
	flag.Parse()

	//Turn off logging
	if *quiet {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	server := Init()
	server.GoServer.KSclient = *keystoreclient.GetClient(server.GetIP)
	server.PrepServer()
	server.Register = server

	server.RegisterServer("buildserver", false)

	server.RegisterRepeatingTask(server.backgroundBuilder, time.Hour)

	fmt.Printf("%v\n", server.Serve())
}
