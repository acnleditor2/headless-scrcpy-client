package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

type CommandSlice [][]string

func (cs *CommandSlice) UnmarshalJSON(data []byte) error {
	var s [][]string

	if len(data) > 2 && data[0] == '{' && data[len(data)-1] == '}' {
		var m map[string]string

		err := json.Unmarshal(data, &m)
		if err != nil {
			return err
		}

		for _, v := range m {
			if len(v) > 2 && v[0] == '"' && v[len(v)-1] == '"' {
				c := ""
				err = json.Unmarshal([]byte(v), &c)
				if err != nil {
					return err
				}

				*cs = CommandSlice([][]string{{os.Expand(c, func(k string) string { return m[k] })}})
				break
			}

			err = json.Unmarshal([]byte(v), &s)
			if err == nil {
				*cs = CommandSlice(s)
				break
			}
		}

		return err
	} else if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		c := ""
		err := json.Unmarshal(data, &c)
		if err == nil {
			*cs = CommandSlice([][]string{{c}})
		}

		return err
	}

	err := json.Unmarshal(data, &s)
	if err == nil {
		*cs = CommandSlice(s)
	}

	return err
}

type HttpEndpoint struct {
	Commands         CommandSlice `json:"commands"`
	Response         string       `json:"response"`
	ClipboardCut     bool         `json:"clipboardCut"`
	ClipboardTimeout int          `json:"clipboardTimeout"`
}

func (e *HttpEndpoint) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '"' && data[len(data)-1] == '"' {
		s := ""
		err := json.Unmarshal(data, &s)
		if err == nil && s != "" {
			var ok bool
			*e, ok = defaultHttpEndpoints[s]
			if !ok {
				e.Commands = CommandSlice([][]string{{s}})
			}
		}

		return err
	}

	type Endpoint HttpEndpoint
	var endpoint Endpoint

	err := json.Unmarshal(data, &endpoint)
	if err == nil {
		*e = HttpEndpoint(endpoint)
	}

	return err
}

type HttpServerConfig struct {
	Enabled   bool                    `json:"enabled"`
	Address   string                  `json:"address"`
	Static    string                  `json:"static"`
	Cert      string                  `json:"cert"`
	Key       string                  `json:"key"`
	Endpoints map[string]HttpEndpoint `json:"endpoints"`
}

func (c *HttpServerConfig) UnmarshalJSON(data []byte) error {
	type HttpServerC HttpServerConfig

	httpServerC := HttpServerC{
		Enabled: true,
		Address: "127.0.0.1:27199",
	}

	err := json.Unmarshal(data, &httpServerC)
	if err == nil {
		*c = HttpServerConfig(httpServerC)
	}

	return err
}

type UdpServerConfig struct {
	Enabled         bool     `json:"enabled"`
	Address         string   `json:"address"`
	HandlerTemplate []string `json:"handlerTemplate"`
}

func (c *UdpServerConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true
		return json.Unmarshal(data, &c.Address)
	}

	type UdpServerC UdpServerConfig
	udpServerC := UdpServerC{Enabled: true}

	err := json.Unmarshal(data, &udpServerC)
	if err == nil {
		*c = UdpServerConfig(udpServerC)
	}

	return err
}

type StdinCommandsConfig struct {
	Enabled         bool     `json:"enabled"`
	HandlerTemplate []string `json:"handlerTemplate"`
}

func (c *StdinCommandsConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '[' && data[len(data)-1] == ']' {
		c.Enabled = true
		return json.Unmarshal(data, &c.HandlerTemplate)
	}

	type StdinCommandsC StdinCommandsConfig
	stdinCommandsC := StdinCommandsC{Enabled: true}

	err := json.Unmarshal(data, &stdinCommandsC)
	if err == nil {
		*c = StdinCommandsConfig(stdinCommandsC)
	}

	return err
}

type AdbConfig struct {
	Enabled    bool     `json:"enabled"`
	Executable string   `json:"executable"`
	Options    []string `json:"options"`
	Device     string   `json:"device"`
}

