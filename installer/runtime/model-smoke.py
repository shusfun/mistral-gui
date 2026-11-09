"""Run a real HY-MT request against an explicitly supplied desktop runtime."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import urllib.request
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("--engine", required=True)
parser.add_argument("--backend", choices=("cpu", "cuda", "metal"), required=True)
parser.add_argument("--data-dir", required=True)
args = parser.parse_args()
data = Path(args.data_dir).resolve()
data.mkdir(parents=True, exist_ok=True)
revision = "265b2e615a7dc9b06c435dc878829ad99a512ba2"
model = data / "HY-MT1.5-1.8B-Q4_K_M.gguf"
digest = "4383ac0c3c8e476de98ff979c2a3f069f8c4fb385e7860cf2d28da896cc477c7"
trace = "00-" + uuid.uuid4().hex + "-" + uuid.uuid4().hex[:16] + "-01"


def emit(phase, **fields):
    print(json.dumps({"traceparent": trace, "phase": phase, **fields}), flush=True)


def checksum(path):
    h = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


emit("model.check", backend=args.backend)
if not model.exists() or checksum(model) != digest:
    partial = model.with_suffix(".gguf.part")
    url = f"https://huggingface.co/tencent/HY-MT1.5-1.8B-GGUF/resolve/{revision}/{model.name}?download=true"
    for attempt in range(3):
        try:
            offset = partial.stat().st_size if partial.exists() else 0
            headers = {"Range": f"bytes={offset}-"} if offset else {}
            with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=120) as response:
                mode = "ab" if offset and response.status == 206 else "wb"
                with partial.open(mode) as output:
                    for chunk in iter(lambda: response.read(1024 * 1024), b""):
                        output.write(chunk)
            if checksum(partial) != digest:
                partial.unlink()
                raise RuntimeError("Model SHA-256 mismatch")
            partial.replace(model)
            break
        except Exception as error:
            if attempt == 2:
                raise
            emit("model.retry", reason=type(error).__name__)

with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    port = listener.getsockname()[1]
engine = Path(args.engine).resolve()
command = [str(engine), "serve", "--host", "127.0.0.1", "--port", str(port), "-f", str(model), "--hf-cache", str(data / "hf-cache"), "--max-seqs", "1", "--max-model-len", "1024", "--tok-model-id", "tencent/HY-MT1.5-1.8B", "--disable-access-log"]
if args.backend == "cpu":
    command.append("--cpu")
environment = dict(os.environ)
environment.pop("HF_TOKEN", None)
environment["HF_HOME"] = str(data / "hf-home")
started = time.monotonic()
with (data / "engine.log").open("wb") as output:
    process = subprocess.Popen(command, stdout=output, stderr=subprocess.STDOUT, env=environment, cwd=engine.parent)
    try:
        emit("runtime.start", pid=process.pid)
        deadline = started + 300
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError(f"Engine exited: {process.returncode}")
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/v1/models", timeout=2) as response:
                    models = json.load(response)
                if models.get("data"):
                    break
            except Exception:
                time.sleep(.5)
        else:
            raise RuntimeError("Runtime readiness timeout")
        loaded = time.monotonic() - started
        emit("runtime.ready", seconds=round(loaded, 3))
        started = time.monotonic()
        request = urllib.request.Request(f"http://127.0.0.1:{port}/v1/chat/completions", data=json.dumps({"model": models["data"][0]["id"], "messages": [{"role": "user", "content": "Translate into Chinese: Hello."}], "max_tokens": 32, "stream": False}).encode(), headers={"Content-Type": "application/json", "traceparent": trace})
        with urllib.request.urlopen(request, timeout=120) as response:
            result = json.load(response)
            translation = result["choices"][0]["message"]["content"]
            if response.status != 200 or "你好" not in translation:
                raise RuntimeError("Runtime did not return the expected translation")
            emit("inference.complete", http_status=response.status, startup_seconds=round(loaded, 3), request_seconds=round(time.monotonic()-started, 3))
    except Exception:
        output.flush()
        print((data / "engine.log").read_text(encoding="utf8", errors="replace")[-8192:])
        raise
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        emit("runtime.stop")
