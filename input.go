package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
)

var keycodeMap = map[string]int{
	"0":              7,
	"1":              8,
	"2":              9,
	"3":              10,
	"4":              11,
	"5":              12,
	"6":              13,
	"7":              14,
	"8":              15,
	"9":              16,
	"a":              29,
	"b":              30,
	"c":              31,
	"d":              32,
	"e":              33,
	"f":              34,
	"g":              35,
	"h":              36,
	"i":              37,
	"j":              38,
	"k":              39,
	"l":              40,
	"m":              41,
	"n":              42,
	"o":              43,
	"p":              44,
	"q":              45,
	"r":              46,
	"s":              47,
	"t":              48,
	"u":              49,
	"v":              50,
	"w":              51,
	"x":              52,
	"y":              53,
	"z":              54,
	" ":              62,
	"#":              18,
	"'":              75,
	"(":              162,
	")":              163,
	"*":              17,
	"+":              81,
	",":              55,
	"-":              69,
	".":              56,
	"/":              76,
	";":              74,
	"=":              70,
	"@":              77,
	"[":              71,
	"\\":             73,
	"]":              72,
	"`":              68,
	"\n":             66,
	"\t":             61,
	"home":           3,
	"back":           4,
	"up":             19,
	"down":           20,
	"left":           21,
	"right":          22,
	"volumeup":       24,
	"volumedown":     25,
	"power":          26,
	"backspace":      67,
	"menu":           82,
	"mediaplaypause": 85,
	"mediastop":      86,
	"medianext":      87,
	"mediaprevious":  88,
	"pageup":         92,
	"pagedown":       93,
	"escape":         111,
	"delete":         112,
	"movehome":       122,
	"moveend":        123,
	"insert":         124,
	"numpad0":        144,
	"numpad1":        145,
	"numpad2":        146,
	"numpad3":        147,
	"numpad4":        148,
	"numpad5":        149,
	"numpad6":        150,
	"numpad7":        151,
	"numpad8":        152,
	"numpad9":        153,
	"numpaddivide":   154,
	"numpadmultiply": 155,
	"numpadsubtract": 156,
	"numpadadd":      157,
	"numpaddot":      158,
	"numpadenter":    160,
	"numpadequals":   161,
	"appswitch":      187,
	"assist":         219,
	"brightnessdown": 220,
	"brightnessup":   221,
	"sleep":          223,
	"wakeup":         224,
	"voiceassist":    231,
	"allapps":        284,
}

func injectKeycode(up bool, keycode int, repeat int, metaState int) bool {
	data := make([]byte, 14)
	data[0] = ScrcpyControlMessageTypes.InjectKeycode
	if up {
		data[1] = 0x01
	}
	binary.BigEndian.PutUint32(data[2:6], uint32(keycode))
	binary.BigEndian.PutUint32(data[6:10], uint32(repeat))
	binary.BigEndian.PutUint32(data[10:], uint32(metaState))

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != 14 {
		return false
	}

	return true
}

func injectText(text string) bool {
	data := make([]byte, 5+len(text))
	data[0] = ScrcpyControlMessageTypes.InjectText
	binary.BigEndian.PutUint32(data[1:5], uint32(len(text)))
	copy(data[5:], []byte(text))

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != len(data) {
		return false
	}

	return true
}

func injectTouchEvent(action int, pointerId int, x int, y int, width int, height int, button int) bool {
	data := make([]byte, 32)
	data[0] = ScrcpyControlMessageTypes.InjectTouchEvent
	data[1] = byte(action)
	binary.BigEndian.PutUint64(data[2:], uint64(pointerId))
	binary.BigEndian.PutUint32(data[10:], uint32(x))
	binary.BigEndian.PutUint32(data[14:], uint32(y))
	binary.BigEndian.PutUint16(data[18:], uint16(width))
	binary.BigEndian.PutUint16(data[20:], uint16(height))
	if action != 1 {
		data[22] = 0xFF
		data[23] = 0xFF
	}
	binary.BigEndian.PutUint32(data[24:], uint32(button))
	if action != 1 {
		binary.BigEndian.PutUint32(data[28:], uint32(button))
	}

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != 32 {
		return false
	}

	return true
}

func injectScrollEvent(x int, y int, width int, height int, direction string) bool {
	data := make([]byte, 21)
	data[0] = ScrcpyControlMessageTypes.InjectScrollEvent
	binary.BigEndian.PutUint32(data[1:], uint32(x))
	binary.BigEndian.PutUint32(data[5:], uint32(y))
	binary.BigEndian.PutUint16(data[9:], uint16(width))
	binary.BigEndian.PutUint16(data[11:], uint16(height))
	switch direction {
	case "left":
		data[13] = 0x80
	case "right":
		data[13] = 0x7F
		data[14] = 0xFF
	case "up":
		data[15] = 0x7F
		data[16] = 0xFF
	case "down":
		data[15] = 0x80
	}

	n, err := controlSocket.Write(data)
	if err != nil {
		return false
	}
	if n != 21 {
		return false
	}

	return true
}

