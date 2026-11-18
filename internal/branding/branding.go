package branding

import _ "embed"

//go:embed assets/logo.png
var Logo []byte

//go:embed assets/tray-dark.png
var TrayDark []byte

//go:embed assets/tray-light.png
var TrayLight []byte

//go:embed assets/tray-template.png
var TrayTemplate []byte
