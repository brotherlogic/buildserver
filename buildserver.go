package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	dstore_client "github.com/brotherlogic/dstore/client"

	pb "github.com/brotherlogic/buildserver/proto"
	dspb "github.com/brotherlogic/dstore/proto"
	pbfc "github.com/brotherlogic/filecopier/proto"
	pbgbs "github.com/brotherlogic/gobuildslave/proto"
	pbg "github.com/brotherlogic/goserver/proto"
	pbvt "github.com/brotherlogic/versiontracker/proto"
)

var (
	// queue size
	queueSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "buildserver_queue",
		Help: "The size of the print queue",
	}, []string{"type"})

	builds = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "buildserver_builds",
		Help: "The number of builds made",
	}, []string{"job", "bits"})

	storedBuilds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "buildserver_storedbuilds",
		Help: "The number of builds stored",
	})
	buildStorage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "buildserver_buildstorage",
		Help: "The number of builds made",
	})
	buildTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "buildserver_build_time",
		Help: "The number of builds made",
	}, []string{"job", "bits"})
)

const (
	CONFIG_KEY = "github.com/brotherlogic/buildserver/config"
)

type queueEntry struct {
	job              *pbgbs.Job
	timeIn           time.Time
	fullBuild        bool
	queueSizeAtEntry int
	buildTime        time.Duration
	pastGithubHash   string
}

// Server main server type
type Server struct {
	*goserver.GoServer
	dhclient          *dstore_client.DStoreClient
	scheduler         *Scheduler
	builds            map[string]time.Time
	buildsMutex       *sync.Mutex
	dir               string
	lister            lister
	jobs              []*pbgbs.Job
	buildRequest      int
	runBuild          bool
	currentBuilds     int
	currentBuildMutex *sync.Mutex
	buildQueue        []queueEntry
	nobuild           []string
	pathMap           map[string]*pb.Version
	pathMapMutex      *sync.Mutex
	crashes           int64
	maxBuilds         int
	buildFails        map[string]int
	buildFailsMutex   *sync.Mutex
	latestHash        map[string]string
	latestBuild       map[string]int64
	latestDate        map[string]time.Time
	latestVersion     map[string]string
	latest            map[string]*pb.Version
	lockTime          time.Duration
	checkError        string
	enqueues          int64
	queue             chan *queueEntry
	done              chan bool
	fanoutQueue       chan fanoutEntry
	testing           bool
	token             string
}

