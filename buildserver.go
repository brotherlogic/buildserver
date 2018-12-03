package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/keystore/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/brotherlogic/buildserver/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbg "github.com/brotherlogic/goserver/proto"
)

type queueEntry struct {
	job       *pbgbs.Job
	timeIn    time.Time
	fullBuild bool
}

//Server main server type
type Server struct {
	*goserver.GoServer
	scheduler         *Scheduler
	builds            map[string]time.Time
	dir               string
	lister            lister
	jobs              map[string]*pbgbs.Job
	jobsMutex         *sync.Mutex
	buildRequest      int
	runBuild          bool
	currentBuilds     int
	currentBuildMutex *sync.Mutex
	buildQueue        []queueEntry
	blacklist         []string
	pathMap           map[string]*pb.Version
	pathMapMutex      *sync.Mutex
	crashes           int64
	maxBuilds         int
}

type fileDetails struct {
	path string
	date int64
}

type lister interface {
	listFiles(job *pbgbs.Job) ([]fileDetails, error)
}

type prodLister struct {
	dir  string
	fail bool
}

func (s *Server) enqueue(job *pbgbs.Job) {
	//Only enqueue if the job isn't already there
	found := false
	for _, j := range s.buildQueue {
		if j.job.Name == job.Name {
			found = true
		}
	}

	if !found {
		s.builds[job.Name] = time.Now()
		s.buildQueue = append(s.buildQueue, queueEntry{job: job, timeIn: time.Now()})
	}
}

func (s *Server) dequeue(ctx context.Context) {
	if len(s.buildQueue) > 0 && s.currentBuilds < s.maxBuilds {
		if s.runBuild {
			go func() {
				job := s.buildQueue[0]
				s.currentBuilds++
				s.buildQueue = s.buildQueue[1:]
				if time.Now().Sub(job.timeIn) > time.Minute*10 {
					s.RaiseIssue(ctx, "Long Build", fmt.Sprintf("%v took %v to get to the front of the queue", job.job.Name, time.Now().Sub(job.timeIn)), false)
				}
				_, err := s.scheduler.build(job, s.Registry.Identifier)
				if err != nil {
					e, ok := status.FromError(err)
					if !ok || e.Code() != codes.AlreadyExists {
						s.RaiseIssue(ctx, "Build Failure", fmt.Sprintf("Build failed for %v: %v", job.job.Name, err), false)
					}
				}
				s.currentBuilds--
			}()
		}
	}
}

func (s *Server) drainQueue(ctx context.Context) {
	for len(s.buildQueue) > 0 || s.currentBuilds > 0 {
		s.dequeue(ctx)
		time.Sleep(time.Second)
	}
}

func (s *Server) load(v *pb.Version) {
	s.pathMapMutex.Lock()
	s.pathMap[v.Path] = v
	s.pathMapMutex.Unlock()
}

func (s *Server) backgroundBuilder(ctx context.Context) {
	for _, j := range s.jobs {
		s.enqueue(j)
	}
}

func (p *prodLister) listFiles(job *pbgbs.Job) ([]fileDetails, error) {
	vals := make([]fileDetails, 0)
	if p.fail {
		return vals, fmt.Errorf("Built to fail")
	}

	// If there's no such directory, return no versions
	directory := p.dir + "/builds/" + job.GoPath + "/"
	_, err := os.Stat(directory)
	if err != nil && os.IsNotExist(err) {
		return vals, nil
	}

	files, err := ioutil.ReadDir(p.dir + "/builds/" + job.GoPath + "/")
	if err != nil {
		return vals, err
	}

	for _, f := range files {
		fpath := p.dir + "/builds/" + job.GoPath + "/" + f.Name()
		info, _ := os.Stat(fpath)
		vals = append(vals, fileDetails{
			path: fpath,
			date: info.ModTime().Unix(),
		})
	}

	return vals, nil
}

func (s *Server) log(st string) {
	s.Log(st)
}

// Init builds the server
func Init() *Server {
	s := &Server{
		&goserver.GoServer{},
		&Scheduler{
			"/media/scratch/buildserver",
			&sync.Mutex{},
			make(map[string]*sync.Mutex),
			nil,
			"md5sum",
			nil,
			make(map[string]time.Time),
			"",
		},
		make(map[string]time.Time),
		"/media/scratch/buildserver",
		&prodLister{dir: "/media/scratch/buildserver"},
		make(map[string]*pbgbs.Job),
		&sync.Mutex{},
		0,
		true,
		0,
		&sync.Mutex{},
		[]queueEntry{},
		[]string{"led"},
		make(map[string]*pb.Version),
		&sync.Mutex{},
		int64(0),
		2,
	}

	s.scheduler.log = s.log
	s.scheduler.load = s.load

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
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	s.pathMapMutex.Lock()
	defer s.pathMapMutex.Unlock()
	return []*pbg.State{
		&pbg.State{Key: "enabled", Text: fmt.Sprintf("%v", s.runBuild)},
		&pbg.State{Key: "buildc", Value: int64(s.buildRequest)},
		&pbg.State{Key: "concurrent_builds", Value: int64(s.currentBuilds)},
		&pbg.State{Key: "build_queue_length", Value: int64(len(s.buildQueue))},
		&pbg.State{Key: "crashes", Value: s.crashes},
		&pbg.State{Key: "paths_read", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "current_build", Text: s.scheduler.cbuild},
	}
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

	server.RegisterRepeatingTask(server.backgroundBuilder, "background_builder", time.Minute*5)
	server.RegisterRepeatingTask(server.dequeue, "dequeue", time.Second)

	server.preloadInfo()

	fmt.Printf("%v\n", server.Serve())
}
