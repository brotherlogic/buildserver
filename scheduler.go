package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	pbgbs "github.com/brotherlogic/gobuildslave/proto"
)

// Scheduler for doing builds
type Scheduler struct {
	dir         string
	masterMutex *sync.Mutex
	mMap        map[string]*sync.Mutex
}

type rCommand struct {
	command   *exec.Cmd
	output    string
	startTime int64
	endTime   int64
	err       error
}

func (s *Scheduler) build(job *pbgbs.Job) string {

	if s.dir == "" {
		return "built"
	}

	// Prep the mutex
	s.masterMutex.Lock()
	if _, ok := s.mMap[job.Name]; !ok {
		s.mMap[job.Name] = &sync.Mutex{}
	}
	s.masterMutex.Unlock()

	// Lock the job for the duration of the build
	s.mMap[job.Name].Lock()
	defer s.mMap[job.Name].Unlock()

	buildCommand := &rCommand{command: exec.Command("go", "get", job.GoPath)}
	s.runAndWait(buildCommand)

	hashCommand := &rCommand{command: exec.Command("md5sum", s.dir+"/bin/"+job.Name)}
	s.runAndWait(hashCommand)

	os.MkdirAll(s.dir+"/builds/"+job.GoPath, 0755)
	copyCommand := &rCommand{command: exec.Command("cp", s.dir+"/bin/"+job.Name, s.dir+"/builds/"+job.GoPath+"-"+strings.Fields(hashCommand.output)[0])}
	s.runAndWait(copyCommand)

	return strings.Fields(hashCommand.output)[0]
}

func (s *Scheduler) runAndWait(c *rCommand) {
	err := s.run(c)
	if err == nil {
		for c.endTime == 0 {
			time.Sleep(time.Second)
		}
	}
	log.Printf("%v and then %v", err, c.output)
}

func (s *Scheduler) run(c *rCommand) error {
	log.Printf("RUNNING %v", c.command.Args)
	env := os.Environ()
	gpath := s.dir
	c.command.Path = strings.Replace(c.command.Path, "$GOPATH", gpath, -1)
	for i := range c.command.Args {
		c.command.Args[i] = strings.Replace(c.command.Args[i], "$GOPATH", gpath, -1)
	}
	path := fmt.Sprintf("GOPATH=" + s.dir)
	pathbin := fmt.Sprintf("GOBIN=" + s.dir + "/bin")
	found := false
	for i, blah := range env {
		if strings.HasPrefix(blah, "GOPATH") {
			env[i] = path
			found = true
		}
		if strings.HasPrefix(blah, "GOBIN") {
			env[i] = pathbin
			found = true
		}
	}
	if !found {
		env = append(env, path)
	}
	c.command.Env = env

	out, _ := c.command.StdoutPipe()

	if out != nil {
		scanner := bufio.NewScanner(out)
		go func() {
			for scanner != nil && scanner.Scan() {
				c.output += scanner.Text()
			}
			out.Close()
		}()
	}

	err := c.command.Start()
	if err != nil {
		return err
	}
	c.startTime = time.Now().Unix()

	// Monitor the job and report completion
	go func() {
		log.Printf("Monitoring")
		err := c.command.Wait()
		c.endTime = time.Now().Unix()
		log.Printf("HERE = %v", err)
		if err != nil {
			c.err = err
		}
	}()

	return nil
}
