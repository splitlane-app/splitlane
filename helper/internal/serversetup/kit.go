package serversetup

import _ "embed"

// Kit is the server kit, scripts/server/install-singbox-server.sh, built into the helper: a setup
// uploads the kit of its own version, never one downloaded at the time. kit/ holds a copy because
// go:embed reads only below the package; TestKitIsTheServerKit fails when the copy drifts.
//
//go:embed kit/install-singbox-server.sh
var Kit []byte
