package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"nametag-updater/utils"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"time"
)

type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

var (
	latestVersion           = "1.0.22"
	lastKnownWorkingVersion = "1.0.22"
	failedVersions          []string
	ghRelease               GitHubRelease
)

func checkForUpdates() error {
	url := "https://api.github.com/repos/mdamour1976/ntworker/releases/latest"

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API failed: %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return fmt.Errorf("JSON parse failed: %w", err)
	}
	latestVersion = ghRelease.TagName
	return nil
}

func updatePoller(interval time.Duration) {
	for {
		// we sleep first because we force a checkForUpdates() upon startup
		// so it doesn't make sense to do it twice
		time.Sleep(interval)
		err := checkForUpdates()
		if err != nil {
			log.Println(err)
		}
	}
}

func startIPC(address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	log.Println("IPC listening on " + address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
		}
		go func() {
			defer conn.Close()
			// ignore all incoming body contents (no read)
			// never send a known bad version
			if slices.Contains(failedVersions, latestVersion) {
				// write the lastKnownWorkingVersion
				conn.Write([]byte(lastKnownWorkingVersion))
			} else {
				// write the currentVersion
				conn.Write([]byte(latestVersion))
			}
		}()
	}
}

func startChildProcess(version string) {

	// download and extract the specified version (if necessary)
	utils.DownloadAndExtract(version)

	// worker program runs indefinitely unless a new version is detected or a critical failure has occurred
	// start worker with appropriate architecture/platform/version
	command := "worker"
	if runtime.GOOS == "windows" {
		command += ".exe"
	}

	worker := *exec.Command("./updates/"+version+"/"+command, "--update-interval=1")
	stdout, err := worker.StderrPipe()
	if err != nil {
		panic(err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Println("Worker:", scanner.Text())
		}
	}()

	if err := worker.Start(); err != nil {
		panic(err)
	}

	if err := worker.Wait(); err != nil {
		log.Println("Failed to start application, rolling back: " + err.Error())
		failedVersions = append(failedVersions, version)
		startChildProcess(lastKnownWorkingVersion)
	} else {
		log.Println("Application terminated normally due to pending update")
		// successful program execution, new version detected
		lastKnownWorkingVersion = version
		startChildProcess(latestVersion)
	}
}

func main() {
	updateIntervalFlag := flag.Int("update-interval", 60, "Specify the update checking interval in minutes")
	portFlag := flag.Int("ipc-port", 9999, "Specify the update checking interval in hours")
	flag.StringVar(&lastKnownWorkingVersion, "known-working-version", "1.0.22", "Specify the last known working version")
	flag.Parse()

	flag.VisitAll(func(f *flag.Flag) {
		log.Printf("Flag: -%s=%s (default: %s)\n", f.Name, f.Value, f.DefValue)
	})

	log.Println("Program starting...")

	// pull latest version upon startup
	checkForUpdates()

	// start update check polling
	// consider adding command line args for poll interval
	go updatePoller(time.Duration(*updateIntervalFlag) * time.Minute)

	// start TCP IPC because it's going to be the most portable
	address := "127.0.0.1:" + strconv.Itoa(*portFlag)
	go startIPC(address)

	// start child
	startChildProcess(ghRelease.TagName)

	// should be unreachable
	log.Println("Program terminating...")
}
