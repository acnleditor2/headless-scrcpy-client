package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func commandsRun(commands CommandSlice) bool {
	for _, command := range commands {
		if len(command) == 0 {
			return false
		}

		cs, ok := config.CustomCommands[command[0]]
		if ok {
			if commandsRun(cs) {
				continue
			} else {
				return false
			}
		}

		if config.Scrcpy.Port < 1 {
			if command[0] != "sleep" && command[0] != "adb" && command[0] != "adb2" {
				return false
			}
		} else if controlSocket == nil {
			if command[0] != "connect" && command[0] != "startscrcpyserver" && command[0] != "sleep" && command[0] != "adb" && command[0] != "adb2" && command[0] != "setconnectedcommands" {
				return false
			}
		}

		switch command[0] {
		case "connect":
			if len(command) == 1 {
				select {
				case connectionControlChannel <- true:
				default:
					return false
				}
			} else {
				return false
			}
		case "disconnect":
			if len(command) == 1 {
				if scrcpyServer != nil {
					return false
				}

				select {
				case connectionControlChannel <- false:
				default:
					return false
				}
			} else {
				return false
			}
		case "startscrcpyserver":
			if !config.Adb.Enabled || !config.Scrcpy.Enabled {
				return false
			}

			if scrcpyServer != nil {
				select {
				case connectionControlChannel <- false:
					time.Sleep(1 * time.Second)
				default:
				}

				scrcpyServer.Process.Kill()
				scrcpyServer.Wait()
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
			)

			if !config.Scrcpy.Video {
				args = append(args, "video=false")
			}

			if !config.Scrcpy.Audio {
				args = append(args, "audio=false")
			}

			if config.Scrcpy.Control {
				if !config.Scrcpy.ClipboardAutosync {
					args = append(args, "clipboard_autosync=false")
				}
			} else {
				args = append(args, "control=false")
			}

			if !config.Scrcpy.Cleanup {
				args = append(args, "cleanup=false")
			}

			if !config.Scrcpy.PowerOn {
				args = append(args, "power_on=false")
			}

			if config.Scrcpy.Forward {
				args = append(args, "tunnel_forward=true")
			}

			if len(config.Scrcpy.ServerOptions) > 0 {
				args = append(args, config.Scrcpy.ServerOptions...)
			}

			if len(command) > 1 {
				args = append(args, command[1:]...)
			}

			scrcpyServer = exec.Command(config.Adb.Executable, args...)
			scrcpyServer.Stdout = os.Stderr
			scrcpyServer.Stderr = os.Stderr

			if scrcpyServer.Start() != nil {
				scrcpyServer = nil
				return false
			}
		case "stopscrcpyserver":
			if len(command) == 1 {
				if scrcpyServer == nil {
					return false
				}

				select {
				case connectionControlChannel <- false:
					time.Sleep(1 * time.Second)
				default:
				}

				scrcpyServer.Process.Kill()
				scrcpyServer.Wait()
				scrcpyServer = nil
			} else {
				return false
			}
		case "createuhiddevices":
			if len(command) == 4 {
				if command[1] != "" {
					if !inputUhidCreateDevice(command[1], 0x01, "", "", "", controlSocket) {
						return false
					}
				}

				if command[2] != "" {
					if !inputUhidCreateDevice(command[2], 0x02, "", "", "", controlSocket) {
						return false
					}
				}

				if command[3] != "" {
					if !inputUhidCreateDevice(command[3], 0x03, "", "", "", controlSocket) {
						return false
					}
				}
			} else if len(command) == 13 {
				if command[1] != "" {
					if !inputUhidCreateDevice(command[1], 0x01, command[2], command[3], command[4], controlSocket) {
						return false
					}
				}

				if command[5] != "" {
					if !inputUhidCreateDevice(command[5], 0x02, command[6], command[7], command[8], controlSocket) {
						return false
					}
				}

				if command[9] != "" {
					if !inputUhidCreateDevice(command[9], 0x03, command[10], command[11], command[12], controlSocket) {
						return false
					}
				}
			} else {
				return false
			}
		case "key", "key2":
			if len(command) == 2 || len(command) == 5 {
				var keycode int
				var err error

				if command[0] == "key" {
					keycode = keycodeMap[command[1]]
					if keycode == 0 {
						return false
					}
				} else {
					keycode, err = strconv.Atoi(command[1])
					if err != nil {
						return false
					}
				}

				if len(command) == 2 {
					if !inputSdkInjectKeycode(false, keycode, 0, 0) {
						return false
					}

					if !inputSdkInjectKeycode(true, keycode, 0, 0) {
						return false
					}
				} else {
					up, err := strconv.ParseBool(command[2])
					if err != nil {
						return false
					}

					repeat, err := strconv.Atoi(command[3])
					if err != nil {
						return false
					}

					metaState, err := strconv.Atoi(command[4])
					if err != nil {
						return false
					}

					if !inputSdkInjectKeycode(up, keycode, repeat, metaState) {
						return false
					}
				}
			} else {
				return false
			}
		case "key3":
			if len(command) == 2 || len(command) == 3 {
				scancode, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				if len(command) == 2 {
					if !inputUhidKeyboardInput(scancode, 0) {
						return false
					}

					if scancode != 0 {
						if !inputUhidKeyboardInput(0, 0) {
							return false
						}
					}
				} else {
					modifiers, err := strconv.Atoi(command[2])
					if err != nil {
						return false
					}

					if !inputUhidKeyboardInput(scancode, modifiers) {
						return false
					}
				}
			} else {
				return false
			}
		case "type", "typebase64", "typebase64url", "typehex":
			if len(command) == 2 {
				if command[1] == "" {
					return false
				}

				var text string

				if command[0] == "typebase64" {
					textBytes, err := base64.StdEncoding.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(textBytes)
				} else if command[0] == "typebase64url" {
					textBytes, err := base64.URLEncoding.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(textBytes)
				} else if command[0] == "typehex" {
					textBytes, err := hex.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(textBytes)
				} else {
					text = command[1]
				}

				if !inputSdkInjectText(text) {
					return false
				}
			} else {
				return false
			}
		case "touch":
			if len(command) == 5 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(0, -2, x, y, width, height, 1) {
					return false
				}

				if !inputSdkInjectTouchEvent(1, -2, x, y, width, height, 1) {
					return false
				}
			} else {
				return false
			}
		case "touchdown":
			if len(command) == 5 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(0, -2, x, y, width, height, 1) {
					return false
				}
			} else {
				return false
			}
		case "touchup":
			if len(command) == 5 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(1, -2, x, y, width, height, 1) {
					return false
				}
			} else {
				return false
			}
		case "touchmove":
			if len(command) == 5 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(2, -2, x, y, width, height, 1) {
					return false
				}
			} else {
				return false
			}
		case "mouseclick":
			if len(command) == 4 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				if !inputUhidMouseInput(inputGetMouseButton(command[1]), x, y, "") {
					return false
				}

				if !inputUhidMouseInput(0, 0, 0, "") {
					return false
				}
			} else if len(command) == 6 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[5])
				if err != nil {
					return false
				}

				button := inputGetMouseButton(command[1])

				if !inputSdkInjectTouchEvent(0, -1, x, y, width, height, button) {
					return false
				}

				if !inputSdkInjectTouchEvent(1, -1, x, y, width, height, button) {
					return false
				}
			} else {
				return false
			}
		case "mousedown":
			if len(command) == 4 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				if !inputUhidMouseInput(inputGetMouseButton(command[1]), x, y, "") {
					return false
				}
			} else if len(command) == 6 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[5])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(0, -1, x, y, width, height, inputGetMouseButton(command[1])) {
					return false
				}
			} else {
				return false
			}
		case "mouseup":
			if len(command) == 1 {
				if !inputUhidMouseInput(0, 0, 0, "") {
					return false
				}
			} else if len(command) == 6 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[5])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(1, -1, x, y, width, height, inputGetMouseButton(command[1])) {
					return false
				}
			} else {
				return false
			}
		case "mousemove":
			if len(command) == 3 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				if !inputUhidMouseInput(0, x, y, "") {
					return false
				}
			} else if len(command) == 4 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				if !inputUhidMouseInput(inputGetMouseButton(command[1]), x, y, "") {
					return false
				}
			} else if len(command) == 6 {
				x, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[5])
				if err != nil {
					return false
				}

				if !inputSdkInjectTouchEvent(2, -1, x, y, width, height, inputGetMouseButton(command[1])) {
					return false
				}
			} else {
				return false
			}
		case "scrollleft", "scrollright", "scrollup", "scrolldown":
			if len(command) == 1 && (command[0] == "scrollup" || command[0] == "scrolldown") {
				if !inputUhidMouseInput(0, 0, 0, command[0][6:]) {
					return false
				}
			} else if len(command) == 5 {
				x, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				y, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				width, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				height, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				if !inputSdkInjectScrollEvent(x, y, width, height, command[0][6:]) {
					return false
				}
			} else {
				return false
			}
		case "gamepadinput":
			if len(command) == 9 {
				leftX, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				leftY, err := strconv.Atoi(command[2])
				if err != nil {
					return false
				}

				rightX, err := strconv.Atoi(command[3])
				if err != nil {
					return false
				}

				rightY, err := strconv.Atoi(command[4])
				if err != nil {
					return false
				}

				leftTrigger, err := strconv.Atoi(command[5])
				if err != nil {
					return false
				}

				rightTrigger, err := strconv.Atoi(command[6])
				if err != nil {
					return false
				}

				buttons, err := strconv.Atoi(command[7])
				if err != nil {
					return false
				}

				dpad, err := strconv.Atoi(command[8])
				if err != nil {
					return false
				}

				if !inputUhidGamepadInput(leftX, leftY, rightX, rightY, leftTrigger, rightTrigger, buttons, dpad) {
					return false
				}
			} else {
				return false
			}
		case "openhardkeyboardsettings":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x0F})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "backorscreenon":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x04, 0x00, 0x04, 0x01})
				if err != nil {
					return false
				}
				if n != 4 {
					return false
				}
			} else {
				return false
			}
		case "expandnotificationspanel":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x05})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "expandsettingspanel":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x06})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "collapsepanels":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x07})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "getclipboard", "getclipboardcut":
			if len(command) == 1 {
				if clipboardGet(command[0] == "getclipboardcut", nil, 0) != http.StatusNoContent {
					return false
				}
			} else {
				return false
			}
		case "setclipboard", "setclipboardbase64", "setclipboardbase64url", "setclipboardhex", "setclipboardpaste", "setclipboardpastebase64", "setclipboardpastebase64url", "setclipboardpastehex":
			if len(command) == 2 || len(command) == 3 || len(command) == 4 {
				var text string

				if strings.HasSuffix(command[0], "base64") {
					decoded, err := base64.StdEncoding.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(decoded)
				} else if strings.HasSuffix(command[0], "base64url") {
					decoded, err := base64.URLEncoding.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(decoded)
				} else if strings.HasSuffix(command[0], "hex") {
					decoded, err := hex.DecodeString(command[1])
					if err != nil {
						return false
					}
					text = string(decoded)
				} else {
					text = command[1]
				}

				var sequenceString string
				var timeout time.Duration
				var err error

				if len(command) > 2 {
					sequenceString = command[2]

					if len(command) == 4 {
						timeout, err = time.ParseDuration(command[3])
						if err != nil {
							return false
						}
					}
				}

				if !clipboardSet(text, sequenceString, strings.HasPrefix(command[0], "setclipboardpaste"), timeout) {
					return false
				}
			} else {
				return false
			}
		case "turnscreenon":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x0A, 0x02})
				if err != nil {
					return false
				}
				if n != 2 {
					return false
				}
			} else {
				return false
			}
		case "turnscreenoff":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x0A, 0x00})
				if err != nil {
					return false
				}
				if n != 2 {
					return false
				}
			} else {
				return false
			}
		case "rotate":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x0B})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "startapp":
			if len(command) == 2 {
				data := make([]byte, 2+len(command[1]))
				data[0] = 0x10
				data[1] = byte(len(command[1]))
				copy(data[2:], []byte(command[1]))

				n, err := controlSocket.Write(data)
				if err != nil {
					return false
				}
				if n != len(data) {
					return false
				}
			} else {
				return false
			}
		case "resetvideo":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{0x11})
				if err != nil {
					return false
				}
				if n != 1 {
					return false
				}
			} else {
				return false
			}
		case "senddata":
			if len(command) == 2 {
				data, err := hex.DecodeString(command[1])
				if err != nil {
					return false
				}
				if len(data) == 0 {
					return false
				}

				n, err := controlSocket.Write(data)
				if err != nil {
					return false
				}
				if n != len(data) {
					return false
				}
			} else {
				return false
			}
		case "sleep":
			if len(command) == 2 {
				duration, err := time.ParseDuration(command[1])
				if err != nil {
					return false
				}

				time.Sleep(duration)
			} else {
				return false
			}
		case "adb", "adb2":
			if len(command) == 2 && config.Adb.Enabled && config.Adb.Executable != "" && (command[1] == "connect" || command[1] == "disconnect") {
				args := append(config.Adb.Options, command[1], config.Adb.Device)

				cmd := exec.Command(config.Adb.Executable, args...)
				cmd.Stdout = os.Stderr
				cmd.Stderr = os.Stderr

				if cmd.Run() != nil && command[0] == "adb" {
					return false
				}
			} else if len(command) > 1 && config.Adb.Enabled && config.Adb.Executable != "" {
				var args []string
				if config.Adb.Device == "usb" {
					args = append(config.Adb.Options, "-d")
				} else if config.Adb.Device == "tcpip" {
					args = append(config.Adb.Options, "-e")
				} else if config.Adb.Device != "" {
					args = append(config.Adb.Options, "-s", config.Adb.Device)
				}

				args = append(args, command[1:]...)

				cmd := exec.Command(config.Adb.Executable, args...)
				cmd.Stdout = os.Stderr
				cmd.Stderr = os.Stderr

				if cmd.Run() != nil && command[0] == "adb" {
					return false
				}
			} else {
				return false
			}
		case "setconnectedcommands":
			if len(command) == 2 {
				defer func(commands string) {
					json.Unmarshal([]byte(commands), &scrcpyConnectedCommands)
				}(command[1])
			} else {
				return false
			}
		}
	}

	return true
}
