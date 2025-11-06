package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var jsonCommandHandlerFuncs template.FuncMap = template.FuncMap{
	"atoi": func(s string) []int {
		i, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}

		return []int{i}
	},
	"contains":  strings.Contains,
	"hasprefix": strings.HasPrefix,
	"hassuffix": strings.HasSuffix,
	"lower":     strings.ToLower,
	"upper":     strings.ToUpper,
	"split":     strings.Split,
	"join":      strings.Join,
	"match":     regexp.MatchString,
	"env":       os.Getenv,
	"pid":       os.Getpid,
	"run": func(cs CommandSlice, wait bool) bool {
		if wait {
			return runCommands(cs)
		}
		go runCommands(cs)
		return true
	},
	"exec": func(stdin string, name string, arg ...string) (result struct {
		Success bool
		Output  string
	}) {
		cmd := exec.Command(name, arg...)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		output, err := cmd.CombinedOutput()
		result.Success = err == nil
		result.Output = string(output)
		return
	},
	"http": func(method string, url string, body string, timeout int, headers ...[2]string) (result struct {
		StatusCode int
		Headers    map[string][]string
		Body       string
	}) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
		defer cancel()

		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			result.StatusCode = -1
			return
		}

		for _, header := range headers {
			req.Header.Add(header[0], header[1])
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			result.StatusCode = -1
			return
		}

		result.StatusCode = resp.StatusCode
		result.Headers = resp.Header
		responseBodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		result.Body = string(responseBodyBytes)
		return
	},
	"httpRequestHeader": func(key string, value string) [2]string {
		return [2]string{key, value}
	},
	"splithostport": func(hostport string) []string {
		host, port, err := net.SplitHostPort(hostport)
		if err != nil {
			return nil
		}

		return []string{host, port}
	},
	"command": func(c ...string) CommandSlice {
		return CommandSlice([][]string{c})
	},
}
