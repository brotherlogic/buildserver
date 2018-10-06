package main

import (
	"bufio"
	"fmt"
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
	log         func(s string)
	md5command  string
}

type rCommand struct {
	command   *exec.Cmd
	output    string
	erroutput string
	startTime int64
	endTime   int64
	err       error
}

func (s *Scheduler) build(job *pbgbs.Job) (string, error) {
	s.log(fmt.Sprintf("BUILDING %v", job.Name))

	if job.Name == "" {
		return "", fmt.Errorf("Job is not specified correctly (has no name)")
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

	// Sometimes go get takes a while to run
	time.Sleep(time.Second * 10)

	getCommand := &rCommand{command: exec.Command("go", "get", "-u", job.GoPath)}
	s.runAndWait(getCommand)

	buildCommand := &rCommand{command: exec.Command("go", "get", job.GoPath)}
	s.runAndWait(buildCommand)

	// If the build has failed, there will be no file output
	if _, err := os.Stat(s.dir + "/bin/" + job.Name); os.IsNotExist(err) {
		return "", fmt.Errorf("Build failed: %v and %v", buildCommand.output, buildCommand.erroutput)
	}

	// Sometimes go get takes a while to run
	time.Sleep(time.Second * 10)

	hashCommand := &rCommand{command: exec.Command(s.md5command, s.dir+"/bin/"+job.Name)}
	s.runAndWait(hashCommand)

	if len(strings.Fields(hashCommand.output)) == 0 {
		return "", fmt.Errorf("Build failed on hash step: %v, %v", hashCommand.output, hashCommand.erroutput)
	}

	os.MkdirAll(s.dir+"/builds/"+job.GoPath, 0755)
	copyCommand := &rCommand{command: exec.Command("mv", s.dir+"/bin/"+job.Name, s.dir+"/builds/"+job.GoPath+"/"+job.Name+"-"+strings.Fields(hashCommand.output)[0])}
	s.runAndWait(copyCommand)

	return strings.Fields(hashCommand.output)[0], nil
}

func (s *Scheduler) runAndWait(c *rCommand) {
	err := s.run(c)
	if err == nil {
		for c.endTime == 0 {
			time.Sleep(time.Second)
		}
	}
}

func (s *Scheduler) run(c *rCommand) error {
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

	out2, _ := c.command.StderrPipe()
	if out2 != nil {
		scanner := bufio.NewScanner(out2)
		go func() {
			for scanner != nil && scanner.Scan() {
				c.erroutput += scanner.Text()
			}
			out2.Close()
		}()
	}

	err := c.command.Start()
	if err != nil {
		return err
	}
	c.startTime = time.Now().Unix()

	// Monitor the job and report completion
	go func() {
		err := c.command.Wait()
		c.endTime = time.Now().Unix()
		if err != nil {
			c.err = err
		}
	}()

	return nil
}
