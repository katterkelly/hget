package main

import (
	"flag"
	"github.com/katterkelly/hget"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

var displayProgress = true

func main() {
	var err error

	conn := flag.Int("n", runtime.NumCPU(), "connection")
	skiptls := flag.Bool("skip-tls", true, "skip verify certificate for https")

	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		hget.Errorln("url is required")
		usage()
		os.Exit(1)
	}

	command := args[0]
	if command == "tasks" {
		if err = hget.TaskPrint(); err != nil {
			hget.Errorf("%v\n", err)
		}
		return
	} else if command == "resume" {
		if len(args) < 2 {
			hget.Errorln("downloading task name is required")
			usage()
			os.Exit(1)
		}

		var task string
		if hget.IsUrl(args[1]) {
			task = hget.TaskFromUrl(args[1])
		} else {
			task = args[1]
		}

		state, err := hget.Resume(task)
		hget.FatalCheck(err)
		Execute(state.Url, state, *conn, *skiptls)
		return
	} else {
		if hget.ExistDir(hget.FolderOf(command)) {
			hget.Warnf("Downloading task already exist, remove first \n")
			err := os.RemoveAll(hget.FolderOf(command))
			hget.FatalCheck(err)
		}
		Execute(command, nil, *conn, *skiptls)
	}
}

func Execute(url string, state *hget.State, conn int, skiptls bool) {
	//otherwise is hget <URL> command
	var err error

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	//set up parallel

	var files = make([]string, 0)
	var parts = make([]hget.Part, 0)
	var isInterrupted = false

	doneChan := make(chan bool, conn)
	fileChan := make(chan string, conn)
	errorChan := make(chan error, 1)
	stateChan := make(chan hget.Part, 1)
	interruptChan := make(chan bool, conn)

	var downloader *hget.HttpDownloader
	if state == nil {
		downloader = hget.NewHttpDownloader(url, conn, skiptls)
	} else {
		downloader = &hget.HttpDownloader{Url: state.Url, File: filepath.Base(state.Url), Par: int64(len(state.Parts)), Parts: state.Parts, Resumable: true}
	}
	go downloader.Do(doneChan, fileChan, errorChan, interruptChan, stateChan)

	for {
		select {
		case <-signal_chan:
			//send par number of interrupt for each routine
			isInterrupted = true
			for i := 0; i < conn; i++ {
				interruptChan <- true
			}
		case file := <-fileChan:
			files = append(files, file)
		case err := <-errorChan:
			hget.Errorf("%v", err)
			panic(err) //maybe need better style
		case part := <-stateChan:
			parts = append(parts, part)
		case <-doneChan:
			if isInterrupted {
				if downloader.Resumable {
					hget.Printf("Interrupted, saving state ... \n")
					s := &hget.State{Url: url, Parts: parts}
					err := s.Save()
					if err != nil {
						hget.Errorf("%v\n", err)
					}
					return
				} else {
					hget.Warnf("Interrupted, but downloading url is not resumable, silently die")
					return
				}
			} else {
				err = hget.JoinFile(files, filepath.Base(url))
				hget.FatalCheck(err)
				err = os.RemoveAll(hget.FolderOf(url))
				hget.FatalCheck(err)
				return
			}
		}
	}
}

func usage() {
	hget.Printf(`Usage:
hget [URL] [-n connection] [-skip-tls true]
hget tasks
hget resume [TaskName]
`)
}
