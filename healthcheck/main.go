// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// checkTimeout bounds each check: a peer that accepts and never answers fails the check
// instead of hanging it.
var checkTimeout = 10 * time.Second

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

// checkURL returns the URL of an -http check: an absolute http or https URL with a host,
// rebuilt from its parsed parts.
func checkURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("%q: use an http:// or https:// URL with a host and no credentials", s)
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath, RawQuery: u.RawQuery}, nil
}

// run performs the TCP and HTTP checks given by args (-tcp host:port and -http url, both
// repeatable) and returns the process exit code: 0 when every check passed, 1 on the first
// failed check, 2 on invalid flags (an -http value that is not an http or https URL included).
// Each check gives up after checkTimeout.
func run(args []string, logger *log.Logger) int {
	var tcpFlags, httpFlags arrayFlags
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.Var(&tcpFlags, "tcp", "tcpcheck: ex: my.domain.com:80")
	fs.Var(&httpFlags, "http", "httpcheck: ex: http://my.domain.com:80/health")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	urls := make([]*url.URL, 0, len(httpFlags))
	for _, s := range httpFlags {
		u, err := checkURL(s)
		if err != nil {
			logger.Println("Invalid -http:", err)
			return 2
		}
		urls = append(urls, u)
	}

	for _, tcpAddr := range tcpFlags {
		conn, err := net.DialTimeout("tcp", tcpAddr, checkTimeout)
		if err != nil {
			logger.Println("Connection error:", err)
			logger.Println(tcpAddr, ": Unreachable")
			return 1
		}
		_ = conn.Close()
		logger.Println(tcpAddr, ": Online")
	}

	client := &http.Client{Timeout: checkTimeout}
	for i, httpURL := range urls {
		status, err := get(client, httpURL)
		if err != nil {
			logger.Println("Connection error:", err)
			logger.Println(httpFlags[i], ": Unreachable")
			return 1
		}
		if status < 400 {
			logger.Println(httpFlags[i], ": Online - response:", status)
		} else {
			logger.Println(httpFlags[i], ": Error - response:", status)
			return 1
		}
	}

	return 0
}

// get requests u and returns the response status.
func get(client *http.Client, u *url.URL) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	req := (&http.Request{Method: http.MethodGet, URL: u, Host: u.Host, Header: http.Header{}}).WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}
