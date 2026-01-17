package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
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
	"path/filepath"
	"runtime"
	"slices"
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
				if err == nil {
					*cs = CommandSlice([][]string{{os.Expand(c, func(k string) string { return m[k] })}})
				}

				return err
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

type JsonCommandHandlerTemplate string

func (t *JsonCommandHandlerTemplate) UnmarshalJSON(data []byte) error {
	if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		s := ""
		err := json.Unmarshal(data, &s)
		if err != nil {
			return err
		}

		if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
			resp, err := http.Get(s)
			if err != nil {
				return err
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return fmt.Errorf("%s returned %s", s, resp.Status)
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return err
			}

			*t = JsonCommandHandlerTemplate(bodyBytes)
		} else {
			b, err := os.ReadFile(s)
			if err != nil {
				return err
			}

			*t = JsonCommandHandlerTemplate(b)
		}

		return nil
	}

	var code []string

	err := json.Unmarshal(data, &code)
	if err == nil {
		*t = JsonCommandHandlerTemplate(strings.Join(code, "\n"))
	}

	return err
}

type HttpServerConfig struct {
	Enabled           bool                `json:"enabled"`
	Address           string              `json:"address"`
	Static            string              `json:"static"`
	Cert              string              `json:"cert"`
	Key               string              `json:"key"`
	ClientCa          string              `json:"clientCa"`
	RequireClientCert bool                `json:"requireClientCert"`
	Endpoints         map[string][]string `json:"endpoints"`
}

func (c *HttpServerConfig) UnmarshalJSON(data []byte) error {
	{
		var wdStatic bool
		if json.Unmarshal(data, &wdStatic) == nil {
			c.Enabled = true
			c.Address = "127.0.0.1:27199"

			if wdStatic {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}

				c.Static = wd
			}

			return nil
		}
	}

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

type TcpJsonCommandsConfig struct {
	Enabled         bool   `json:"enabled"`
	Address         string `json:"address"`
	HandlerTemplate string `json:"handlerTemplate"`
}

func (c *TcpJsonCommandsConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true
		return json.Unmarshal(data, &c.Address)
	}

	type TcpJsonCommandsC TcpJsonCommandsConfig
	tcpJsonCommandsC := TcpJsonCommandsC{Enabled: true}

	err := json.Unmarshal(data, &tcpJsonCommandsC)
	if err == nil {
		*c = TcpJsonCommandsConfig(tcpJsonCommandsC)
	}

	return err
}

type UdpJsonCommandsConfig struct {
	Enabled         bool   `json:"enabled"`
	Address         string `json:"address"`
	HandlerTemplate string `json:"handlerTemplate"`
}

func (c *UdpJsonCommandsConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true
		return json.Unmarshal(data, &c.Address)
	}

	type UdpJsonCommandsC UdpJsonCommandsConfig
	udpJsonCommandsC := UdpJsonCommandsC{Enabled: true}

	err := json.Unmarshal(data, &udpJsonCommandsC)
	if err == nil {
		*c = UdpJsonCommandsConfig(udpJsonCommandsC)
	}

	return err
}

type TlsJsonCommandsConfig struct {
	Enabled           bool     `json:"enabled"`
	Address           string   `json:"address"`
	Cert              string   `json:"cert"`
	Key               string   `json:"key"`
	ClientCa          string   `json:"clientCa"`
	RequireClientCert bool     `json:"requireClientCert"`
	Clients           []string `json:"clients"`
	HandlerTemplate   string   `json:"handlerTemplate"`
}

type StdinJsonCommandsConfig struct {
	Enabled         bool   `json:"enabled"`
	HandlerTemplate string `json:"handlerTemplate"`
}

