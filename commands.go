package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func runCommands(commands CommandSlice) bool {
	for _, command := range commands {
		if len(command) == 0 {
			return false
		}

		cs, ok := config.CustomCommands[command[0]]
		if ok {
			if runCommands(cs) {
				continue
			} else {
				return false
			}
		}

		if !config.Scrcpy.Enabled && command[0] != "sleep" && command[0] != "adb" && command[0] != "adb2" {
			return false
		} else if controlSocket == nil && command[0] != "connect" && command[0] != "startscrcpyserver" && command[0] != "sleep" && command[0] != "adb" && command[0] != "adb2" && command[0] != "setconnectedcommands" {
			return false
		}

		switch command[0] {
		case "connect":
			if len(command) == 1 {
				select {
				case connectionControlChannel <- config.Scrcpy.Address:
				default:
					return false
				}
			} else if len(command) == 2 && config.Scrcpy.Forward {
				select {
				case connectionControlChannel <- command[1]:
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
				case connectionControlChannel <- "":
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
				if len(command) == 2 && strings.HasPrefix(command[1], "[") {
					var options []string
					if json.Unmarshal([]byte(command[1]), &options) != nil {
						return false
					}

					args = append(args, options...)
				} else {
					args = append(args, command[1:]...)
				}
			}

			if scrcpyServer != nil {
				select {
				case connectionControlChannel <- "":
					time.Sleep(1 * time.Second)
				default:
				}

				scrcpyServer.Process.Kill()
				scrcpyServer.Wait()
			}

			scrcpyServer = exec.Command(config.Adb.Executable, args...)

			if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
				scrcpyServer.Stdout = os.Stderr
				scrcpyServer.Stderr = os.Stderr
			}

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
				case connectionControlChannel <- "":
					time.Sleep(1 * time.Second)
				default:
				}

				scrcpyServer.Process.Kill()
				scrcpyServer.Wait()
				scrcpyServer = nil
			} else {
				return false
			}
		case "uhidinput":
			if len(command) == 3 {
				id, err := strconv.Atoi(command[1])
				if err != nil {
					return false
				}

				data, err := hex.DecodeString(command[2])
				if err != nil {
					return false
				}
				if len(data) == 0 {
					return false
				}

				if !uhidInput(id, data) {
					return false
				}
			} else {
				return false
			}
		case "key", "key2":
			if len(command) == 2 {
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

				if !injectKeycode(false, keycode, 0, 0) {
					return false
				}

				if !injectKeycode(true, keycode, 0, 0) {
					return false
				}
			} else {
				return false
			}
		case "key3", "key4":
			if len(command) == 5 {
				var keycode int
				var err error

				if command[0] == "key3" {
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

				if !injectKeycode(up, keycode, repeat, metaState) {
					return false
				}
			} else {
				return false
			}
		case "type":
			if len(command) == 2 {
				if command[1] == "" {
					return false
				}

				if !injectText(command[1]) {
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

				if !injectTouchEvent(0, -2, x, y, width, height, 1) {
					return false
				}

				if !injectTouchEvent(1, -2, x, y, width, height, 1) {
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

				if !injectTouchEvent(0, -2, x, y, width, height, 1) {
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

				if !injectTouchEvent(1, -2, x, y, width, height, 1) {
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

				if !injectTouchEvent(2, -2, x, y, width, height, 1) {
					return false
				}
			} else {
				return false
			}
		case "mouseclick":
			if len(command) == 6 {
				button := getMouseButton(command[1])
				if button == -1 {
					return false
				}

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

				if !injectTouchEvent(0, -1, x, y, width, height, button) {
					return false
				}

				if !injectTouchEvent(1, -1, x, y, width, height, button) {
					return false
				}
			} else {
				return false
			}
		case "mousedown":
			if len(command) == 6 {
				button := getMouseButton(command[1])
				if button == -1 {
					return false
				}

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

				if !injectTouchEvent(0, -1, x, y, width, height, button) {
					return false
				}
			} else {
				return false
			}
		case "mouseup":
			if len(command) == 6 {
				button := getMouseButton(command[1])
				if button == -1 {
					return false
				}

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

				if !injectTouchEvent(1, -1, x, y, width, height, button) {
					return false
				}
			} else {
				return false
			}
		case "mousemove":
			if len(command) == 6 {
				button := getMouseButton(command[1])
				if button == -1 {
					return false
				}

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

				if !injectTouchEvent(2, -1, x, y, width, height, button) {
					return false
				}
			} else {
				return false
			}
		case "scrollleft", "scrollright", "scrollup", "scrolldown":
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

				if !injectScrollEvent(x, y, width, height, command[0][6:]) {
					return false
				}
			} else {
				return false
			}
		case "openhardkeyboardsettings":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.OpenHardKeyboardSettings})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.BackOrScreenOn, 0x00, ScrcpyControlMessageTypes.BackOrScreenOn, 0x01})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.ExpandNotificationPanel})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.ExpandSettingsPanel})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.CollapsePanels})
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
				if !getClipboard(command[0] == "getclipboardcut") {
					return false
				}
			} else {
				return false
			}
		case "setclipboard", "setclipboardpaste":
			if len(command) == 2 || len(command) == 3 || len(command) == 4 {
				var sequence int
				var timeout time.Duration
				var err error

				if len(command) > 2 {
					sequence, err = strconv.Atoi(command[2])
					if err != nil {
						return false
					}

					if len(command) == 4 {
						timeout, err = time.ParseDuration(command[3])
						if err != nil {
							return false
						}
					}
				}

				if !setClipboard(command[1], sequence, command[0] == "setclipboardpaste", timeout) {
					return false
				}
			} else {
				return false
			}
		case "turnscreenon":
			if len(command) == 1 {
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.SetDisplayPower, 0x02})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.SetDisplayPower, 0x00})
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.RotateDevice})
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
				data[0] = ScrcpyControlMessageTypes.StartApp
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
				n, err := controlSocket.Write([]byte{ScrcpyControlMessageTypes.ResetVideo})
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
			if len(command) == 2 && config.Adb.Enabled && (command[1] == "connect" || command[1] == "disconnect") {
				args := append(config.Adb.Options, command[1], config.Adb.Device)

				cmd := exec.Command(config.Adb.Executable, args...)

				if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
					cmd.Stdout = os.Stderr
					cmd.Stderr = os.Stderr
				}

				if cmd.Run() != nil && command[0] == "adb" {
					return false
				}
			} else if len(command) > 1 && config.Adb.Enabled {
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

				if !config.Scrcpy.StderrClipboard && !config.Scrcpy.StderrUhidOutput {
					cmd.Stdout = os.Stderr
					cmd.Stderr = os.Stderr
				}

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
