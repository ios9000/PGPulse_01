//go:build desktop

package desktop

import _ "embed"

//go:embed icons/pgpulse-tray.png
var IconTrayDefault []byte

//go:embed icons/pgpulse-tray-warning.png
var IconTrayWarning []byte

//go:embed icons/pgpulse-tray-critical.png
var IconTrayCritical []byte

//go:embed icons/pgpulse.ico
var IconWindow []byte
