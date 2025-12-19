package main

import (
	"encoding/binary"
	"io"
	"net/http"
	"strconv"
)

func writeAudioStream(raw bool, w io.Writer, flusher http.Flusher) {
	headerBytes := make([]byte, 12)
	var packetSize int
	var packet []byte
	var n int
	var err error

	if raw {
		for {
			n, err = io.ReadFull(audioSocket, headerBytes)
			if err != nil {
				break
			}
			if n != 12 {
				break
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(audioSocket, packet)
			if err != nil {
				break
			}
			if n != packetSize {
				break
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
			n, err = io.ReadFull(audioSocket, headerBytes)
			if err != nil {
				break
			}
			if n != 12 {
				break
			}

			packetSize = int(binary.BigEndian.Uint32(headerBytes[8:]))
			packet = make([]byte, packetSize)

			n, err = io.ReadFull(audioSocket, packet)
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
}

func audioStreamHandler(w http.ResponseWriter, req *http.Request) {
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
		case <-audioConnectedChannel:
		case <-req.Context().Done():
			return
		}

		w.Header().Set("Device-Name", deviceName)
		w.Header().Set("Codec", strconv.FormatUint(uint64(audioCodec), 10))
		writeAudioStream(req.URL.Path == "/rawaudiostream", w, w.(http.Flusher))
	default:
		if origin != "" {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Allow", "OPTIONS, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
