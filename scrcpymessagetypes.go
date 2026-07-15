package main

var ScrcpyControlMessageTypes = struct {
	InjectKeycode            byte
	InjectText               byte
	InjectTouchEvent         byte
	InjectScrollEvent        byte
	BackOrScreenOn           byte
	ExpandNotificationPanel  byte
	ExpandSettingsPanel      byte
	CollapsePanels           byte
	GetClipboard             byte
	SetClipboard             byte
	SetDisplayPower          byte
	RotateDevice             byte
	UhidCreate               byte
	UhidInput                byte
	OpenHardKeyboardSettings byte
	StartApp                 byte
	ResetVideo               byte
	CameraSetTorch           byte
	CameraZoomIn             byte
	CameraZoomOut            byte
	ResizeDisplay            byte
	ScanFile                 byte
}{
	InjectKeycode:            0x00,
	InjectText:               0x01,
	InjectTouchEvent:         0x02,
	InjectScrollEvent:        0x03,
	BackOrScreenOn:           0x04,
	ExpandNotificationPanel:  0x05,
	ExpandSettingsPanel:      0x06,
	CollapsePanels:           0x07,
	GetClipboard:             0x08,
	SetClipboard:             0x09,
	SetDisplayPower:          0x0A,
	RotateDevice:             0x0B,
	UhidCreate:               0x0C,
	UhidInput:                0x0D,
	OpenHardKeyboardSettings: 0x0F,
	StartApp:                 0x10,
	ResetVideo:               0x11,
	CameraSetTorch:           0x12,
	CameraZoomIn:             0x13,
	CameraZoomOut:            0x14,
	ResizeDisplay:            0x15,
	ScanFile:                 0x16,
}

var ScrcpyDeviceMessageTypes = struct {
	Clipboard    byte
	AckClipboard byte
	UhidOutput   byte
}{
	Clipboard:    0x00,
	AckClipboard: 0x01,
	UhidOutput:   0x02,
}
