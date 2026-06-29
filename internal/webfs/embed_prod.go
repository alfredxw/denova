//go:build embedweb

package webfs

import "embed"

// hasEmbedded is true when this file (build tag "embedweb") is compiled in.
const hasEmbedded = true

// embeddedFS holds the built frontend, copied to ./dist by build.sh before the
// tagged build. `all:` includes dotfiles so the manifest/icons come along too.
//
//go:embed all:dist
var embeddedFS embed.FS
