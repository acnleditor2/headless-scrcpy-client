package main

import (
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

func videoStreamHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if config.HttpServer.ClientAuthCa != "" && tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS) == "" {
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
			w.Header().Set("Access-Control-Expose-Headers", "Device-Name, Codec, Initial-Width, Initial-Height")
		}

		select {
		case <-videoConnectedChannel:
		case <-req.Context().Done():
			return
		}

		w.Header().Set("Device-Name", deviceName)
		w.Header().Set("Codec", strconv.FormatUint(uint64(videoCodec), 10))
		w.Header().Set("Initial-Width", strconv.Itoa(initialVideoWidth))
		w.Header().Set("Initial-Height", strconv.Itoa(initialVideoHeight))

		headerBytes := make([]byte, 12)
		var packetSize int
		var packet []byte
		var n int
		var err error

		if req.URL.Path == "/videostream" {
			var data []byte

			for {
				n, err = io.ReadFull(videoSocket, headerBytes)
				if err != nil {
					break
				}
				if n != 12 {
					break
				}

				packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
				packet = make([]byte, packetSize)

				n, err = io.ReadFull(videoSocket, packet)
				if err != nil {
					break
				}
				if n != packetSize {
					break
				}

				data = make([]byte, 12+packetSize)
				copy(data[:12], headerBytes)
				copy(data[12:12+packetSize], packet)

				n, err = w.Write(data)
				if err != nil {
					connectionControlChannel <- false
					break
				}
				if n < 12+packetSize {
					connectionControlChannel <- false
					break
				}

				w.(http.Flusher).Flush()
			}
		} else {
			for {
				n, err = io.ReadFull(videoSocket, headerBytes)
				if err != nil {
					break
				}
				if n != 12 {
					break
				}

				packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
				packet = make([]byte, packetSize)

				n, err = io.ReadFull(videoSocket, packet)
				if err != nil {
					break
				}
				if n != packetSize {
					break
				}

				n, err = w.Write(packet)
				if err != nil {
					connectionControlChannel <- false
					break
				}
				if n < packetSize {
					connectionControlChannel <- false
					break
				}

				w.(http.Flusher).Flush()
			}
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

		decoder.Stderr = os.Stderr

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

				videoFrameMutex.Lock()

				if videoFrameWidth != frameWidth || videoFrameHeight != frameHeight {
					videoFrameWidth = frameWidth
					videoFrameHeight = frameHeight
					videoFrame = make([]byte, frameSize)
				}

				copy(videoFrame, frame)

				videoFrameMutex.Unlock()
			}
		}()

		headerBytes := make([]byte, 12)
		var n int
		var packetSize int
		var packet []byte
		var data []byte

		for {
			n, err = io.ReadFull(videoSocket, headerBytes)
			if err != nil {
				break
			}
			if n != 12 {
				break
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(videoSocket, packet)
			if err != nil {
				break
			}
			if n != packetSize {
				break
			}

			data = make([]byte, 12+packetSize)
			copy(data[:12], headerBytes)
			copy(data[12:12+packetSize], packet)

			n, err = decoderStdin.Write(data)
			if err != nil {
				connectionControlChannel <- false
				break
			}
			if n < 12+packetSize {
				connectionControlChannel <- false
				break
			}
		}
	}
}

func decodeVideoFfmpeg() {
	var err error
	var ffmpeg *exec.Cmd
	var ffmpegStdin io.WriteCloser
	var ffmpegStdout io.ReadCloser

	for {
		<-videoConnectedChannel

		videoFrameSize := initialVideoWidth * initialVideoHeight * map[bool]int{
			false: 3,
			true:  4,
		}[config.VideoDecoder.Alpha]

		videoFrameMutex.Lock()
		videoFrameWidth = initialVideoWidth
		videoFrameHeight = initialVideoHeight
		if len(videoFrame) != videoFrameSize {
			videoFrame = make([]byte, videoFrameSize)
		}
		videoFrameMutex.Unlock()

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
				if initialVideoWidth >= initialVideoHeight {
					return "transpose=1:landscape"
				}

				return "transpose=1:portrait"
			}(),
			"-",
		)

		ffmpeg.Stderr = os.Stderr

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

			videoFrameMutex.RLock()
			frame := make([]byte, len(videoFrame))
			videoFrameMutex.RUnlock()

			for {
				n, err = io.ReadFull(ffmpegStdout, frame)
				if err != nil {
					break
				}
				if n != len(frame) {
					break
				}

				videoFrameMutex.Lock()
				copy(videoFrame, frame)
				videoFrameMutex.Unlock()
			}
		}()

		headerBytes := make([]byte, 12)
		var n int
		var packetSize int
		var packet []byte

		for {
			n, err = io.ReadFull(videoSocket, headerBytes)
			if err != nil {
				ffmpeg.Process.Kill()
				ffmpeg.Wait()
				ffmpeg = nil
				break
			}
			if n != 12 {
				ffmpeg.Process.Kill()
				ffmpeg.Wait()
				ffmpeg = nil
				break
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(videoSocket, packet)
			if err != nil {
				ffmpeg.Process.Kill()
				ffmpeg.Wait()
				ffmpeg = nil
				break
			}
			if n != packetSize {
				ffmpeg.Process.Kill()
				ffmpeg.Wait()
				ffmpeg = nil
				break
			}

			n, err = ffmpegStdin.Write(packet)
			if err != nil {
				connectionControlChannel <- false
				break
			}
			if n < packetSize {
				connectionControlChannel <- false
				break
			}
		}
	}
}

func videoFrameHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if config.HttpServer.ClientAuthCa != "" && tlsClientAuth(config.HttpServer.Endpoints[req.URL.Path], req.TLS) == "" {
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

		videoFrameMutex.RLock()
		defer videoFrameMutex.RUnlock()

		if len(videoFrame) == 0 {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Device-Name", deviceName)
		w.Header().Set("Width", strconv.Itoa(videoFrameWidth))
		w.Header().Set("Height", strconv.Itoa(videoFrameHeight))
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