func (c *AdbConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '[' && data[len(data)-1] == ']' {
		c.Enabled = true

		err := json.Unmarshal(data, &c.Options)
		if err != nil {
			return err
		}
	} else if len(data) > 1 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true

		err := json.Unmarshal(data, &c.Executable)
		if err != nil {
			return err
		}
	}

	if !c.Enabled {
		type AdbC AdbConfig
		adbC := AdbC{Enabled: true}

		err := json.Unmarshal(data, &adbC)
		if err == nil {
			*c = AdbConfig(adbC)
		} else {
			return err
		}
	}

	if c.Executable == "" {
		var err error
		c.Executable, err = exec.LookPath("adb")
		return err
	}

	return nil
}

type ScrcpyConfig struct {
	Enabled           bool         `json:"enabled"`
	Port              int          `json:"port"`
	Video             bool         `json:"video"`
	Audio             bool         `json:"audio"`
	Control           bool         `json:"control"`
	Forward           bool         `json:"forward"`
	UhidDevices       []UhidDevice `json:"uhidDevices"`
	StdoutClipboard   bool         `json:"stdoutClipboard"`
	StdoutUhidOutput  bool         `json:"stdoutUhidOutput"`
	ConnectedCommands CommandSlice `json:"connectedCommands"`
	Server            string       `json:"server"`
	ServerVersion     string       `json:"serverVersion"`
	ServerOptions     []string     `json:"serverOptions"`
	ClipboardAutosync bool         `json:"clipboardAutosync"`
	Cleanup           bool         `json:"cleanup"`
	PowerOn           bool         `json:"powerOn"`
}

func (c *ScrcpyConfig) UnmarshalJSON(data []byte) error {
	type ScrcpyC ScrcpyConfig

	scrcpyC := ScrcpyC{
		Enabled:       true,
		Port:          27183,
		Server:        "/data/local/tmp/scrcpy-server.jar",
		ServerVersion: "3.3.3",
	}

	err := json.Unmarshal(data, &scrcpyC)
	if err == nil {
		*c = ScrcpyConfig(scrcpyC)
	}

	return err
}

type VideoDecoderConfig struct {
	Enabled    bool   `json:"enabled"`
	Executable string `json:"executable"`
	Alpha      bool   `json:"alpha"`
}

func (c *VideoDecoderConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true

		err := json.Unmarshal(data, &c.Executable)
		if err != nil {
			return err
		}
	} else if json.Unmarshal(data, &c.Alpha) == nil {
		c.Enabled = true
	}

	if !c.Enabled {
		type VideoDecoderC VideoDecoderConfig
		videoDecoderC := VideoDecoderC{Enabled: true}

		err := json.Unmarshal(data, &videoDecoderC)
		if err == nil {
			*c = VideoDecoderConfig(videoDecoderC)
		} else {
			return err
		}
	}

	if c.Executable == "" {
		var err error
		c.Executable, err = exec.LookPath("ffmpeg")
		return err
	}

	return nil
}

type UhidDevice struct {
	Id         int    `json:"id"`
	ReportDesc string `json:"reportDesc"`
	Name       string `json:"name"`
	VendorId   string `json:"vendorId"`
	ProductId  string `json:"productId"`
}

type Config struct {
	CustomCommands map[string]CommandSlice `json:"customCommands"`
	HttpServer     HttpServerConfig        `json:"httpServer"`
	UdpServer      UdpServerConfig         `json:"udpServer"`
	StdinCommands  StdinCommandsConfig     `json:"stdinCommands"`
	Adb            AdbConfig               `json:"adb"`
	Scrcpy         ScrcpyConfig            `json:"scrcpy"`
	VideoDecoder   VideoDecoderConfig      `json:"videoDecoder"`
}

type CommandHandlerData struct {
	From     string       `json:"from"`
	Commands CommandSlice `json:"commands"`
}