func createUhidDevices() bool {
	for i := range config.Scrcpy.UhidDevices {
		reportDesc, err := hex.DecodeString(config.Scrcpy.UhidDevices[i].ReportDesc)
		if err != nil {
			return false
		}

		var b bytes.Buffer

		b.WriteByte(ScrcpyControlMessageTypes.UhidCreate)
		binary.Write(&b, binary.BigEndian, uint16(config.Scrcpy.UhidDevices[i].Id))
		if config.Scrcpy.UhidDevices[i].VendorId == "" || config.Scrcpy.UhidDevices[i].ProductId == "" {
			binary.Write(&b, binary.BigEndian, uint32(0))
		} else if len(config.Scrcpy.UhidDevices[i].VendorId) == 4 && len(config.Scrcpy.UhidDevices[i].ProductId) == 4 {
			vendorId, err := strconv.ParseUint(config.Scrcpy.UhidDevices[i].VendorId, 16, 16)
			if err != nil {
				return false
			}

			productId, err := strconv.ParseUint(config.Scrcpy.UhidDevices[i].ProductId, 16, 16)
			if err != nil {
				return false
			}

			binary.Write(&b, binary.BigEndian, uint16(vendorId))
			binary.Write(&b, binary.BigEndian, uint16(productId))
		}
		b.WriteByte(byte(len(config.Scrcpy.UhidDevices[i].Name)))
		if config.Scrcpy.UhidDevices[i].Name != "" {
			b.WriteString(config.Scrcpy.UhidDevices[i].Name)
		}
		binary.Write(&b, binary.BigEndian, uint16(len(reportDesc)))
		b.Write(reportDesc)

		_, err = b.WriteTo(controlSocket)
		if err != nil {
			return false
		}
	}

	return true
}

func uhidInput(id int, data []byte) bool {
	var b bytes.Buffer

	b.WriteByte(ScrcpyControlMessageTypes.UhidInput)
	binary.Write(&b, binary.BigEndian, uint16(id))
	binary.Write(&b, binary.BigEndian, uint16(len(data)))
	b.Write(data)

	_, err := b.WriteTo(controlSocket)
	if err != nil {
		return false
	}

	return true
}

func getMouseButton(buttonString string) int {
	switch buttonString {
	case "1", "left":
		return 1
	case "2", "right":
		return 2
	case "4", "middle":
		return 4
	default:
		return -1
	}
}

func keyHandler(w http.ResponseWriter, req *http.Request) {
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
		var keycode int

		if query.Has("keycode") {
			var err error
			keycode, err = strconv.Atoi(query.Get("keycode"))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		} else {
			keycode = keycodeMap[query.Get("key")]
			if keycode == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		switch req.URL.Path {
		case "/key":
			if !injectKeycode(false, keycode, 0, 0) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if !injectKeycode(true, keycode, 0, 0) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/keydown":
			if !injectKeycode(false, keycode, 0, 0) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/keyup":
			if !injectKeycode(true, keycode, 0, 0) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
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

func typeHandler(w http.ResponseWriter, req *http.Request) {
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

		text := req.URL.Query().Get("text")
		if text == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if !injectText(text) {
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

func touchHandler(w http.ResponseWriter, req *http.Request) {
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

		x, err := strconv.Atoi(query.Get("x"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		y, err := strconv.Atoi(query.Get("y"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		width, err := strconv.Atoi(query.Get("w"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		height, err := strconv.Atoi(query.Get("h"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch req.URL.Path {
		case "/touch":
			if !injectTouchEvent(0, -2, x, y, width, height, 1) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if !injectTouchEvent(1, -2, x, y, width, height, 1) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/touchdown":
			if !injectTouchEvent(0, -2, x, y, width, height, 1) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/touchup":
			if !injectTouchEvent(1, -2, x, y, width, height, 1) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/touchmove":
			if !injectTouchEvent(2, -2, x, y, width, height, 1) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
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

func mouseHandler(w http.ResponseWriter, req *http.Request) {
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

		button := getMouseButton(query.Get("button"))
		if button == -1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		x, err := strconv.Atoi(query.Get("x"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		y, err := strconv.Atoi(query.Get("y"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		width, err := strconv.Atoi(query.Get("w"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		height, err := strconv.Atoi(query.Get("h"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch req.URL.Path {
		case "/mouseclick":
			if !injectTouchEvent(0, -1, x, y, width, height, button) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if !injectTouchEvent(1, -1, x, y, width, height, button) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/mousedown":
			if !injectTouchEvent(0, -1, x, y, width, height, button) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/mouseup":
			if !injectTouchEvent(1, -1, x, y, width, height, button) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "/mousemove":
			if !injectTouchEvent(2, -1, x, y, width, height, button) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
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

func scrollHandler(w http.ResponseWriter, req *http.Request) {
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

		x, err := strconv.Atoi(query.Get("x"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		y, err := strconv.Atoi(query.Get("y"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		width, err := strconv.Atoi(query.Get("w"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		height, err := strconv.Atoi(query.Get("h"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if !injectScrollEvent(x, y, width, height, req.URL.Path[7:]) {
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

func uhidInputHandler(w http.ResponseWriter, req *http.Request) {
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

		id, err := strconv.Atoi(query.Get("id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		data, err := hex.DecodeString(query.Get("data"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(data) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if !uhidInput(id, data) {
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

func uhidOutputStreamHandler(w http.ResponseWriter, req *http.Request) {
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
			case line := <-uhidOutputChannel:
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
