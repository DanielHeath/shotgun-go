// A port of the ruby 'shotgun' library
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var port int
var rawurl string
var proxyUrl *url.URL

func init() {
	flag.IntVar(&port, "p", 8009, "Shorthand for --port")
	flag.IntVar(&port, "port", 8009, "The port for shotgun to listen on")
	flag.StringVar(&rawurl, "u", "", "Shorthand for --url")
	flag.StringVar(&rawurl, "url", "", "The url to proxy for")
	flag.Parse()

	// This is crap. Replace it with an unbuffered channel.
	runtime.GOMAXPROCS(1) // We need to wait for the subcommand to exit before starting the next iteration.

	var err error
	proxyUrl, err = url.Parse(rawurl)
	if rawurl == "" || err != nil {
		fmt.Fprintf(os.Stderr, "Usage of %s: %s [options] command\n", os.Args[0], os.Args[0])
		flag.PrintDefaults()
		if rawurl != "" && err != nil {
			fmt.Fprintf(os.Stderr, "URL must be a valid url:", err)
		}
		os.Exit(-1)
	}
}

func startProcess(c *exec.Cmd) chan *[]byte {
	var sem = make(chan *[]byte)

	go func() {
		out, _ := c.CombinedOutput()
		fmt.Println("******************* subprocess output **************")
		fmt.Print(string(out))
		sem <- &out
	}()
	return sem
}

func waitUntilUp(c *exec.Cmd) error {
	var wg sync.WaitGroup
	var ticks int
	var err error

	wg.Add(1)
	ticker := time.NewTicker(time.Millisecond * 50)

	once := sync.Once{}
	finished := func() {
		once.Do(func() {
			ticker.Stop()
			wg.Done()
		})
	}

	go func() {
		go func() {
			time.Sleep(time.Millisecond)
			if c.Process == nil {
				fmt.Println("Process not started")
				return
			}
			c.Process.Wait()
			finished()
			err = errors.New("Process exited")
			return
		}()

		for _ = range ticker.C {
			if c.ProcessState != nil && c.ProcessState.Exited() {
				err = errors.New("Process quit.")
			}
			_, err = http.Head(proxyUrl.String())

			if err == nil {
				// We're up!
				finished()
				return
			}

			fmt.Print(".")
			ticks = ticks + 1
			if ticks > 70 {
				fmt.Print("Giving up")
				err = errors.New(fmt.Sprintf("Process did not listen after waiting 70*50ms\n%s", err))
				finished()
				return
			}
		}
	}()

	wg.Wait()
	return err
}

func waitUntilDown(c *exec.Cmd) error {
	var wg sync.WaitGroup
	var ticks int
	var resultErr error

	wg.Add(1)
	ticker := time.NewTicker(time.Millisecond * 50)

	go func() {
		for _ = range ticker.C {
			_, err := http.Head(proxyUrl.String())

			if err != nil {
				ticker.Stop()
				wg.Done()
				return
			}

			ticks = ticks + 1
			if ticks > 50 {
				resultErr = errors.New(fmt.Sprintf("Process %s did not die after waiting", c.Process.Pid))
				ticker.Stop()
				wg.Done()
				return
			}
		}
	}()

	wg.Wait()
	return resultErr
}

func killProcess(c *exec.Cmd) {
	if c.Process != nil {
		c.Process.Signal(syscall.SIGTERM)
		c.Process.Release()
	}
	err := waitUntilDown(c)
	if err != nil {
		fmt.Println(err)
	}
}

func main() {
	proxy := httputil.NewSingleHostReverseProxy(proxyUrl)

	args := flag.Args()

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		var command *exec.Cmd

		uptodate := exec.Command(
			"bash",
			"-c",
			"[ ! -e /tmp/shotgunner ] || find src -newer /tmp/shotgunner | grep .",
		)
		uptodate.CombinedOutput() // Block until finished
		if uptodate.ProcessState.Success() {
			buildout, err := exec.Command("go", "build", "-o", "/tmp/shotgunner", args[0]).CombinedOutput()
			if err != nil {
				http.Error(rw, err.Error()+string(buildout), http.StatusInternalServerError)
				return
			}
		}
		command = exec.Command("/tmp/shotgunner")

		defer killProcess(command)
		out := startProcess(command)

		pid := -1
		if command.Process != nil {
			pid = command.Process.Pid
		}
		if command.ProcessState != nil {
			pid = command.ProcessState.Pid()
		}
		fmt.Println(pid)

		if err := waitUntilUp(command); err != nil {
			http.Error(rw, "Error starting server.", http.StatusInternalServerError)
			output := <-out
			fmt.Fprint(rw, string(*output))
			return
		}
		proxy.ServeHTTP(rw, r)
	})

	bindErr := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	fmt.Println(bindErr)
}