var stdinDecoder *json.Decoder
var config Config
var listener net.Listener
var videoSocket net.Conn
var audioSocket net.Conn
var controlSocket net.Conn
var connectionControlChannel chan bool = make(chan bool)
var videoConnectedChannel chan struct{} = make(chan struct{})
var audioConnectedChannel chan struct{} = make(chan struct{})
var clipboardChannel chan string = make(chan string)
var uhidOutputChannel chan string = make(chan string)
var deviceName string
var videoCodec uint32
var audioCodec uint32
var initialVideoWidth int
var initialVideoHeight int
var scrcpyServer *exec.Cmd
var scrcpyConnectedCommands CommandSlice
var videoFrame []byte
var videoFrameWidth int
var videoFrameHeight int
var videoFrameMutex sync.RWMutex
var udpServerHandlerChannel chan *CommandHandlerData = make(chan *CommandHandlerData)
var stdinCommandHandlerChannel chan *CommandHandlerData = make(chan *CommandHandlerData)

var commandHandlerFuncs template.FuncMap = template.FuncMap{
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
			return commandsRun(cs)
		}
		go commandsRun(cs)
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
	"command": func(c ...string) CommandSlice {
		return CommandSlice([][]string{c})
	},
}

func list(serverArg string) (string, int) {
	if !config.Adb.Enabled || !config.Scrcpy.Enabled || config.Scrcpy.Server == "" || config.Scrcpy.ServerVersion == "" {
		return "", http.StatusNotFound
	}

	var args []string
	if config.Adb.Device == "usb" {
		args = append(config.Adb.Options, "-d")
	} else if config.Adb.Device == "tcpip" {
		args = append(config.Adb.Options, "-e")
	} else if config.Adb.Device != "" {
		args = append(config.Adb.Options, "-s", config.Adb.Device)
	} else {
		args = config.Adb.Options
	}

	args = append(
		args,
		"shell",
		fmt.Sprintf("CLASSPATH=%s", config.Scrcpy.Server),
		"app_process",
		"/",
		"com.genymobile.scrcpy.Server",
		config.Scrcpy.ServerVersion,
		serverArg,
	)

	if !config.Scrcpy.Cleanup {
		args = append(args, "cleanup=false")
	}

	output, err := exec.Command(config.Adb.Executable, args...).CombinedOutput()
	if err != nil {
		return string(output), http.StatusInternalServerError
	}

	return string(output), http.StatusOK
}

func readDummyByte(c net.Conn) bool {
	data := make([]byte, 1)

	n, err := c.Read(data)
	if err != nil {
		return false
	}
	if n != 1 {
		return false
	}

	return true
}

func readDeviceMeta() bool {
	data := make([]byte, 64)
	var n int
	var err error

	if config.Scrcpy.Video {
		n, err = io.ReadFull(videoSocket, data)
	} else if config.Scrcpy.Audio {
		n, err = io.ReadFull(audioSocket, data)
	} else {
		n, err = io.ReadFull(controlSocket, data)
	}

	if err != nil {
		return false
	}

	if n != 64 {
		return false
	}

	deviceName = string(data[:bytes.IndexByte(data, 0)])
	return true
}

