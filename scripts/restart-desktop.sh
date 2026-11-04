#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
APP_PATH="${ROOT_DIR}/desktop/dist/mac-arm64/Denova.app"

if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
    echo "Error: this script requires native macOS on Apple Silicon." >&2
    exit 1
fi

cd "${ROOT_DIR}"
ELECTRON_PATH="$(cd desktop && node -p "require('electron')")"
mkdir -p desktop/build
LOCK_PATH="${ROOT_DIR}/desktop/build/.restart-desktop.lock"
if ! shlock -f "${LOCK_PATH}" -p "$$"; then
    echo "Error: another desktop restart is in progress or the build directory is not writable." >&2
    exit 1
fi
trap 'rm -f "${LOCK_PATH}"' EXIT

echo "==> Waiting for the local desktop app to quit (confirm in Denova if prompted)"
osascript -l JavaScript - "${APP_PATH}" "${ELECTRON_PATH}" <<'JXA'
ObjC.import('AppKit')

function runningApplications(bundleId) {
    const running = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(bundleId)
    return Array.from({ length: running.count }, (_, index) => running.objectAtIndex(index))
}

function run([bundlePath, electronPath]) {
    const packaged = runningApplications('app.denova.desktop')
    for (const app of packaged) {
        if (!app.isTerminated && ObjC.unwrap(app.bundleURL.path) !== bundlePath) {
            throw new Error('Quit the other running copy of Denova before restarting this build.')
        }
    }

    const development = runningApplications('com.github.Electron').filter(app =>
        !app.isTerminated && ObjC.unwrap(app.executableURL.path) === electronPath)
    const apps = [...packaged, ...development]
    for (const app of apps) {
        if (!app.isTerminated && !app.terminate && !app.isTerminated) {
            throw new Error('Could not request a normal quit. Quit Denova and rerun this script.')
        }
    }

    const deadline = Date.now() + 120000
    while (apps.some(app => !app.isTerminated)) {
        if (Date.now() >= deadline) {
            throw new Error('Timed out waiting for Denova to quit. Quit Denova and rerun this script.')
        }
        $.NSRunLoop.currentRunLoop.runUntilDate($.NSDate.dateWithTimeIntervalSinceNow(0.25))
    }
}
JXA

if [ ! -x desktop/build/backend/denova ]; then
    echo "==> Building the missing desktop backend"
    env -u DENOVA_RELEASE_FRONTEND_DIR DENOVA_DESKTOP_TARGET=darwin-arm64 pnpm --dir desktop build:backend
fi

rm -f "${LOCK_PATH}"
trap - EXIT
echo "==> Starting Denova Desktop with Vite hot updates"
exec pnpm --dir desktop dev:hmr