type fanoutEntry struct {
	version *pb.Version
	server  string
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

func (s *Server) validateBuilds(ctx context.Context) error {
	s.pathMapMutex.Lock()
	defer s.pathMapMutex.Unlock()
	for _, versionString := range s.latestVersion {
		for _, version := range s.pathMap {
			if version.Version == versionString && time.Now().Sub(time.Unix(version.LastBuildTime, 0)) > time.Hour*24 {
				s.enqueue(version.Job, true)
			}

		}
	}
	return nil
}

func (s *Server) runCheck(ctx context.Context) error {
	entries, err := utils.ResolveAll("buildserver")
	jobs := make(map[string]*pbgbs.Job)
	s.pathMapMutex.Lock()
	for _, v := range s.pathMap {
		if _, ok := jobs[v.Job.Name]; !ok {
			jobs[v.Job.Name] = v.Job
		}
	}
	s.pathMapMutex.Unlock()

	if err == nil {
		for _, entry := range entries {
			if entry.Identifier != s.Registry.Identifier {
				conn, err := s.DoDial(entry)
				if err != nil {
					return err
				}
				defer conn.Close()

				client := pb.NewBuildServiceClient(conn)
				for _, job := range jobs {
					latest, err := client.GetVersions(ctx, &pb.VersionRequest{Job: job, JustLatest: true})
					if err == nil {

						if len(latest.GetVersions()) > 0 && latest.GetVersions()[0].VersionDate > s.latestBuild[job.Name] && latest.GetVersions()[0].Version != s.latestVersion[job.Name] {

							// Ensure blacklisted jobs get built
							s.enqueue(job, true)
						}
					}
				}
			}
		}
	} else {
		return err
	}

	return nil
}

func (s *Server) enqueue(job *pbgbs.Job, force bool) {
	s.enqueues++
	//Only enqueue if the job isn't already there
	found := false
	for _, j := range s.buildQueue {
		if j.job.GetName() == job.GetName() {
			found = true
		}
	}

	if !found {
		s.buildsMutex.Lock()
		s.builds[job.Name] = time.Now()
		s.buildsMutex.Unlock()

		forceBuild := force
		if val, ok := s.latestDate[job.Name]; ok {
			if time.Now().Sub(val) > time.Hour*24 {
				forceBuild = true
			}
		}

		//Reject a full build if there's one in the queue
		for _, entry := range s.buildQueue {
			forceBuild = forceBuild && !entry.fullBuild
		}

		s.queue <- &queueEntry{job: job, timeIn: time.Now(), queueSizeAtEntry: len(s.buildQueue), fullBuild: forceBuild}
	}

	queueSize.With(prometheus.Labels{"type": "reported"}).Set(float64(len(s.queue)))
}

func (s *Server) build(ctx context.Context, job *queueEntry) (*pb.Version, error) {
	t := time.Now()
	defer func() {
		buildTime.With(prometheus.Labels{"job": job.job.GetName(), "bits": fmt.Sprintf("%v", s.Bits)}).Set(time.Since(t).Seconds())
	}()
	s.CtxLog(ctx, fmt.Sprintf("Building: %+v (%v)", job, job.job.GetName()))
	builds.With(prometheus.Labels{"job": job.job.GetName(), "bits": fmt.Sprintf("%v", s.Bits)}).Inc()
	s.currentBuilds++
	_, version, err := s.scheduler.build(ctx, *job, s.Registry.Identifier, s.latestHash[job.job.Name])
	s.CtxLog(ctx, fmt.Sprintf("Complete: %v -> %v", err, version))

	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	if err != nil {
		e, ok := status.FromError(err)
		if !ok || e.Code() != codes.AlreadyExists {
			num, err := s.BounceImmediateIssue(ctx, job.job.Name, fmt.Sprintf("Build Failure for %v", job.job.Name), fmt.Sprintf("Build failed for %v: %v running on %v", job.job.Name, err, s.Registry.Identifier), false, false)
			if err != nil {
				return nil, err
			}
			config.FailureTracker[job.job.Name] = num.GetNumber()
			err = s.saveConfig(ctx, config)
			if err != nil {
				return nil, err
			}
		}
	} else {
		if val, ok := config.GetFailureTracker()[job.job.Name]; ok {
			err = s.DeleteBounceIssue(ctx, val, job.job.Name)
			if err != nil {
				return nil, err
			}
			delete(config.FailureTracker, job.job.Name)
			err = s.saveConfig(ctx, config)
			if err != nil {
				return nil, err
			}
		}
	}
	s.currentBuilds--

	return version, err
}

var (
	dequeues = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "buildserver_dequeue",
		Help: "The size of the print queue",
	}, []string{"version", "error"})
)

func (s *Server) dequeue() {
	for job := range s.queue {
		ctx, cancel := utils.ManualContext("buildserver-dequeue", time.Minute*60)
		s.CtxLog(ctx, fmt.Sprintf("Building: %v", job))
		version, err := s.build(ctx, job)
		time.Sleep(time.Second)
		s.CtxLog(ctx, fmt.Sprintf("BUILT %v, %v", version, err))
		time.Sleep(time.Second)
		dequeues.With(prometheus.Labels{"version": fmt.Sprintf("%v", version), "error": fmt.Sprintf("%v", err)}).Inc()
		if version != nil {
			s.doFanout(ctx, version)
		}
		cancel()

		time.Sleep(time.Second)
		queueSize.With(prometheus.Labels{"type": "reported"}).Set(float64(len(s.queue)))
	}
	s.done <- true
}

var (
	fanouts = promauto.NewCounterVec(prometheus.CounterOpts{Name: "buildserver_fanouts", Help: "The number of builds made"},
		[]string{"version", "error", "server"})
)