func httpHandler(w http.ResponseWriter, req *http.Request) {
	origin := req.Header.Get("Origin")

	w.Header().Set("Cache-Control", "no-store")

	switch req.Method {
	case http.MethodOptions:
		if req.Header.Get("Access-Control-Request-Method") == "" {
			w.Header().Set("Allow", "OPTIONS, GET")
		} else if origin != "" {
			requestHeaders := req.Header.Get("Access-Control-Request-Headers")

			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET")

			if requestHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
			}
		}
	case http.MethodGet:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		endpoint := config.HttpServer.Endpoints[req.URL.Path]

		if endpoint.Response != "" {
			switch endpoint.Response {
			case "videoStream":
				videoSendStream(w, req, true)
			case "rawVideoStream":
				videoSendStream(w, req, false)
			case "audioStream":
				audioSendStream(w, req, true)
			case "rawAudioStream":
				audioSendStream(w, req, false)
			case "clipboardStream":
				clipboardSendStream(w, req)
			case "uhidOutputStream":
				inputUhidSendOutputStream(w, req)
			case "clipboard":
				if controlSocket == nil {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				var text string
				status := clipboardGet(endpoint.ClipboardCut, &text, time.Duration(endpoint.ClipboardTimeout)*time.Millisecond)

				if status != http.StatusOK {
					w.WriteHeader(status)
					return
				}

				w.Write([]byte(text))
			case "deviceName":
				if deviceName == "" {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.Write([]byte(deviceName))
				}
			case "videoCodec":
				if videoCodec == 0 {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.Write([]byte(strconv.FormatUint(uint64(videoCodec), 10)))
				}
			case "audioCodec":
				if audioCodec == 0 {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.Write([]byte(strconv.FormatUint(uint64(audioCodec), 10)))
				}
			case "initialVideoWidth":
				if initialVideoWidth == 0 {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.Write([]byte(strconv.Itoa(initialVideoWidth)))
				}
			case "initialVideoHeight":
				if initialVideoHeight == 0 {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.Write([]byte(strconv.Itoa(initialVideoHeight)))
				}
			case "videoFrame":
				videoSendFrame(w, req)
			case "encoders", "displays", "cameras", "apps":
				output, status := list(fmt.Sprintf("list_%s=true", endpoint.Response))

				if status != http.StatusOK {
					w.WriteHeader(status)
				}

				if output != "" {
					w.Write([]byte(output))
				}
			case "cameraSizes":
				output, status := list("list_camera_sizes=true")

				if status != http.StatusOK {
					w.WriteHeader(status)
				}

				if output != "" {
					w.Write([]byte(output))
				}
			}
		} else {
			if len(endpoint.Commands) == 0 {
				var ok bool
				endpoint, ok = defaultHttpEndpoints[req.URL.Path[1:]]
				if !ok {
					endpoint.Commands = CommandSlice([][]string{{req.URL.Path[1:]}})
				}
			}

			query := req.URL.Query()
			commands := make(CommandSlice, len(endpoint.Commands))
			for i := range endpoint.Commands {
				commands[i] = make([]string, len(endpoint.Commands[i]))
				for j := range endpoint.Commands[i] {
					commands[i][j] = os.Expand(endpoint.Commands[i][j], query.Get)
				}
			}

			go commandsRun(commands)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	if len(os.Args) != 1 && len(os.Args) != 2 {
		os.Exit(1)
	}

	var err error

	if len(os.Args) == 1 || os.Args[1] == "-" {
		stdinDecoder = json.NewDecoder(os.Stdin)
		err = stdinDecoder.Decode(&config)
	} else if strings.HasPrefix(os.Args[1], "http://") || strings.HasPrefix(os.Args[1], "https://") {
		resp, err := http.Get(os.Args[1])
		if err != nil {
			panic(err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			os.Exit(1)
		}

		err = json.NewDecoder(resp.Body).Decode(&config)
		resp.Body.Close()
	} else {
		configFile, err := os.Open(os.Args[1])
		if err != nil {
			panic(err)
		}

		err = json.NewDecoder(configFile).Decode(&config)
		configFile.Close()
	}

	if err != nil {
		panic(err)
	}

	if !config.Adb.Enabled && !config.Scrcpy.Enabled {
		os.Exit(1)
	}

	if !config.HttpServer.Enabled && !config.UdpServer.Enabled && !config.StdinCommands.Enabled {
		os.Exit(1)
	}

	if config.Adb.Enabled && config.Adb.Executable == "" {
		os.Exit(1)
	}

	if config.Scrcpy.Enabled && config.Scrcpy.Port < 1 {
		os.Exit(1)
	}

	if config.VideoDecoder.Enabled && (!config.Scrcpy.Enabled || config.VideoDecoder.Executable == "") {
		os.Exit(1)
	}

	if config.HttpServer.Enabled && (config.HttpServer.Address == "" || len(config.HttpServer.Endpoints) == 0) {
		os.Exit(1)
	}

	if config.UdpServer.Enabled {
		if config.UdpServer.Address == "" {
			os.Exit(1)
		}

		if len(config.UdpServer.HandlerTemplate) > 0 {
			if config.UdpServer.HandlerTemplate[0] == "" {
				os.Exit(1)
			}

			var t *template.Template

			if strings.HasPrefix(config.UdpServer.HandlerTemplate[0], "http://") || strings.HasPrefix(config.UdpServer.HandlerTemplate[0], "https://") {
				resp, err := http.Get(config.UdpServer.HandlerTemplate[0])
				if err != nil {
					panic(err)
				}

				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					os.Exit(1)
				}

				bodyBytes, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					panic(err)
				}

				t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(string(bodyBytes)))
			} else if config.UdpServer.HandlerTemplate[0][0] == ' ' {
				b, err := os.ReadFile(config.UdpServer.HandlerTemplate[0][1:])
				if err != nil {
					panic(err)
				}

				t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(string(b)))
			} else {
				t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(strings.Join(config.UdpServer.HandlerTemplate, "\n")))
			}

			go func() {
				err := t.Execute(io.Discard, udpServerHandlerChannel)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}()
		}

	}

	if config.StdinCommands.Enabled && len(config.StdinCommands.HandlerTemplate) > 0 {
		if config.StdinCommands.HandlerTemplate[0] == "" {
			os.Exit(1)
		}

		var t *template.Template

		if strings.HasPrefix(config.StdinCommands.HandlerTemplate[0], "http://") || strings.HasPrefix(config.StdinCommands.HandlerTemplate[0], "https://") {
			resp, err := http.Get(config.StdinCommands.HandlerTemplate[0])
			if err != nil {
				panic(err)
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				os.Exit(1)
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				panic(err)
			}

			t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(string(bodyBytes)))
		} else if config.StdinCommands.HandlerTemplate[0][0] == ' ' {
			b, err := os.ReadFile(config.StdinCommands.HandlerTemplate[0][1:])
			if err != nil {
				panic(err)
			}

			t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(string(b)))
		} else {
			t = template.Must(template.New("").Funcs(commandHandlerFuncs).Parse(strings.Join(config.StdinCommands.HandlerTemplate, "\n")))
		}

		go func() {
			err := t.Execute(io.Discard, stdinCommandHandlerChannel)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}()
	}

	if config.Scrcpy.Enabled {
		scrcpyConnectedCommands = config.Scrcpy.ConnectedCommands

		if config.Scrcpy.Video && config.VideoDecoder.Enabled {
			if runtime.GOOS == "windows" {
				go videoDecodeFfmpeg()
			} else {
				_, ok := exec.Command(config.VideoDecoder.Executable).Run().(*exec.ExitError)
				if ok {
					go videoDecodeFfmpeg()
				} else {
					go videoDecode()
				}
			}
		}

		go func() {
			var err error

			if !config.Scrcpy.Forward {
				listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", config.Scrcpy.Port))
				if err != nil {
					return
				}
			}

			for connect := range connectionControlChannel {
				if connect {
					if config.Scrcpy.Forward {
						var connected bool

						for i := 0; i < 100; i++ {
							if videoSocket != nil {
								videoSocket.Close()
							}

							if audioSocket != nil {
								audioSocket.Close()
							}

							if controlSocket != nil {
								controlSocket.Close()
							}

							if i != 0 {
								time.Sleep(100 * time.Millisecond)
							}

							if config.Scrcpy.Video {
								videoSocket, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", config.Scrcpy.Port))
								if err != nil {
									break
								}

								if !readDummyByte(videoSocket) {
									continue
								}
							}

							if config.Scrcpy.Audio {
								audioSocket, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", config.Scrcpy.Port))
								if err != nil {
									break
								}

								if !config.Scrcpy.Video && !readDummyByte(audioSocket) {
									continue
								}
							}

							if config.Scrcpy.Control {
								controlSocket, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", config.Scrcpy.Port))
								if err != nil {
									break
								}

								if !config.Scrcpy.Video && !config.Scrcpy.Audio && !readDummyByte(controlSocket) {
									continue
								}
							}

							connected = true
							break
						}

						if !connected {
							continue
						}
					} else {
						if videoSocket != nil {
							videoSocket.Close()
						}

						if audioSocket != nil {
							audioSocket.Close()
						}

						if controlSocket != nil {
							controlSocket.Close()
						}

						if config.Scrcpy.Video {
							videoSocket, err = listener.Accept()
							if err != nil {
								return
							}
						}

						if config.Scrcpy.Audio {
							audioSocket, err = listener.Accept()
							if err != nil {
								return
							}
						}

						if config.Scrcpy.Control {
							controlSocket, err = listener.Accept()
							if err != nil {
								return
							}
						}
					}

					if !readDeviceMeta() {
						continue
					}

					if config.Scrcpy.Video {
						data := make([]byte, 12)
						n, err := io.ReadFull(videoSocket, data)
						if err != nil {
							continue
						}
						if n != 12 {
							continue
						}

						videoCodec = binary.BigEndian.Uint32(data[:4])
						initialVideoWidth = int(binary.BigEndian.Uint32(data[4:8]))
						initialVideoHeight = int(binary.BigEndian.Uint32(data[8:]))
					}

					if config.Scrcpy.Audio {
						data := make([]byte, 4)
						n, err := io.ReadFull(audioSocket, data)
						if err != nil {
							continue
						}
						if n != 4 {
							continue
						}

						audioCodec = binary.BigEndian.Uint32(data)
					}

					if config.Scrcpy.Control {
						if !inputUhidCreateDevices() {
							go func() { connectionControlChannel <- false }()
							continue
						}

						go func() {
							data := make([]byte, 262130)

							for {
								n, err := io.ReadFull(controlSocket, data[:1])
								if err != nil {
									return
								}
								if n != 1 {
									return
								}

								switch data[0] {
								case 0:
									n, err = io.ReadFull(controlSocket, data[:4])
									if err != nil {
										return
									}
									if n != 4 {
										return
									}

									clipboardLength := int(binary.BigEndian.Uint32(data[:4]))

									n, err = io.ReadFull(controlSocket, data[:clipboardLength])
									if err != nil {
										return
									}
									if n != clipboardLength {
										return
									}

									lineBytes, err := json.Marshal(string(data[:clipboardLength]))
									if err != nil {
										panic(err)
									}

									if config.Scrcpy.StdoutClipboard {
										fmt.Println(string(lineBytes))
									} else if config.HttpServer.Enabled {
										go func(line string) {
											clipboardChannel <- line
										}(string(lineBytes))
									}
								case 1:
									n, err = io.ReadFull(controlSocket, data[:8])
									if err != nil {
										return
									}
									if n != 8 {
										return
									}

									if config.Scrcpy.StdoutClipboard {
										fmt.Println(strconv.FormatUint(binary.BigEndian.Uint64(data[:8]), 10))
									} else if config.HttpServer.Enabled {
										go func(line string) {
											clipboardChannel <- line
										}(strconv.FormatUint(binary.BigEndian.Uint64(data[:8]), 10))
									}
								case 2:
									n, err = io.ReadFull(controlSocket, data[:4])
									if err != nil {
										return
									}
									if n != 4 {
										return
									}

									size := int(binary.BigEndian.Uint16(data[:4]))

									n, err = io.ReadFull(controlSocket, data[:size])
									if err != nil {
										return
									}
									if n != size {
										return
									}

									if config.Scrcpy.StdoutUhidOutput {
										fmt.Println(hex.EncodeToString(data[:size]))
									} else if config.HttpServer.Enabled {
										select {
										case uhidOutputChannel <- hex.EncodeToString(data[:size]):
										default:
										}
									}
								}
							}
						}()
					}

					if config.Scrcpy.Video {
						videoConnectedChannel <- struct{}{}
					}

					if config.Scrcpy.Audio {
						audioConnectedChannel <- struct{}{}
					}

					if len(scrcpyConnectedCommands) > 0 {
						go commandsRun(scrcpyConnectedCommands)
					}
				} else {
					if videoSocket != nil {
						videoSocket.Close()
					}

					if audioSocket != nil {
						audioSocket.Close()
					}

					if controlSocket != nil {
						controlSocket.Close()
					}
				}
			}
		}()
	}

	if config.HttpServer.Enabled {
		for endpointPath, endpoint := range config.HttpServer.Endpoints {
			if strings.TrimSpace(endpointPath) !=
				endpointPath || !strings.HasPrefix(endpointPath, "/") ||
				strings.HasSuffix(endpointPath, "/") {
				os.Exit(1)
			}

			if len(endpoint.Commands) > 0 && endpoint.Response != "" {
				os.Exit(1)
			}

			if endpoint.Response != "" && endpoint.Response != "videoStream" && endpoint.Response != "rawVideoStream" && endpoint.Response != "audioStream" && endpoint.Response != "rawAudioStream" && endpoint.Response != "clipboardStream" && endpoint.Response != "uhidOutputStream" && endpoint.Response != "clipboard" && endpoint.Response != "deviceName" && endpoint.Response != "videoCodec" && endpoint.Response != "audioCodec" && endpoint.Response != "initialVideoWidth" && endpoint.Response != "initialVideoHeight" && endpoint.Response != "videoFrame" && endpoint.Response != "encoders" && endpoint.Response != "displays" && endpoint.Response != "cameras" && endpoint.Response != "cameraSizes" && endpoint.Response != "apps" {
				os.Exit(1)
			}

			if endpoint.Response == "clipboard" && endpoint.ClipboardTimeout < 1 {
				os.Exit(1)
			}

			http.HandleFunc(endpointPath, httpHandler)
		}

		if config.HttpServer.Static != "" {
			http.Handle("/", http.FileServer(http.Dir(config.HttpServer.Static)))
		}

		if config.HttpServer.Cert == "" && config.HttpServer.Key == "" {
			go http.ListenAndServe(config.HttpServer.Address, nil)
		} else {
			go http.ListenAndServeTLS(config.HttpServer.Address, config.HttpServer.Cert, config.HttpServer.Key, nil)
		}
	}

	if config.UdpServer.Enabled {
		go func() {
			c, err := net.ListenPacket("udp", config.UdpServer.Address)
			if err != nil {
				return
			}
			defer c.Close()

			data := make([]byte, 1024)

			for {
				n, addr, err := c.ReadFrom(data)
				if err != nil {
					break
				}

				var cs CommandSlice

				if json.Unmarshal(data[:n], &cs) == nil && len(cs) > 0 {
					if len(config.UdpServer.HandlerTemplate) == 0 {
						go commandsRun(cs)
					} else {
						udpServerHandlerChannel <- &CommandHandlerData{
							From:     addr.String(),
							Commands: cs,
						}
					}
				}
			}
		}()
	}

	if config.StdinCommands.Enabled {
		go func() {
			if stdinDecoder == nil {
				stdinDecoder = json.NewDecoder(os.Stdin)
			}

			for {
				var cs CommandSlice

				err = stdinDecoder.Decode(&cs)
				if err != nil {
					if err == io.EOF {
						break
					} else {
						stdinDecoder = json.NewDecoder(os.Stdin)
					}

					fmt.Fprintln(os.Stderr, err)
				} else if len(cs) > 0 {
					if len(config.StdinCommands.HandlerTemplate) == 0 {
						commandsRun(cs)
					} else {
						stdinCommandHandlerChannel <- &CommandHandlerData{
							From:     "stdin",
							Commands: cs,
						}
					}
				}
			}
		}()
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt

	select {
	case connectionControlChannel <- false:
	default:
	}
}
