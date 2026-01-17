package main

import (
	"context"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	"run": func(cs CommandSlice, wait bool, commands ...[]string) bool {
		if cs != nil {
			if wait {
				return runCommands(cs)
			}

			go runCommands(cs)
			return true
		}

		if wait {
			return runCommands(commands)
		}

		go runCommands(commands)

		return true
	},
	"exec": func(stdin string, wait bool, name string, arg ...string) (result struct {
		Success bool
		Output  string
	}) {
		cmd := exec.Command(name, arg...)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}

		if wait {
			output, err := cmd.Output()
			result.Success = err == nil
			result.Output = string(output)
			return
		}

		go func() {
			if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
				cmd.Stdout = os.Stderr
				cmd.Stderr = os.Stderr
			}

			cmd.Run()
		}()

		return
	},
	"http": func(method string, url string, body string, timeout int, headers ...[2]string) (result struct {
		StatusCode int
		Headers    map[string][]string
		Body       string
	}) {
		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			result.StatusCode = -1
			return
		}

		for _, header := range headers {
			req.Header.Add(header[0], header[1])
		}

		if timeout > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
			defer cancel()

			resp, err := http.DefaultClient.Do(req.WithContext(ctx))
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
		}

		go func() {
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}

			resp.Body.Close()
		}()

		return
	},
	"list": func(serverArgs ...string) string {
		if !config.Adb.Enabled || !config.Scrcpy.Enabled {
			return ""
		}

		return list(serverArgs)
	},
	"readfile": func(name string) []string {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil
		}

		return []string{string(data)}
	},
	"writefile": func(name string, data string) bool {
		return os.WriteFile(name, []byte(data), 0600) == nil
	},
	"exists": func(name string) bool {
		_, err := os.Stat(name)
		return err == nil
	},
	"glob": func(pattern string) []string {
		matches, _ := filepath.Glob(pattern)
		return matches
	},
	"remove": func(name string) bool {
		return os.Remove(name) == nil
	},
	"httprequestheader": func(key string, value string) [2]string {
		return [2]string{key, value}
	},
	"splithostport": func(hostport string) []string {
		host, port, err := net.SplitHostPort(hostport)
		if err != nil {
			return nil
		}

		return []string{host, port}
	},
	"hexencode": func(s string) string {
		return hex.EncodeToString([]byte(s))
	},
	"hexdecode": func(s string) []string {
		decoded, err := hex.DecodeString(s)
		if err != nil {
			return nil
		}

		return []string{string(decoded)}
	},
	"iscustomcommand": func(s string) bool {
		_, ok := config.CustomCommands[s]
		return ok
	},
	"command": func(c ...string) []string {
		return c
	},
	"formattime": func(layout string) string {
		return time.Now().Format(layout)
	},
	"contains":  strings.Contains,
	"hasprefix": strings.HasPrefix,
	"hassuffix": strings.HasSuffix,
	"trimspace": strings.TrimSpace,
	"lower":     strings.ToLower,
	"upper":     strings.ToUpper,
	"split":     strings.Split,
	"join":      strings.Join,
	"match":     regexp.MatchString,
	"env":       os.Getenv,
	"pid":       os.Getpid,
}
