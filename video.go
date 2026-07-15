package main

import (
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

func writeVideoStream(raw bool, newSession func(width int, height int) (io.Writer, http.Flusher)) bool {
	headerBytes := make([]byte, 12)
	var packetSize int
	var packet []byte
	var w io.Writer
	var flusher http.Flusher
	var sessions bool
	var n int
	var err error

	{
		n, err = io.ReadFull(videoSocket, headerBytes[:4])
		if err != nil {
			return false
		}
		if n != 4 {
			return false
		}

		width := int(binary.BigEndian.Uint32(headerBytes))
		var height int

		if width == 0x80000000 {
			n, err = io.ReadFull(videoSocket, headerBytes[:8])
			if err != nil {
				return false
			}
			if n != 8 {
				return false
			}

			width = int(binary.BigEndian.Uint32(headerBytes))
			height = int(binary.BigEndian.Uint32(headerBytes[4:]))
			sessions = true
		} else {
			n, err = io.ReadFull(videoSocket, headerBytes[:4])
			if err != nil {
				return false
			}
			if n != 4 {
				return false
			}

			height = int(binary.BigEndian.Uint32(headerBytes))
		}

		w, flusher = newSession(width, height)
	}

	if raw {
		for {
			n, err = io.ReadFull(videoSocket, headerBytes)
			if err != nil {
				return false
			}
			if n != 12 {
				return false
			}

			if sessions && (headerBytes[0]&0x80) != 0 {
				width := int(binary.BigEndian.Uint32(headerBytes[4:]))
				height := int(binary.BigEndian.Uint32(headerBytes[8:]))
				w, flusher = newSession(width, height)
				continue
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(videoSocket, packet)
			if err != nil {
				return false
			}
			if n != packetSize {
				return false
			}

			n, err = w.Write(packet)
			if err != nil {
				connectionControlChannel <- ""
				break
			}
			if n < packetSize {
				connectionControlChannel <- ""
				break
			}

			if flusher != nil {
				flusher.Flush()
			}
		}
	} else {
		var data []byte

		for {
			n, err = io.ReadFull(videoSocket, headerBytes)
			if err != nil {
				return false
			}
			if n != 12 {
				return false
			}

			if sessions && (headerBytes[0]&0x80) != 0 {
				width := int(binary.BigEndian.Uint32(headerBytes[4:]))
				height := int(binary.BigEndian.Uint32(headerBytes[8:]))
				w, flusher = newSession(width, height)
				continue
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(videoSocket, packet)
			if err != nil {
				return false
			}
			if n != packetSize {
				return false
			}

			data = make([]byte, 12+packetSize)
			copy(data[:12], headerBytes)
			copy(data[12:12+packetSize], packet)

			n, err = w.Write(data)
			if err != nil {
				connectionControlChannel <- ""
				break
			}
			if n < 12+packetSize {
				connectionControlChannel <- ""
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}

	return true
}

func videoStreamHandler(w http.ResponseWriter, req *http.Request) {
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
			w.Header().Set("Access-Control-Expose-Headers", "Device-Name, Codec")
		}

		select {
		case <-videoConnectedChannel:
		case <-req.Context().Done():
			return
		}

		w.Header().Set("Device-Name", deviceName)
		w.Header().Set("Codec", strconv.FormatUint(uint64(videoCodec), 10))

		writeVideoStream(req.URL.Path == "/rawvideostream", func(width int, height int) (io.Writer, http.Flusher) {
			videoMutex.Lock()
			videoWidth = width
			videoHeight = height
			videoMutex.Unlock()

			return w, w.(http.Flusher)
		})
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func decodeVideo() {
	var err error
	var decoder *exec.Cmd
	var decoderStdin io.WriteCloser
	var decoderStdout io.ReadCloser

	for {
		<-videoConnectedChannel

		if decoder != nil {
			decoder.Process.Kill()
			decoder.Wait()
		}

		decoder = exec.Command(
			config.VideoDecoder.Executable,
			strconv.FormatUint(uint64(videoCodec), 10),
			map[bool]string{
				false: "0",
				true:  "1",
			}[config.VideoDecoder.Alpha],
		)

		if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
			decoder.Stderr = os.Stderr
		}

		decoderStdin, err = decoder.StdinPipe()
		if err != nil {
			return
		}

		decoderStdout, err = decoder.StdoutPipe()
		if err != nil {
			return
		}

		err = decoder.Start()
		if err != nil {
			return
		}

		go func() {
			data := make([]byte, 8)
			var n int
			var err error
			var frame []byte
			var frameWidth int
			var frameWidth2 int
			var frameHeight int
			var frameHeight2 int
			var frameSize int
			var frameSize2 int

			for {
				n, err = io.ReadFull(decoderStdout, data)
				if err != nil {
					break
				}
				if n != 8 {
					break
				}

				frameWidth2 = int(binary.NativeEndian.Uint32(data[:4]))
				frameHeight2 = int(binary.NativeEndian.Uint32(data[4:]))

				if frameWidth != frameWidth2 || frameHeight != frameHeight2 {
					frameWidth = frameWidth2
					frameHeight = frameHeight2

					frameSize2 = frameWidth * frameHeight * map[bool]int{
						false: 3,
						true:  4,
					}[config.VideoDecoder.Alpha]

					if frameSize != frameSize2 {
						frame = make([]byte, frameSize2)
						frameSize = frameSize2
					}
				}

				n, err = io.ReadFull(decoderStdout, frame)
				if err != nil {
					break
				}
				if n != frameSize {
					break
				}

				videoMutex.Lock()

				if videoWidth != frameWidth || videoHeight != frameHeight {
					videoWidth = frameWidth
					videoHeight = frameHeight
					videoFrame = make([]byte, frameSize)
				}

				copy(videoFrame, frame)

				videoMutex.Unlock()
			}
		}()

		writeVideoStream(false, func(width int, height int) (io.Writer, http.Flusher) { return decoderStdin, nil })
	}
}

func decodeVideoFfmpeg() {
	var err error
	var ffmpeg *exec.Cmd
	var ffmpegStdin io.WriteCloser
	var ffmpegStdout io.ReadCloser

	newSession := func(width int, height int) (w io.Writer, flusher http.Flusher) {
		videoFrameSize := width * height * map[bool]int{
			false: 3,
			true:  4,
		}[config.VideoDecoder.Alpha]

		videoMutex.Lock()
		videoWidth = width
		videoHeight = height
		if len(videoFrame) != videoFrameSize {
			videoFrame = make([]byte, videoFrameSize)
		}
		videoMutex.Unlock()

		if ffmpeg != nil {
			ffmpeg.Process.Kill()
			ffmpeg.Wait()
		}

		ffmpeg = exec.Command(
			config.VideoDecoder.Executable,
			"-probesize",
			"32",
			"-analyzeduration",
			"0",
			"-re",
			"-f",
			map[uint32]string{
				0x68323634: "h264",
				0x68323635: "hevc",
				0x617631:   "av1",
			}[videoCodec],
			"-i",
			"-",
			"-f",
			"rawvideo",
			"-pix_fmt",
			map[bool]string{
				false: "rgb24",
				true:  "rgba",
			}[config.VideoDecoder.Alpha],
			"-vf",
			func() string {
				if width >= height {
					return "transpose=1:landscape"
				}

				return "transpose=1:portrait"
			}(),
			"-",
		)

		if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
			ffmpeg.Stderr = os.Stderr
		}

		ffmpegStdin, err = ffmpeg.StdinPipe()
		if err != nil {
			return
		}

		ffmpegStdout, err = ffmpeg.StdoutPipe()
		if err != nil {
			return
		}

		err = ffmpeg.Start()
		if err != nil {
			return
		}

		go func() {
			var n int
			var err error

			videoMutex.RLock()
			frame := make([]byte, len(videoFrame))
			videoMutex.RUnlock()

			for {
				n, err = io.ReadFull(ffmpegStdout, frame)
				if err != nil {
					break
				}
				if n != len(frame) {
					break
				}

				videoMutex.Lock()
				copy(videoFrame, frame)
				videoMutex.Unlock()
			}
		}()

		return ffmpegStdin, nil
	}

	for {
		<-videoConnectedChannel

		if !writeVideoStream(true, newSession) && ffmpeg != nil {
			ffmpeg.Process.Kill()
			ffmpeg.Wait()
			ffmpeg = nil
		}
	}
}

func videoFrameHandler(w http.ResponseWriter, req *http.Request) {
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
			w.Header().Set("Access-Control-Expose-Headers", "Device-Name, Width, Height")
		}

		videoMutex.RLock()
		defer videoMutex.RUnlock()

		if len(videoFrame) == 0 {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Device-Name", deviceName)
		w.Header().Set("Width", strconv.Itoa(videoWidth))
		w.Header().Set("Height", strconv.Itoa(videoHeight))
		w.Write(videoFrame)
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
