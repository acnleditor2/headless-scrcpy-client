package main

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func getClipboard(cut bool) bool {
	data := make([]byte, 2)
	data[0] = ScrcpyControlMessageTypes.GetClipboard
	if cut {
		data[1] = 0x02
	} else {
		data[1] = 0x01
	}

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != 2 {
		return false
	}

	return true
}

func setClipboard(text string, sequence int, paste bool, timeout time.Duration) bool {
	data := make([]byte, 14+len(text))
	data[0] = ScrcpyControlMessageTypes.SetClipboard
	binary.BigEndian.PutUint64(data[1:], uint64(sequence))
	if paste {
		data[9] = 0x01
	}
	binary.BigEndian.PutUint32(data[10:], uint32(len(text)))
	copy(data[14:], []byte(text))

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != len(data) {
		return false
	}

	if timeout > 0 {
		select {
		case s := <-clipboardChannel:
			if s != strconv.Itoa(sequence) {
				return false
			}
		case <-time.After(timeout):
			return false
		}
	}

	return true
}

func clipboardHandler(w http.ResponseWriter, req *http.Request) {
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

		if controlSocket == nil {
			http.NotFound(w, req)
			return
		}

		if !getClipboard(req.URL.Path == "/clipboardcut") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		select {
		case s := <-clipboardChannel:
			if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
				w.Write([]byte(s))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusInternalServerError)
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

func setClipboardHandler(w http.ResponseWriter, req *http.Request) {
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

		query := req.URL.Query()
		sequenceString := query.Get("sequence")
		var sequence int
		var timeout time.Duration
		var err error

		if sequenceString != "" {
			sequence, err = strconv.Atoi(sequenceString)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			timeout = 2 * time.Second
		}

		if !setClipboard(query.Get("text"), sequence, req.URL.Path == "/setclipboardpaste", timeout) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

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

func clipboardStreamHandler(w http.ResponseWriter, req *http.Request) {
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

		var err error

		for {
			select {
			case line := <-clipboardChannel:
				_, err = fmt.Fprintln(w, line)
				if err != nil {
					return
				}

				w.(http.Flusher).Flush()
			case <-req.Context().Done():
				return
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