func (c *StdinJsonCommandsConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '"' && data[len(data)-1] == '"' {
		c.Enabled = true
		return json.Unmarshal(data, &c.HandlerTemplate)
	}

	type StdinJsonCommandsC StdinJsonCommandsConfig
	stdinJsonCommandsC := StdinJsonCommandsC{Enabled: true}

	err := json.Unmarshal(data, &stdinJsonCommandsC)
	if err == nil {
		*c = StdinJsonCommandsConfig(stdinJsonCommandsC)
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
		if err != nil {
			return err
		}

		*c = AdbConfig(adbC)
	}

	if c.Enabled && c.Executable == "" {
		var err error
		c.Executable, err = exec.LookPath("adb")
		return err
	}

	return nil
}

type ScrcpyConfig struct {
	Enabled              bool         `json:"enabled"`
	Address              string       `json:"address"`
	Video                bool         `json:"video"`
	Audio                bool         `json:"audio"`
	Control              bool         `json:"control"`
	Forward              bool         `json:"forward"`
	UhidDevices          []UhidDevice `json:"uhidDevices"`
	StdoutClipboard      bool         `json:"stdoutClipboard"`
	StdoutUhidOutput     bool         `json:"stdoutUhidOutput"`
	StderrClipboard      bool         `json:"stderrClipboard"`
	StderrUhidOutput     bool         `json:"stderrUhidOutput"`
	StdoutVideoStream    bool         `json:"stdoutVideoStream"`
	StdoutVideoStreamRaw bool         `json:"stdoutVideoStreamRaw"`
	StdoutAudioStream    bool         `json:"stdoutAudioStream"`
	StdoutAudioStreamRaw bool         `json:"stdoutAudioStreamRaw"`
	ConnectedCommands    CommandSlice `json:"connectedCommands"`
	Server               string       `json:"server"`
	ServerVersion        string       `json:"serverVersion"`
	ServerOptions        []string     `json:"serverOptions"`
	ClipboardAutosync    bool         `json:"clipboardAutosync"`
	Cleanup              bool         `json:"cleanup"`
	PowerOn              bool         `json:"powerOn"`
}

func (c *ScrcpyConfig) UnmarshalJSON(data []byte) error {
	type ScrcpyC ScrcpyConfig

	scrcpyC := ScrcpyC{
		Enabled:       true,
		Address:       "127.0.0.1:27183",
		Server:        "/data/local/tmp/scrcpy-server.jar",
		ServerVersion: "3.3.4",
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
		if err != nil {
			return err
		}

		*c = VideoDecoderConfig(videoDecoderC)
	}

	if c.Enabled && c.Executable == "" {
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
	CustomCommands              map[string]CommandSlice               `json:"customCommands"`
	JsonCommandHandlerTemplates map[string]JsonCommandHandlerTemplate `json:"jsonCommandHandlerTemplates"`
	HttpServer                  HttpServerConfig                      `json:"httpServer"`
	TcpJsonCommands             TcpJsonCommandsConfig                 `json:"tcpJsonCommands"`
	UdpJsonCommands             UdpJsonCommandsConfig                 `json:"udpJsonCommands"`
	TlsJsonCommands             TlsJsonCommandsConfig                 `json:"tlsJsonCommands"`
	StdinJsonCommands           StdinJsonCommandsConfig               `json:"stdinJsonCommands"`
	Adb                         AdbConfig                             `json:"adb"`
	Scrcpy                      ScrcpyConfig                          `json:"scrcpy"`
	VideoDecoder                VideoDecoderConfig                    `json:"videoDecoder"`
}

type JsonCommandHandlerData struct {
	Server       string
	Address      string
	HttpEndpoint string
	HttpQuery    map[string][]string
	HttpHeaders  map[string][]string
	TlsClient    string
	Commands     CommandSlice
}

var stdinDecoder *json.Decoder
var config Config
var scrcpyListener net.Listener
var videoSocket net.Conn
var audioSocket net.Conn
var controlSocket net.Conn
var connectionControlChannel chan string = make(chan string)
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
var jsonCommandHandlerChannels map[string]chan *JsonCommandHandlerData = map[string]chan *JsonCommandHandlerData{}

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

func tlsClientAuth(clients []string, tlsState *tls.ConnectionState) string {
	if tlsState == nil || len(tlsState.PeerCertificates) == 0 {
		if len(clients) == 0 {
			return ""
		} else {
			return " "
		}
	}

	client := tlsState.PeerCertificates[0].Subject.CommonName

	if len(clients) == 0 {
		return client
	}

	if len(clients) == 1 && clients[0] == "*" {
		return client
	}

	if len(clients) == 2 && (clients[0] == "filenames" || clients[0] == "hexfilenames") {
		var err error
		if clients[0] == "filenames" {
			_, err = os.Stat(filepath.Join(clients[1], client))
		} else {
			_, err = os.Stat(filepath.Join(clients[1], hex.EncodeToString([]byte(client))))
		}

		if err != nil {
			return " "
		}

		return client
	}

	if slices.Contains(clients, client) {
		return client
	}

	return " "
}

func list(serverArgs []string) string {
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
	)

	args = append(args, serverArgs...)

	if !config.Scrcpy.Cleanup {
		args = append(args, "cleanup=false")
	}

	output, err := exec.Command(config.Adb.Executable, args...).CombinedOutput()
	if err != nil {
		if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrClipboard {
			fmt.Fprintln(os.Stderr, err)

			if len(output) > 0 {
				fmt.Fprintln(os.Stderr, string(output))
			}
		}

		return ""
	}

	return string(output)
}

func commandHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if config.HttpServer.ClientCa != "" && tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS) == " " {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	origin := req.Header.Get("Origin")

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

		go runCommands([][]string{{req.URL.Path[1:]}})
		w.WriteHeader(http.StatusNoContent)
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func infoHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if config.HttpServer.ClientCa != "" && tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS) == " " {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	origin := req.Header.Get("Origin")

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

		switch req.URL.Path {
		case "/devicename":
			if deviceName == "" {
				http.NotFound(w, req)
				return
			}

			w.Write([]byte(deviceName))
		case "/videocodec":
			if videoCodec == 0 {
				http.NotFound(w, req)
				return
			}

			w.Write([]byte(strconv.FormatUint(uint64(videoCodec), 10)))
		case "/audiocodec":
			if audioCodec == 0 {
				http.NotFound(w, req)
				return
			}

			w.Write([]byte(strconv.FormatUint(uint64(audioCodec), 10)))
		case "/initialvideowidth":
			if initialVideoWidth == 0 {
				http.NotFound(w, req)
				return
			}

			w.Write([]byte(strconv.Itoa(initialVideoWidth)))
		case "/initialvideoheight":
			if initialVideoHeight == 0 {
				http.NotFound(w, req)
				return
			}

			w.Write([]byte(strconv.Itoa(initialVideoHeight)))
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

func listHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if config.HttpServer.ClientCa != "" && tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS) == " " {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	origin := req.Header.Get("Origin")

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

		var serverArg string
		if req.URL.Path == "/camerasizes" {
			serverArg = "list_camera_sizes=true"
		} else {
			serverArg = fmt.Sprintf("list_%s=true", req.URL.Path[1:])
		}

		output := list([]string{serverArg})

		if output == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Write([]byte(output))
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func jsonCommandsHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	var tlsClient string
	if config.HttpServer.ClientCa != "" {
		tlsClient = tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS)
		if tlsClient == " " {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	origin := req.Header.Get("Origin")

	switch req.Method {
	case http.MethodOptions:
		if req.Header.Get("Access-Control-Request-Method") == "" {
			w.Header().Set("Allow", "OPTIONS, POST")
		} else if origin != "" {
			requestHeaders := req.Header.Get("Access-Control-Request-Headers")

			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "POST")

			if requestHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
			}
		}
	case http.MethodPost:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		decoder := json.NewDecoder(req.Body)
		var err error

		for {
			var cs CommandSlice

			err = decoder.Decode(&cs)
			if err != nil {
				break
			}

			if len(cs) > 0 {
				jsonCommandHandlerChannels[req.URL.Path[1:]] <- &JsonCommandHandlerData{
					Server:       "http",
					Address:      req.RemoteAddr,
					HttpEndpoint: req.URL.Path,
					HttpQuery:    req.URL.Query(),
					HttpHeaders:  req.Header,
					TlsClient:    tlsClient,
					Commands:     cs,
				}
			}
		}
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, POST")
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
		var resp *http.Response
		resp, err = http.Get(os.Args[1])
		if err != nil {
			panic(err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			fmt.Fprintln(os.Stderr, os.Args[1], "returned", resp.Status)
			os.Exit(1)
		}

		err = json.NewDecoder(resp.Body).Decode(&config)
		resp.Body.Close()
	} else {
		var configFile *os.File
		configFile, err = os.Open(os.Args[1])
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

	if !config.HttpServer.Enabled && !config.TcpJsonCommands.Enabled && !config.UdpJsonCommands.Enabled && !config.TlsJsonCommands.Enabled && !config.StdinJsonCommands.Enabled {
		os.Exit(1)
	}

	if config.VideoDecoder.Enabled && !config.Scrcpy.Enabled {
		os.Exit(1)
	}

	if config.HttpServer.Enabled && config.HttpServer.Address == "" {
		os.Exit(1)
	}

	if config.TcpJsonCommands.Enabled {
		if config.TcpJsonCommands.Address == "" {
			os.Exit(1)
		}

		if config.TcpJsonCommands.HandlerTemplate != "" && config.JsonCommandHandlerTemplates[config.TcpJsonCommands.HandlerTemplate] == "" {
			os.Exit(1)
		}
	}

	if config.UdpJsonCommands.Enabled {
		if config.UdpJsonCommands.Address == "" {
			os.Exit(1)
		}

		if config.UdpJsonCommands.HandlerTemplate != "" && config.JsonCommandHandlerTemplates[config.UdpJsonCommands.HandlerTemplate] == "" {
			os.Exit(1)
		}
	}

	if config.TlsJsonCommands.Enabled {
		if config.TlsJsonCommands.Address == "" {
			os.Exit(1)
		}

		if config.TlsJsonCommands.Cert == "" {
			os.Exit(1)
		}

		if config.TlsJsonCommands.Key == "" {
			os.Exit(1)
		}

		if config.TlsJsonCommands.HandlerTemplate != "" && config.JsonCommandHandlerTemplates[config.TlsJsonCommands.HandlerTemplate] == "" {
			os.Exit(1)
		}
	}

	if config.StdinJsonCommands.Enabled && config.StdinJsonCommands.HandlerTemplate != "" && config.JsonCommandHandlerTemplates[config.StdinJsonCommands.HandlerTemplate] == "" {
		os.Exit(1)
	}

	for handlerTemplateName, handlerTemplate := range config.JsonCommandHandlerTemplates {
		t := template.Must(template.New("").Funcs(jsonCommandHandlerFuncs).Parse(string(handlerTemplate)))
		jsonCommandHandlerChannels[handlerTemplateName] = make(chan *JsonCommandHandlerData)

		go func(c chan *JsonCommandHandlerData) {
			err := t.Execute(io.Discard, c)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}(jsonCommandHandlerChannels[handlerTemplateName])
	}

	if config.Scrcpy.Enabled {
		scrcpyConnectedCommands = config.Scrcpy.ConnectedCommands

		if config.Scrcpy.Video {
			if config.Scrcpy.StdoutVideoStream {
				go func() {
					for {
						<-videoConnectedChannel
						writeVideoStream(config.Scrcpy.StdoutVideoStreamRaw, os.Stdout, nil)
					}
				}()
			} else if config.VideoDecoder.Enabled {
				if runtime.GOOS == "windows" {
					go decodeVideoFfmpeg()
				} else {
					_, ok := exec.Command(config.VideoDecoder.Executable).Run().(*exec.ExitError)
					if ok {
						go decodeVideoFfmpeg()
					} else {
						go decodeVideo()
					}
				}
			}
		}

		if config.Scrcpy.Audio && config.Scrcpy.StdoutAudioStream {
			go func() {
				for {
					<-audioConnectedChannel
					writeAudioStream(config.Scrcpy.StdoutAudioStreamRaw, os.Stdout, nil)
				}
			}()
		}

		go func() {
			var err error

			if !config.Scrcpy.Forward {
				scrcpyListener, err = net.Listen("tcp", config.Scrcpy.Address)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return
				}
				defer scrcpyListener.Close()
			}

			for address := range connectionControlChannel {
				if address == "" {
					if videoSocket != nil {
						videoSocket.Close()
					}

					if audioSocket != nil {
						audioSocket.Close()
					}

					if controlSocket != nil {
						controlSocket.Close()
					}
				} else {
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
								videoSocket, err = net.Dial("tcp", address)
								if err != nil {
									break
								}

								if !readDummyByte(videoSocket) {
									continue
								}
							}

							if config.Scrcpy.Audio {
								audioSocket, err = net.Dial("tcp", address)
								if err != nil {
									break
								}

								if !config.Scrcpy.Video && !readDummyByte(audioSocket) {
									continue
								}
							}

							if config.Scrcpy.Control {
								controlSocket, err = net.Dial("tcp", address)
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
							videoSocket, err = scrcpyListener.Accept()
							if err != nil {
								return
							}
						}

						if config.Scrcpy.Audio {
							audioSocket, err = scrcpyListener.Accept()
							if err != nil {
								return
							}
						}

						if config.Scrcpy.Control {
							controlSocket, err = scrcpyListener.Accept()
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
						if !createUhidDevices() {
							go func() { connectionControlChannel <- "" }()
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
								case ScrcpyDeviceMessageTypes.Clipboard:
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
									}

									if config.Scrcpy.StderrClipboard {
										fmt.Fprintln(os.Stderr, string(lineBytes))
									}

									select {
									case clipboardChannel <- string(lineBytes):
									default:
									}
								case ScrcpyDeviceMessageTypes.AckClipboard:
									n, err = io.ReadFull(controlSocket, data[:8])
									if err != nil {
										return
									}
									if n != 8 {
										return
									}

									line := strconv.FormatUint(binary.BigEndian.Uint64(data[:8]), 10)

									if config.Scrcpy.StdoutClipboard {
										fmt.Println(line)
									}

									if config.Scrcpy.StderrClipboard {
										fmt.Fprintln(os.Stderr, line)
									}

									select {
									case clipboardChannel <- line:
									default:
									}
								case ScrcpyDeviceMessageTypes.UhidOutput:
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

									line := hex.EncodeToString(data[:size])

									if config.Scrcpy.StdoutUhidOutput {
										fmt.Println(line)
									}

									if config.Scrcpy.StderrUhidOutput {
										fmt.Fprintln(os.Stderr, line)
									}

									select {
									case uhidOutputChannel <- line:
									default:
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
						go runCommands(scrcpyConnectedCommands)
					}
				}
			}
		}()
	}

	if config.HttpServer.Enabled {
		endpoint := func(path string, handler func(http.ResponseWriter, *http.Request)) {
			if len(config.HttpServer.Endpoints) > 0 {
				_, ok := config.HttpServer.Endpoints[path]
				if !ok {
					return
				}
			}

			http.HandleFunc(path, handler)
		}

		if config.Scrcpy.Enabled {
			endpoint("/connect", commandHandler)
			endpoint("/disconnect", commandHandler)
			endpoint("/devicename", infoHandler)

			if config.Scrcpy.Video {
				endpoint("/videocodec", infoHandler)
				endpoint("/initialvideowidth", infoHandler)
				endpoint("/initialvideoheight", infoHandler)

				if !config.Scrcpy.StdoutVideoStream {
					if config.VideoDecoder.Enabled {
						endpoint("/videoframe", videoFrameHandler)
					} else {
						endpoint("/videostream", videoStreamHandler)
						endpoint("/rawvideostream", videoStreamHandler)
					}
				}
			}

			if config.Scrcpy.Audio {
				endpoint("/audiocodec", infoHandler)

				if !config.Scrcpy.StdoutAudioStream {
					endpoint("/audiostream", audioStreamHandler)
					endpoint("/rawaudiostream", audioStreamHandler)
				}
			}

			if config.Scrcpy.Control {
				endpoint("/key", keyHandler)
				endpoint("/keydown", keyHandler)
				endpoint("/keyup", keyHandler)
				endpoint("/type", typeHandler)
				endpoint("/touch", touchHandler)
				endpoint("/touchdown", touchHandler)
				endpoint("/touchup", touchHandler)
				endpoint("/touchmove", touchHandler)
				endpoint("/mouseclick", mouseHandler)
				endpoint("/mousedown", mouseHandler)
				endpoint("/mouseup", mouseHandler)
				endpoint("/mousemove", mouseHandler)
				endpoint("/scrollleft", scrollHandler)
				endpoint("/scrollright", scrollHandler)
				endpoint("/scrollup", scrollHandler)
				endpoint("/scrolldown", scrollHandler)
				endpoint("/getclipboard", commandHandler)
				endpoint("/getclipboardcut", commandHandler)
				endpoint("/clipboard", clipboardHandler)
				endpoint("/clipboardcut", clipboardHandler)
				endpoint("/setclipboard", setClipboardHandler)
				endpoint("/setclipboardpaste", setClipboardHandler)
				endpoint("/clipboardstream", clipboardStreamHandler)
				endpoint("/uhidinput", uhidInputHandler)
				endpoint("/uhidoutputstream", uhidOutputStreamHandler)
				endpoint("/openhardkeyboardsettings", commandHandler)
				endpoint("/backorscreenon", commandHandler)
				endpoint("/expandnotificationspanel", commandHandler)
				endpoint("/expandsettingspanel", commandHandler)
				endpoint("/collapsepanels", commandHandler)
				endpoint("/turnscreenon", commandHandler)
				endpoint("/turnscreenoff", commandHandler)
				endpoint("/rotate", commandHandler)
				endpoint("/resetvideo", commandHandler)
			}

			if config.Adb.Enabled {
				endpoint("/startscrcpyserver", commandHandler)
				endpoint("/stopscrcpyserver", commandHandler)
				endpoint("/encoders", listHandler)
				endpoint("/displays", listHandler)
				endpoint("/cameras", listHandler)
				endpoint("/apps", listHandler)
				endpoint("/camerasizes", listHandler)
			}
		}

		for name := range config.CustomCommands {
			endpoint(fmt.Sprintf("/%s", name), commandHandler)
		}

		for name := range config.JsonCommandHandlerTemplates {
			endpoint(fmt.Sprintf("/%s", name), jsonCommandsHandler)
		}

		if config.HttpServer.Static != "" {
			http.Handle("/", http.FileServer(http.Dir(config.HttpServer.Static)))
		}

		server := &http.Server{Addr: config.HttpServer.Address}

		if config.HttpServer.Cert == "" && config.HttpServer.Key == "" {
			go func() {
				err := server.ListenAndServe()
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}()
		} else {
			serverCert, err := tls.LoadX509KeyPair(config.HttpServer.Cert, config.HttpServer.Key)
			if err != nil {
				panic(err)
			}

			tlsConfig := &tls.Config{Certificates: []tls.Certificate{serverCert}}

			if config.HttpServer.ClientCa != "" {
				caCert, _ := os.ReadFile(config.HttpServer.ClientCa)
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCert)
				tlsConfig.ClientCAs = caCertPool

				if config.HttpServer.RequireClientCert {
					tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
				} else {
					tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
				}
			}

			server.TLSConfig = tlsConfig

			go func() {
				err := server.ListenAndServeTLS("", "")
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}()
		}
	}

	if config.TcpJsonCommands.Enabled {
		go func() {
			listener, err := net.Listen("tcp", config.TcpJsonCommands.Address)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			defer listener.Close()

			for {
				c, err := listener.Accept()
				if err != nil {
					break
				}

				go func() {
					defer c.Close()
					data := make([]byte, 1024)

					for {
						n, err := c.Read(data)
						if err != nil {
							break
						}

						var cs CommandSlice

						if json.Unmarshal(data[:n], &cs) != nil {
							break
						}

						if len(cs) == 0 {
							break
						}

						if len(config.TcpJsonCommands.HandlerTemplate) == 0 {
							go runCommands(cs)
						} else {
							jsonCommandHandlerChannels[config.TcpJsonCommands.HandlerTemplate] <- &JsonCommandHandlerData{
								Server:   "tcp",
								Address:  c.RemoteAddr().String(),
								Commands: cs,
							}
						}
					}
				}()
			}
		}()
	}

	if config.UdpJsonCommands.Enabled {
		go func() {
			c, err := net.ListenPacket("udp", config.UdpJsonCommands.Address)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
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

				if json.Unmarshal(data[:n], &cs) != nil {
					break
				}

				if len(cs) == 0 {
					break
				}

				if len(config.UdpJsonCommands.HandlerTemplate) == 0 {
					go runCommands(cs)
				} else {
					jsonCommandHandlerChannels[config.UdpJsonCommands.HandlerTemplate] <- &JsonCommandHandlerData{
						Server:   "udp",
						Address:  addr.String(),
						Commands: cs,
					}
				}
			}
		}()
	}

	if config.TlsJsonCommands.Enabled {
		go func() {
			serverCert, err := tls.LoadX509KeyPair(config.HttpServer.Cert, config.HttpServer.Key)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}

			tlsConfig := &tls.Config{Certificates: []tls.Certificate{serverCert}}

			if config.TlsJsonCommands.ClientCa != "" {
				caCert, _ := os.ReadFile(config.TlsJsonCommands.ClientCa)
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCert)
				tlsConfig.ClientCAs = caCertPool

				if config.TlsJsonCommands.RequireClientCert {
					tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
				} else {
					tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
				}
			}

			listener, err := tls.Listen("tcp", config.TlsJsonCommands.Address, tlsConfig)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			defer listener.Close()

			for {
				c, err := listener.Accept()
				if err != nil {
					break
				}

				go func() {
					defer c.Close()

					tlsConn, ok := c.(*tls.Conn)
					if !ok {
						return
					}

					err := tlsConn.Handshake()
					if err != nil {
						return
					}

					var client string
					if config.TlsJsonCommands.ClientCa != "" {
						state := tlsConn.ConnectionState()
						client = tlsClientAuth(config.TlsJsonCommands.Clients, &state)
						if client == " " {
							return
						}
					}

					data := make([]byte, 1024)

					for {
						n, err := c.Read(data)
						if err != nil {
							break
						}

						var cs CommandSlice

						if json.Unmarshal(data[:n], &cs) != nil {
							break
						}

						if len(cs) == 0 {
							break
						}

						if len(config.TlsJsonCommands.HandlerTemplate) == 0 {
							go runCommands(cs)
						} else {
							jsonCommandHandlerChannels[config.TlsJsonCommands.HandlerTemplate] <- &JsonCommandHandlerData{
								Server:    "tls",
								Address:   c.RemoteAddr().String(),
								TlsClient: client,
								Commands:  cs,
							}
						}
					}
				}()
			}
		}()
	}

	if config.StdinJsonCommands.Enabled {
		go func() {
			if stdinDecoder == nil {
				stdinDecoder = json.NewDecoder(os.Stdin)
			}

			for {
				var cs CommandSlice

				err := stdinDecoder.Decode(&cs)
				if err != nil {
					if err == io.EOF {
						break
					}

					stdinDecoder = json.NewDecoder(os.Stdin)
					fmt.Fprintln(os.Stderr, err)
				} else if len(cs) > 0 {
					if len(config.StdinJsonCommands.HandlerTemplate) == 0 {
						runCommands(cs)
					} else {
						jsonCommandHandlerChannels[config.StdinJsonCommands.HandlerTemplate] <- &JsonCommandHandlerData{Commands: cs}
					}
				}
			}
		}()
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt

	select {
	case connectionControlChannel <- "":
	default:
	}
}
