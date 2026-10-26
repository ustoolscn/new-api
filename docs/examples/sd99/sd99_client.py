#!/usr/bin/env python3
"""New API SD99 example client. POST is never automatically retried."""

import argparse
import json
import os
from pathlib import Path
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # The New API content proxy returns bytes itself. Reject unexpected
        # redirects so a client credential can never reach another origin.
        return None


class Client:
    def __init__(self):
        self.base = os.environ.get("NEW_API_BASE_URL", "").rstrip("/")
        self.key = os.environ.get("NEW_API_KEY", "")
        parsed = urllib.parse.urlsplit(self.base)
        if parsed.scheme not in ("https", "http") or not parsed.netloc:
            raise ValueError("Set NEW_API_BASE_URL to your New API service origin")
        if parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError("NEW_API_BASE_URL must not contain credentials, query or fragment")
        if self.base.endswith("/v1"):
            raise ValueError("Use the service root in NEW_API_BASE_URL, without /v1")
        if not self.key:
            raise ValueError("Set NEW_API_KEY to a key created on your New API platform")
        self.opener = urllib.request.build_opener(NoRedirect())

    def open(self, path, body=None, timeout=30):
        headers = {"Authorization": "Bearer " + self.key}
        data = None
        if body is not None:
            headers["Content-Type"] = "application/json; charset=utf-8"
            data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        request = urllib.request.Request(self.base + path, data=data, headers=headers)
        return self.opener.open(request, timeout=timeout)

    def json(self, path, body=None, timeout=30):
        with self.open(path, body, timeout) as response:
            result = json.load(response)
        if not isinstance(result, dict):
            raise ValueError("Expected a JSON object")
        return result


def save_record(file, record):
    file.seek(0)
    json.dump(record, file, ensure_ascii=False, indent=2)
    file.write("\n")
    file.truncate()
    file.flush()
    os.fsync(file.fileno())


def create(client, args):
    payload = json.loads(Path(args.request).read_text(encoding="utf-8-sig"))
    if not isinstance(payload, dict):
        raise ValueError("Request file must contain a JSON object")
    # Reserve the record before sending a potentially billable request. An
    # existing file is never overwritten by create, even after a crash.
    with Path(args.task_file).open("x", encoding="utf-8") as file:
        record = {"state": "submitting", "submitted_at": int(time.time())}
        save_record(file, record)
        try:
            response = client.json("/v1/video/generations", payload, timeout=180)
            record["response"] = response
            task_id = response.get("id", "")
            if not isinstance(task_id, str) or not re.fullmatch(r"task_[A-Za-z0-9_-]{1,128}", task_id):
                raise ValueError("No valid public task ID; verify submission before retrying")
            record.update(state="submitted", id=task_id)
            save_record(file, record)
        except (Exception, KeyboardInterrupt):
            record["state"] = "submission_unconfirmed"
            save_record(file, record)
            print("Submission needs verification. Do not repeat POST or delete the task file.", file=sys.stderr)
            raise
    print(json.dumps(response, ensure_ascii=False, indent=2))


def task_id_from_file(path):
    record = json.loads(Path(path).read_text(encoding="utf-8-sig"))
    task_id = record.get("id", "")
    if not isinstance(task_id, str) or not re.fullmatch(r"task_[A-Za-z0-9_-]{1,128}", task_id):
        raise ValueError("Task file has no valid ID. Verify the original submission; do not recreate it")
    return task_id


def wait(client, task_id, timeout, interval):
    if timeout <= 0 or interval <= 0:
        raise ValueError("timeout and interval must be positive")
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        remaining = deadline - time.monotonic()
        try:
            result = client.json("/v1/videos/" + task_id, timeout=min(30, remaining))
        except (urllib.error.URLError, TimeoutError, ValueError):
            print("Query failed; retrying GET for the same task.", file=sys.stderr)
        else:
            status = result.get("status", "unknown")
            print(f"{task_id}: {status} ({result.get('progress', 0)}%)", flush=True)
            if status == "completed":
                return
            if status == "failed":
                raise RuntimeError(json.dumps(result.get("error", {}), ensure_ascii=False))
            if status not in ("queued", "in_progress", "unknown"):
                print("Unexpected status; retaining the original task.", file=sys.stderr)
        remaining = deadline - time.monotonic()
        if remaining > 0:
            time.sleep(min(interval, remaining))
    raise TimeoutError("Local wait timed out. Keep this task file and resume GET later; do not recreate")


def download(client, task_id, output):
    status = client.json("/v1/videos/" + task_id)
    if status.get("status") != "completed":
        raise ValueError("Task is not completed. Query or wait on the same task before downloading")
    destination = Path(output)
    if destination.exists():
        raise FileExistsError("Output file already exists; choose a new --output path")
    with client.open("/v1/videos/" + task_id + "/content", timeout=600) as response:
        prefix = response.read(1024)
        content_type = response.headers.get("Content-Type", "").lower()
        if not prefix or "json" in content_type or "html" in content_type or prefix.lstrip()[:1] in (b"{", b"[", b"<"):
            raise ValueError("Download returned empty data or an error document; retry this download only")
        # Exclusive open avoids accidentally replacing an existing result.
        with destination.open("xb") as file:
            try:
                file.write(prefix)
                while True:
                    chunk = response.read(1024 * 1024)
                    if not chunk:
                        break
                    file.write(chunk)
            except (Exception, KeyboardInterrupt):
                file.close()
                destination.unlink()
                raise
    print(f"Saved video to {destination.resolve()}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    create_parser = actions.add_parser("create", help="Send exactly one billable creation request")
    create_parser.add_argument("request", help="Path to one JSON request file")
    create_parser.add_argument("--task-file", default="created-task.json")
    for name in ("status", "wait", "download"):
        action = actions.add_parser(name)
        action.add_argument("--task-file", default="created-task.json")
        if name == "wait":
            action.add_argument("--timeout", type=float, default=1800)
            action.add_argument("--interval", type=float, default=15)
        if name == "download":
            action.add_argument("--output", default="generated-video.mp4")
    args = parser.parse_args()
    client = Client()
    if args.action == "create":
        create(client, args)
        return
    task_id = task_id_from_file(args.task_file)
    if args.action == "status":
        print(json.dumps(client.json("/v1/videos/" + task_id), ensure_ascii=False, indent=2))
    elif args.action == "wait":
        wait(client, task_id, args.timeout, args.interval)
    else:
        download(client, task_id, args.output)


if __name__ == "__main__":
    try:
        main()
    except (Exception, KeyboardInterrupt) as error:
        print(f"Error: {error}", file=sys.stderr)
        sys.exit(1)
