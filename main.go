package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func notify(notCh chan bool, path string) {
	inotifyFd, err := syscall.InotifyInit()
	// inotifyFd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("inotifyFd: %d", inotifyFd)

	wd, err := syscall.InotifyAddWatch(inotifyFd, path, syscall.IN_MODIFY)
	if err != nil {
		log.Fatalf("addWatch err: %v", err)
	}
	log.Printf("watchDesc: %d", wd)
	f := os.NewFile(uintptr(inotifyFd), "watcher")
	reader := bufio.NewReader(f)

	var buf [512]byte
	for {
		var inotifyEvent syscall.InotifyEvent
		err = binary.Read(reader, binary.LittleEndian, &inotifyEvent)
		if err != nil {
			log.Fatalf("binary.read(): %v", err)
		}

		if inotifyEvent.Len > 0 {
			n, err := io.ReadFull(reader, buf[:inotifyEvent.Len])
			if err != nil {
				log.Fatalf("io.readfull(): %v", err)
			}
			// log.Printf("read n: %d, fname: %q", n, filename[:n])
			filename := getName(buf[:n])
			log.Printf("read n: %d, filename: %q", n, filename)
			notCh <- true
		}
	}
}

// StringFromNullTerminated returns a string from a nul-terminated byte slice
func getName(b []byte) string {
	n := bytes.IndexByte(b, '\x00')
	if n < 1 {
		return ""
	}
	return string(b[:n])
}

// TODO: add diffirent colors for watcher and for subprocess output
// TODO: add gracefully shutdown for goroutines we fire
// TODO: if port is being used, the for loop in main, running without stopping, handle error(port in use) in subprocess
func main() {
	path := flag.String("p", ".", "path to watch")
	flag.Parse()
	cmdArgs := flag.Args()

	log.Printf("path: %q", *path)
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("cwd: %q", cwd)

	notCh := make(chan bool)

	go notify(notCh, *path)

	log.Printf("cmd: %v", cmdArgs)
	if len(cmdArgs) < 1 {
		log.Printf("cmdArgs must be bigger than 0: %v", cmdArgs)
		os.Exit(1)
	}

	// to direct signal into the for loop to kill subprocess before shutdown the app
	sigDir := make(chan os.Signal, 1)

	// gracefully shutdown to kill subprocess
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("got a signal to shut down: %v", s)
		sigDir <- s
		log.Println("redirect signal into the for")
		<-sig
		log.Println("got back signal from the for to here")
		os.Exit(1)
	}()

	var out bytes.Buffer
	for {
		log.Println("in for, running")

		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		// cmd := exec.Command("ls", "-l")
		cmd.Stdout = &out
		// https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773#:~:text=many%20use%20cases.-,Solution%3A,-In%20addition%20to
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		err = cmd.Start()
		if err != nil {
			log.Fatalf("start err: %v", err)
		}

		go func(cmd *exec.Cmd) {
			var s os.Signal
			select {
			case <-notCh:
				log.Println("received from notCh")
			case s = <-sigDir:
				log.Printf("got a signal to shut down: %v", s)

			}

			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if err != nil {
				log.Printf("kill err: %v", err)
			}
			log.Println("process killed")
			if s != nil {
				log.Printf("os.signal is not nil: %v", s)
				sig <- s
			}
		}(cmd)

		log.Printf("Waiting for command to finish...")
		err = cmd.Wait()
		if err != nil {
			if strings.Contains(err.Error(), "signal: killed") {
				log.Printf("we killed the process so continue: %v", err)
				continue
			}

			log.Fatalf("wait err: %#v", err)
		}
		log.Printf("out: %s", out.String())
		log.Printf("Command finished with error: %v", err)
	}
}
