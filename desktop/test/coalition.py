"""List a macOS app's complete resource coalition, including launchd-owned WebKit XPCs."""
import ctypes
import json
import re
import subprocess
import sys

lib = ctypes.CDLL('/usr/lib/libproc.dylib')
lib.proc_pidinfo.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_uint64, ctypes.c_void_p, ctypes.c_int]
lib.proc_pidinfo.restype = ctypes.c_int


def coalition(pid):
    # PROC_PIDCOALITIONINFO: two coalition IDs followed by three reserved words.
    info = (ctypes.c_uint64 * 5)()
    if lib.proc_pidinfo(pid, 20, 0, info, ctypes.sizeof(info)) != ctypes.sizeof(info):
        return None
    return tuple(info[:2])


owner = coalition(int(sys.argv[1]))
if not owner or not all(owner):
    raise RuntimeError('Cannot identify the application coalition')
selected = []
for line in subprocess.check_output(['ps', '-axo', 'pid=,ppid=,rss=,comm='], text=True).splitlines():
    match = re.match(r'\s*(\d+)\s+(\d+)\s+(\d+)\s+(.*)', line)
    pid, ppid, rss, name = match.groups()
    if coalition(int(pid)) == owner:
        selected.append(dict(pid=int(pid), ppid=int(ppid), rss=int(rss) * 1024, name=name))
print(json.dumps(dict(coalition=owner, processes=selected)))