func (s *Server) doFanout(ctx context.Context, v *pb.Version) {
	if !s.testing {
		servers, err := s.FFind(ctx, "versiontracker")
		if err == nil {
			for _, server := range servers {
				fanouts.With(prometheus.Labels{"version": fmt.Sprintf("%v", v), "server": server, "error": ""}).Inc()
				s.fanoutQueue <- fanoutEntry{version: v, server: server}
			}
		} else {
			fanouts.With(prometheus.Labels{"error": fmt.Sprintf("%v", err), "version": fmt.Sprintf("%v", v), "server": ""}).Inc()
		}
	}
}

var (
	fproc = promauto.NewCounterVec(prometheus.CounterOpts{Name: "buildserver_fanoutproc", Help: "The number of builds made"},
		[]string{"error", "written"})
	flen = promauto.NewGauge(prometheus.GaugeOpts{Name: "buildserver_fanoutlen", Help: "Length of the fanout queue"})
)

func (s *Server) fanout() {
	for fanout := range s.fanoutQueue {
		ctx, cancel := utils.ManualContext("buildserver", time.Minute)
		conn, err := s.FDial(fanout.server)
		if err != nil {
			fproc.With(prometheus.Labels{"written": fanout.server, "error": fmt.Sprintf("Dial %v", err)}).Inc()
			s.fanoutQueue <- fanout
			continue
		}

		client := pbvt.NewVersionTrackerServiceClient(conn)
		_, err = client.NewVersion(ctx, &pbvt.NewVersionRequest{Version: fanout.version})
		s.CtxLog(ctx, fmt.Sprintf("Fanning out to %v: %v -> %v", fanout.server, fanout.version, err))
		if err != nil && status.Convert(err).Code() != codes.Unavailable {
			fproc.With(prometheus.Labels{"written": fanout.server, "error": fmt.Sprintf("%v", err)}).Inc()
			s.fanoutQueue <- fanout
		}
		conn.Close()
		cancel()
		fproc.With(prometheus.Labels{"written": fanout.server, "error": "none"}).Inc()

		// Slow down
		time.Sleep(time.Second)
		flen.Set(float64(len(s.fanoutQueue)))
	}
}

func (s *Server) load(ctx context.Context, v *pb.Version) {
	jobn := v.Job.Name
	if v.VersionDate > s.latestBuild[jobn] {
		s.latestBuild[jobn] = v.VersionDate
		s.latestHash[jobn] = v.GithubHash
		s.latestDate[jobn] = time.Unix(v.VersionDate, 0)

		s.latestVersion[jobn] = v.Version
		s.latest[jobn] = v
	}

	config, err := s.loadConfig(ctx)

	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Load error: %v", err))
	}

	if s.Bits == 32 {
		if val, ok := config.GetLatestVersions()[jobn]; !ok || v.VersionDate > val.GetVersionDate() {
			config.LatestVersions[jobn] = v
			err = s.saveConfig(ctx, config)
			if err != nil {
				s.CtxLog(ctx, fmt.Sprintf("Bad save: %v", err))
			}
		}
	} else {
		if val, ok := config.GetLatest64Versions()[jobn]; !ok || v.VersionDate > val.GetVersionDate() {
			config.Latest64Versions[jobn] = v
			err = s.saveConfig(ctx, config)
			if err != nil {
				s.CtxLog(ctx, fmt.Sprintf("Bad save: %v", err))
			}
		}
	}
}

func (s *Server) saveConfig(ctx context.Context, config *pb.Config) error {
	storedBuilds.Set(float64(len(config.GetLatestVersions())))

	data, err := proto.Marshal(config)
	if err != nil {
		return err
	}

	_, err = s.dhclient.Write(ctx, &dspb.WriteRequest{Key: CONFIG_KEY, Value: &anypb.Any{Value: data}})
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) loadConfig(ctx context.Context) (*pb.Config, error) {
	res, err := s.dhclient.Read(ctx, &dspb.ReadRequest{Key: CONFIG_KEY})
	if err != nil {
		if status.Convert(err).Code() == codes.InvalidArgument {
			return &pb.Config{
				LatestVersions: make(map[string]*pb.Version),
			}, nil
		}
		return nil, err
	}

	s.CtxLog(ctx, fmt.Sprintf("Read with this %v", res.GetConsensus()))

	queue := &pb.Config{}
	err = proto.Unmarshal(res.GetValue().GetValue(), queue)
	if err != nil {
		return nil, err
	}

	if queue.GetLatest64Versions() == nil {
		queue.Latest64Versions = make(map[string]*pb.Version)
	}

	if queue.GetFailureTracker() == nil {
		queue.FailureTracker = make(map[string]int32)
	}

	return queue, nil
}

