package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], log.Default()))
}

// run performs the TCP and HTTP checks given by args (-tcp host:port and -http url, both
// repeatable) and returns the process exit code: 0 when every check passed, 1 on the first
// failed check, 2 on invalid flags.
func run(args []string, logger *log.Logger) int {
	var tcpFlags, httpFlags arrayFlags
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.Var(&tcpFlags, "tcp", "tcpcheck: ex: my.domain.com:80")
	fs.Var(&httpFlags, "http", "httpcheck: ex: my.domain.com:80")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	for _, tcpAddr := range tcpFlags {
		conn, err := net.Dial("tcp", tcpAddr)
		if err != nil {
			logger.Println("Connection error:", err)
			logger.Println(tcpAddr, ": Unreachable")
			return 1
		}
		defer func() { _ = conn.Close() }()
		logger.Println(tcpAddr, ": Online")
	}

	for _, httpURL := range httpFlags {
		resp, err := http.Get(httpURL)
		if err != nil {
			// handle error
			logger.Println("Connection error:", err)
			logger.Println(httpURL, ": Unreachable")
			return 1
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			logger.Println(httpURL, ": Online - response:", resp.StatusCode)
		} else {
			logger.Println(httpURL, ": Error - response:", resp.StatusCode)
			return 1
		}
	}

	return 0
}
