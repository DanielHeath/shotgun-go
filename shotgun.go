// A port of the ruby 'shotgun' library
package main

import (
	"bytes"
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
var secondsToKill int
var msToStart int
var rawurl string
var proxyUrl *url.URL

func init() {
	flag.IntVar(&port, "p", 8009, "Shorthand for --port")
	flag.IntVar(&port, "port", 8009, "The port for shotgun to listen on")
	flag.StringVar(&rawurl, "u", "", "Shorthand for --url")
	flag.StringVar(&rawurl, "url", "", "The url to proxy for")
	flag.IntVar(&secondsToKill, "t", 1, "Shorthand for --timeout")
	flag.IntVar(&secondsToKill, "timeout", 1, "How long (seconds) to wait before sending SIGKILL to old process")
	flag.IntVar(&msToStart, "s", 100, "Shorthand for --start")
	flag.IntVar(&msToStart, "start", 100, "How long (milliseconds) to wait before forwarding request to new process")
	flag.Parse()
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

func startProcess(c *exec.Cmd) (*bytes.Buffer, error) {
	b := bytes.NewBuffer(make([]byte, 1024024))
	c.Stderr = b
	c.Stdout = b
	return b, c.Start()
}

func waitUntilUp(c *exec.Cmd) error {
	var wg sync.WaitGroup
	var ticks int
	var err error

	wg.Add(1)
	ticker := time.NewTicker(time.Millisecond * 50)

	go func() {
		for _ = range ticker.C {
			_, err = http.Head(proxyUrl.String())

			if err == nil {
				ticker.Stop()
				wg.Done()
				return
			}

			fmt.Print(".")
			ticks = ticks + 1
			if ticks > 50 {
				err = errors.New(fmt.Sprintf("Process did not listen after waiting ages\n%s", err))
				ticker.Stop()
				wg.Done()
				return
			}
		}
	}()

	wg.Wait()
	return err
}

func waitUntilDown() error {
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
				resultErr = errors.New(fmt.Sprintf("Process did not die after waiting"))
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
	err := waitUntilDown()
	if err != nil {
		fmt.Println(err)
	}
}

func main() {
	proxy := httputil.NewSingleHostReverseProxy(proxyUrl)

	args := flag.Args()

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		var command *exec.Cmd
		var err error

		os.Remove("/tmp/shotgunner")
		buildout, err := exec.Command("go", "build", "-o", "/tmp/shotgunner", args[0]).CombinedOutput()
		if err != nil {
			http.Error(rw, err.Error()+string(buildout), http.StatusInternalServerError)
			return
		}

		command = exec.Command("/tmp/shotgunner")

		defer killProcess(command)
		out, err := startProcess(command)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := waitUntilUp(command); err != nil {
			fmt.Println(out.String())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			fmt.Fprint(rw, out.String())
			return
		}
		proxy.ServeHTTP(rw, r)
	})

	bindErr := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	fmt.Println(bindErr)
}