func (s *Server) backgroundBuilder(ctx context.Context) error {
	oldestJob := ""
	oldest := time.Now().Unix()
	for key, val := range s.latestBuild {
		if val < oldest {
			oldest = val
			oldestJob = key
		}
	}

	s.enqueue(&pbgbs.Job{Name: oldestJob, GoPath: "github.com/brotherlogic/" + oldestJob}, false)
	return nil
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

// Init builds the server
func Init() *Server {
	s := &Server{
		&goserver.GoServer{},
		&dstore_client.DStoreClient{},
		&Scheduler{
			"/media/scratch/buildserver",
			&sync.Mutex{},
			make(map[string]*sync.Mutex),
			nil,
			"md5sum",
			nil,
			&sync.Mutex{},
			make(map[string]time.Time),
			"",
			time.Minute,
			int64(0),
			int64(0),
			int64(0),
			int32(32),
		},
		make(map[string]time.Time),
		&sync.Mutex{},
		"/media/scratch/buildserver",
		&prodLister{dir: "/media/scratch/buildserver"},
		make([]*pbgbs.Job, 0),
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
		make(map[string]int),
		&sync.Mutex{},
		make(map[string]string),
		make(map[string]int64),
		make(map[string]time.Time),
		make(map[string]string),
		make(map[string]*pb.Version),
		0,
		"",
		int64(0),
		make(chan *queueEntry, 100),
		make(chan bool),
		make(chan fanoutEntry, 100),
		false,
		"",
	}

	s.scheduler.log = s.CtxLog
	s.scheduler.load = s.load
	s.dhclient = &dstore_client.DStoreClient{Gs: s.GoServer}

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

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	s.pathMapMutex.Lock()
	defer s.pathMapMutex.Unlock()
	s.buildFailsMutex.Lock()
	defer s.buildFailsMutex.Unlock()

	counts := 0
	for _, hash := range s.latestHash {
		if len(hash) > 0 {
			counts++
		}
	}

	memoryCrashes := make(map[string]int)
	sumv := int64(0)
	largest := 0
	for _, v := range s.pathMap {
		mCrash := 0
		sumv += int64(proto.Size(v))
		if proto.Size(v) > largest {
			largest = proto.Size(v)
		}
		for _, c := range v.Crashes {
			if c.CrashType == pb.Crash_MEMORY {
				mCrash++
			}
		}

		if mCrash > 0 {
			if _, ok := memoryCrashes[v.Job.Name]; !ok {
				memoryCrashes[v.Job.Name] = 0
			}
			memoryCrashes[v.Job.Name] += mCrash
		}
	}

	return []*pbg.State{
		&pbg.State{Key: "tracked_jobs", Value: int64(len(s.jobs))},
		&pbg.State{Key: "enqueues", Value: s.enqueues},
		&pbg.State{Key: "check_error", Text: s.checkError},
		&pbg.State{Key: "latest_versions", Value: int64(len(s.latestVersion))},
		&pbg.State{Key: "build_queue_length", Value: int64(len(s.buildQueue))},
		&pbg.State{Key: "versions", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "version_size", Value: sumv},
		&pbg.State{Key: "memory", Text: fmt.Sprintf("%v", memoryCrashes)},
		&pbg.State{Key: "enabled", Text: fmt.Sprintf("%v", s.runBuild)},
		&pbg.State{Key: "buildc", Value: int64(s.buildRequest)},
		&pbg.State{Key: "concurrent_builds", Value: int64(s.currentBuilds)},
		&pbg.State{Key: "crashes", Value: s.crashes},
		&pbg.State{Key: "paths_read", Value: int64(len(s.pathMap))},
		&pbg.State{Key: "current_build", Text: s.scheduler.cbuild},
		&pbg.State{Key: "build_fails", Text: fmt.Sprintf("%v", s.buildFails)},
		&pbg.State{Key: "hashses_stored", Value: int64(counts)},
		&pbg.State{Key: "scheduled_runs", Value: s.scheduler.runs},
		&pbg.State{Key: "command_runs", Value: s.scheduler.cRuns},
		&pbg.State{Key: "command_finishes", Value: s.scheduler.cFins},
	}
}

type properties struct {
	Binaries []string
	Versions []*pb.Version
	Version  *pb.Version
}

func (s *Server) deliver(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := utils.ManualContext("buildserver-deliver", time.Hour)
	defer cancel()
	binaries := []string{}
	for _, v := range s.pathMap {
		found := false
		for _, ver := range binaries {
			if ver == v.Job.Name {
				found = true
			}
		}
		if !found {
			binaries = append(binaries, v.Job.Name)
		}
	}
	data, err := Asset("templates/main.html")
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err)
		return
	}
	err = s.render(string(data), properties{Binaries: binaries}, w)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error writing: %s", err))
	}
}

