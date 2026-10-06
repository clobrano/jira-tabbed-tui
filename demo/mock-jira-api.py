#!/usr/bin/env python3
"""Mock Jira REST API for the jira-tabbed-tui demo.

The TUI uses the `jira` CLI for most reads and writes, but talks to the Jira
REST API directly for a few things the CLI can't do: listing an issue's
transitions (the status picker) and its remote/web links. This tiny server
answers those calls with canned data so the demo GIF can be recorded with no
live Jira instance. demo/config.yaml points backend.url at it.

It also answers GitHub's pull request API under /github, so the PR links show
their state; the tape sets GITHUB_API_URL to that prefix.

Statuses changed with `jira issue move` (handled by demo/bin/jira) are read
back from the same state file, so the status picker always marks the issue's
current status.

It is NOT a real Jira server. Usage: mock-jira-api.py [PORT]   (default 8642)
"""

import json
import os
import re
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8642
STATE = os.environ.get("JIRA_DEMO_STATE") or os.path.join(
    tempfile.gettempdir(), "jira-tabbed-tui-demo.state"
)

DEFAULT_STATUS = {
    "PLATFORM-151": "In Progress",
    "PLATFORM-152": "To Do",
    "PLATFORM-98": "Done",
    "PLATFORM-138": "To Do",
    "PLATFORM-127": "To Do",
    "PLATFORM-131": "In Review",
}
WORKFLOW = ["To Do", "In Progress", "In Review", "Done"]


def _remote(relationship, title, url):
    return {"relationship": relationship, "object": {"url": url, "title": title}}


REMOTE_LINKS = {
    "PLATFORM-142": [
        _remote("mentioned in", "Incident: Safari login flicker",
                "https://status.example.com/incidents/safari-login-flicker"),
        _remote("fixed by", "acme/web#482", "https://github.com/acme/web/pull/482"),
        _remote("relates to", "acme/web#471", "https://github.com/acme/web/pull/471"),
        _remote("relates to", "acme/web#465", "https://github.com/acme/web/pull/465"),
    ],
}

# GitHub pull request states, served under /github (the tape points
# $GITHUB_API_URL there) so PR links show open / merged / closed.
PULLS = {
    "482": {"state": "open", "merged": False, "draft": False},
    "471": {"state": "closed", "merged": True, "draft": False},
    "465": {"state": "closed", "merged": False, "draft": False},
}


def status_of(key):
    status = DEFAULT_STATUS.get(key, "In Progress")
    try:
        with open(STATE) as f:
            for line in f:
                k, _, v = line.rstrip("\n").partition("\t")
                if k == key and v:
                    status = v
    except FileNotFoundError:
        pass
    return status


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        pr = re.match(r"^/github/repos/[^/]+/[^/]+/pulls/(\d+)$", self.path)
        if pr:
            if pr.group(1) not in PULLS:
                self.send_error(404)
                return
            return self.reply(PULLS[pr.group(1)])
        m = re.match(r"^/rest/api/\d+/issue/([^/]+)/(transitions|remotelink)", self.path)
        if not m:
            return self.reply([])
        key, what = m.groups()
        if what == "remotelink":
            return self.reply(REMOTE_LINKS.get(key, []))
        current = status_of(key)
        return self.reply(
            {
                "transitions": [
                    {"id": str(i + 1), "name": name, "isCurrentStatus": name == current}
                    for i, name in enumerate(WORKFLOW)
                ]
            }
        )

    def reply(self, body):
        data = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass  # keep the recording's terminal quiet


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
