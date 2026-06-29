//go:build !embedweb

package webfs

import "io/fs"

// hasEmbedded is false in default builds; the frontend is served from disk.
const hasEmbedded = false

// embeddedFS is unused (nil) in non-embedweb builds.
var embeddedFS fs.FS