func (s *Server) deliverVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := utils.ManualContext("deliver_version", time.Minute)
	defer cancel()
	var version *pb.Version
	ver, ok := r.URL.Query()["version"]

	if !ok {
		return
	}

	for _, v := range s.pathMap {
		if v.Version == ver[0] {
			version = v
		}
	}

	data, err := Asset("templates/version.html")
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err)
		return
	}
	err = s.renderVersion(string(data), properties{Version: version}, w)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error writing: %v", err))
	}
}

func (s *Server) deliverBinary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := utils.ManualContext("deliver_binary", time.Minute)
	defer cancel()

	versions := []*pb.Version{}
	binary, ok := r.URL.Query()["binary"]

	if !ok {
		return
	}

	for _, v := range s.pathMap {
		if v.Job.Name == binary[0] {
			versions = append(versions, v)
		}
	}
	data, err := Asset("templates/binary.html")
	if err != nil {
		fmt.Fprintf(w, "Error: %v", err)
		return
	}
	err = s.renderBinary(string(data), properties{Versions: versions}, w)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error writing: %v", err))
	}
}

func (s *Server) serveUp(port int32) {
	http.HandleFunc("/", s.deliver)
	http.HandleFunc("/version", s.deliverVersion)
	http.HandleFunc("/binary", s.deliverBinary)
	err := http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
	if err != nil {
		panic(err)
	}
}

func (s *Server) aligner(ctx context.Context) error {
	for _, job := range s.jobs {
		entries, err := utils.ResolveAll("buildserver")
		if err != nil {
			return err
		}

		for _, e := range entries {
			conn, err := s.DoDial(e)
			if err != nil {
				return err
			}
			defer conn.Close()

			client := pb.NewBuildServiceClient(conn)
			latest, err := client.GetVersions(ctx, &pb.VersionRequest{Job: job, JustLatest: true})
			if err != nil {
				return err
			}

			if len(latest.GetVersions()) > 0 &&
				latest.GetVersions()[0].VersionDate > s.latestBuild[job.Name] &&
				latest.GetVersions()[0].Version != s.latestVersion[job.Name] {
				cconn, err := s.DialMaster("filecopier")
				if err != nil {
					return err
				}
				defer cconn.Close()

				cclient := pbfc.NewFileCopierServiceClient(cconn)
				cclient.QueueCopy(ctx, &pbfc.CopyRequest{
					InputFile:    latest.GetVersions()[0].Path,
					InputServer:  latest.GetVersions()[0].Server,
					OutputServer: s.Registry.Identifier,
					OutputFile:   latest.GetVersions()[0].Path,
				})
				cclient.QueueCopy(ctx, &pbfc.CopyRequest{
					InputFile:    latest.GetVersions()[0].Path + ".version",
					InputServer:  latest.GetVersions()[0].Server,
					OutputServer: s.Registry.Identifier,
					OutputFile:   latest.GetVersions()[0].Path + ".version",
				})

			}
		}
	}

	s.preloadInfo(ctx)

	return nil
}

