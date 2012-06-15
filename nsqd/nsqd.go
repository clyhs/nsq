package main

import (
	"../util"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
)

const VERSION = "0.1"

var (
	showVersion     = flag.Bool("version", false, "print version string")
	bindAddress     = flag.String("address", "0.0.0.0", "address to bind to")
	webPort         = flag.String("web-port", "5151", "port to listen on for HTTP connections")
	tcpPort         = flag.String("tcp-port", "5150", "port to listen on for TCP connections")
	debugMode       = flag.Bool("debug", false, "enable debug mode")
	memQueueSize    = flag.Int("mem-queue-size", 10000, "number of messages to keep in memory (per topic)")
	cpuProfile      = flag.String("cpu-profile", "", "write cpu profile to file")
	goMaxProcs      = flag.Int("go-max-procs", 4, "runtime configuration for GOMAXPROCS")
	dataPath        = flag.String("data-path", "", "path to store disk-backed messages")
	lookupAddresses = util.StringArray{}
)

func init() {
	flag.Var(&lookupAddresses, "lookup-address", "lookup address (may be given multiple times)")
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("nsqd v%s\n", VERSION)
		return
	}

	if *goMaxProcs > 0 {
		runtime.GOMAXPROCS(*goMaxProcs)
	}

	endChan := make(chan int)
	signalChan := make(chan os.Signal, 1)

	if *cpuProfile != "" {
		log.Printf("CPU Profiling Enabled")
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	go func() {
		<-signalChan
		endChan <- 1
	}()
	signal.Notify(signalChan, os.Interrupt)

	tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(*bindAddress, *tcpPort))
	if err != nil {
		log.Fatal(err)
	}

	webAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(*bindAddress, *webPort))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("nsqd v%s", VERSION)

	// prepare the lookup peer instances
	lookupHosts := make([]string, 0)
	lookupHosts = append(lookupHosts, "127.0.0.1:5160")
	go LookupRouter(lookupHosts)

	go TopicFactory(*memQueueSize, *dataPath)
	go UuidFactory()

	tcpListener, err := net.Listen("tcp", tcpAddr.String())
	if err != nil {
		log.Fatalf("FATAL: listen (%s) failed - %s", tcpAddr.String(), err.Error())
	}
	go util.TcpServer(tcpListener, tcpClientHandler)

	webListener, err := net.Listen("tcp", webAddr.String())
	if err != nil {
		log.Fatalf("FATAL: listen (%s) failed - %s", webAddr.String(), err.Error())
	}
	go HttpServer(webListener)

	<-endChan

	for _, topic := range TopicMap {
		topic.Close()
	}
}
