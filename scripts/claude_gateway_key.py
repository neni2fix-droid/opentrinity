import os
import subprocess
import sys

root = r"C:\Users\ai9\Documents\opentrinity"
env = dict(os.environ)
env["OT_USER_DATA"] = os.path.join(root, ".demo-userdata")
exe = os.path.join(root, "dist", "opentrinity.exe")

result = subprocess.run(
    [exe, "gateway", "key", "show"],
    input="SHOW\n",
    text=True,
    stdout=subprocess.PIPE,
    stderr=subprocess.DEVNULL,
    env=env,
)
if result.returncode != 0:
    sys.exit("gateway key unavailable")

key = result.stdout.strip().split(":", 1)[-1].strip()
if not key:
    sys.exit("gateway key unavailable")
sys.stdout.write(key)