func (s *Server) drainQueue(ctx context.Context) {
	close(s.queue)
	<-s.done
}
func (s *Server) drainAndRestoreQueue(ctx context.Context) {
	close(s.queue)
	<-s.done

	s.queue = make(chan *queueEntry)
	go func() {
		s.dequeue()
	}()
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func (s *Server) getDirSize(ctx context.Context) {
	size, err := dirSize(s.dir)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error running cleanup: %v", err))
	}
	buildStorage.Set(float64(size))
}

func (s *Server) runCleanup(ctx context.Context) {
	defer s.getDirSize(ctx)

	ctx, cancel := utils.ManualContext("buildserver-cleanup", time.Minute)
	defer cancel()
	config, err := s.loadConfig(ctx)
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error loading config for cleanup: %v", err))
	}

	os.RemoveAll("/media/scratch/buildserver/pkg")

	latestVersions := config.GetLatestVersions()
	if s.Bits == 64 {
		latestVersions = config.GetLatest64Versions()

		for _, v := range latestVersions {
			if v.GetBitSize() != 64 {
				s.RaiseIssue("Bad issue pull", fmt.Sprintf("%v is the wrong bit size -> %v vs %v", v, config.GetLatest64Versions(), config.GetLatestVersions()))
			}
		}
	}

	toRemove := []string{}
	err = filepath.Walk(s.dir+"/builds", func(p1 string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !strings.HasSuffix(info.Name(), ".version") && strings.Contains(p1, "brotherlogic") && !strings.Contains(p1, "pkg") {
			elems := strings.Split(p1, "/")
			if latestVersions[elems[7]] != nil && p1 != latestVersions[elems[7]].GetPath() {
				st, err := os.Stat(p1)
				if err == nil && time.Since(st.ModTime()) > time.Hour*24 {
					toRemove = append(toRemove, p1)
					toRemove = append(toRemove, p1+".version")
					s.CtxLog(ctx, fmt.Sprintf("Removing %v -> %v, %v", p1, config.GetLatestVersions()[elems[7]], elems))
				} else {
					s.CtxLog(ctx, fmt.Sprintf("Keeping %v", p1))
				}
			}
		}
		return err
	})
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Error walking dir: %v", err))
	}

	for _, f := range toRemove {
		err := os.Remove(f)
		if err != nil {
			s.CtxLog(ctx, fmt.Sprintf("Unable to remove %v -> %v", f, err))
		}
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
	server.PrepServer("buildserver")
	server.Register = server

	ctx, cancel := utils.ManualContext("buildserver-init", time.Minute*5)
	rcm := &rCommand{command: exec.Command("git", "config", "--global", "url.git@github.com:.insteadOf", "https://github.com")}
	server.scheduler.runAndWait(ctx, rcm)
	server.CtxLog(ctx, fmt.Sprintf("Configured %v and %v (%v)", rcm.err, rcm.output, rcm.erroutput))
	rcm2 := &rCommand{command: exec.Command("go", "env", "-w", "GOPRIVATE=github.com/brotherlogic/*")}
	server.scheduler.runAndWait(ctx, rcm2)
	server.CtxLog(ctx, fmt.Sprintf("Configured %v and %v (%v)", rcm2.err, rcm2.output, rcm2.erroutput))
	cancel()

	go func() {
		server.dequeue()
	}()
	go func() {
		server.fanout()
	}()

	ctx, cancel = utils.ManualContext("buildserver-clean", time.Minute)
	server.preloadInfo(ctx)
	server.DiskLog = true
	server.runCleanup(ctx)
	cancel()

	err := server.RegisterServerV2(false)
	if err != nil {
		return
	}

	if server.Bits == 64 {
		server.scheduler.bitSize = int32(64)
	}
	server.CtxLog(ctx, fmt.Sprintf("BITS: %v -> %v", server.Bits, server.scheduler.bitSize))
	cancel()

	fmt.Printf("%v\n", server.Serve())
}
