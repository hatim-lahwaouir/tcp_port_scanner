package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Commit struct {
	Msg      string
	Author   string
	Hash     string
	Branche  string
	FilePath string
}

type Branche struct {
	Name string
}

// commits that we need to search for

// files that we should ignore
var ignored_files []string = []string{".git"}

func IgnoreFile(path string) bool {

	for _, fn := range ignored_files {
		if strings.Contains(path, fn) {
			return true
		}
	}
	return false
}

func GetBranchesName() []string {

	var (
		cmd *exec.Cmd
		out bytes.Buffer
		brs []string
	)

	cmd = exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	for {
		line, err := out.ReadBytes('\n')

		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		line = line[:len(line)-1]

		brs = append(brs, string(line))
	}

	return brs
}

func GetCommits(br string, commits map[string]Commit) {
	var (
		cmd *exec.Cmd
		out bytes.Buffer
	)
	// switch to the branche
	cmd = exec.Command("git", "switch", br)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("git", "log", `--pretty=format:%h|%an|%s`)
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	for {
		line, err := out.ReadBytes('\n')

		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		line = line[:len(line)-1]
		arr := strings.SplitN(string(line), "|", 3)
		commits[arr[0]] = Commit{Hash: arr[0], Author: arr[1], Msg: arr[2], Branche: br}
	}
}

func Reset() {
	cmd := exec.Command("git", "switch", "main")
	if err := cmd.Run(); err != nil {
		log.Fatal(err, "we need to have access to main branch")
	}
}

func ListAllFiles(cmt Commit) []string {
	var (
		cmd       *exec.Cmd
		filesPath []string
	)

	cmd = exec.Command("git", "checkout", cmt.Hash)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	err := filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {

		if !info.IsDir() && IgnoreFile(path) == false {
			filesPath = append(filesPath, path)
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return filesPath
}

func Search(chanel chan Commit, pattern string, mt *sync.Mutex, ft *int) {
	var (
		re  *regexp.Regexp
		arr [][]byte
	)
	re = regexp.MustCompile(pattern)

	for f := range chanel {
		c, err := os.ReadFile(f.FilePath)
		if err != nil {
			log.Fatal(err)
		}
		arr = re.FindAll(c, -1)

		if len(arr) != 0 {
			fmt.Printf("At file %s\n", f.FilePath)
		}
		mt.Lock()
		*ft += 1
		mt.Unlock()
	}

}

func main() {

	if len(os.Args) != 2 {
		log.Fatal("Invalid args")
	}
	var (
		filesPath    []string
		commits      map[string]Commit
		chanel       chan Commit
		wg           sync.WaitGroup
		mt           sync.Mutex
		file_treated int
	)

	Reset()
	chanel = make(chan Commit, 50)
	commits = make(map[string]Commit)

	brs := GetBranchesName()
	for _, br := range brs {
		GetCommits(br, commits)
	}

	// creating workers
	for i := 0; i < 20; i += 1 {
		wg.Add(1)
		go func() {
			Search(chanel, os.Args[1], &mt, &file_treated)
			wg.Done()
		}()
	}

	for _, cmt := range commits {
		filesPath = ListAllFiles(cmt)
		file_treated = 0

		fmt.Printf("-- searching this commit %s --\n", cmt.Hash)
		for _, f := range filesPath {
			c_cmt := cmt
			c_cmt.FilePath = f
			chanel <- c_cmt
		}
		for {
			mt.Lock()
			if file_treated == len(filesPath) {
				mt.Unlock()
				break
			}
			mt.Unlock()
			time.Sleep(300 * time.Millisecond)
		}
	}
	close(chanel)
	wg.Wait()
	Reset()
}
