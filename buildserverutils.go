package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/brotherlogic/buildserver/proto"
	"github.com/golang/protobuf/proto"
)

func (s *Server) preloadInfo() error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".version") {
			data, _ := ioutil.ReadFile(path)
			val := &pb.Version{}
			proto.Unmarshal(data, val)
			s.pathMapMutex.Lock()
			s.pathMap[path[:len(path)-len(".version")]] = val
			s.pathMapMutex.Unlock()
		}
		return nil
	})
}
