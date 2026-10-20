#!/usr/bin/env python3
"""Drive the web hash-password CLI on a PTY.

Reads the password from a 0600 file and writes only the scrypt verifier line.
"""

import argparse
import os
import pty
import re
import select
import shutil
import stat
import subprocess
import sys
import time
import tty

CLI = os.path.join("scripts", "hash-password.mjs")
PROMPTS = (b"Password: ", b"Confirm password: ")
VERIFIER = re.compile(r"^scrypt\$N=32768\$r=8\$p=3\$[0-9a-f]{32}\$[0-9a-f]{64}$")
PROMPT_TIMEOUT = 30.0
EXIT_TIMEOUT = 60.0


def fail(message):
    print(f"error: {message}", file=sys.stderr)
    sys.exit(1)


def read_password(path, output_path):
    if os.path.abspath(path) == os.path.abspath(output_path):
        fail("the output file must not be the password file")
    try:
        mode = stat.S_IMODE(os.stat(path).st_mode)
    except OSError as exc:
        fail(f"cannot stat the password file: {exc.strerror}")
    if mode != 0o600:
        fail(f"the password file must have mode 0600, found {mode:04o}")
    try:
        with open(path, "rb") as handle:
            data = handle.read()
    except OSError as exc:
        fail(f"cannot read the password file: {exc.strerror}")
    if not data.endswith(b"\n"):
        fail("the password file must end with a newline")
    lines = data[:-1].split(b"\n")
    if len(lines) != 2:
        fail(f"the password file must hold exactly two lines, found {len(lines)}")
    password, confirmation = lines
    if password != confirmation:
        fail("the two password lines differ")
    if not password:
        fail("the password line is empty")
    if b"\r" in password:
        fail("the password file must use newline line endings, not CRLF")
    return password


def read_chunk(master, output, remaining):
    try:
        ready, _, _ = select.select([master], [], [], remaining)
    except InterruptedError:
        return b""
    if not ready:
        return b""
    try:
        chunk = os.read(master, 4096)
    except OSError:
        return None
    if not chunk:
        return None
    output.extend(chunk)
    return chunk


def wait_for(master, output, needle, start, timeout):
    deadline = time.monotonic() + timeout
    label = needle.decode("ascii")
    while True:
        index = output.find(needle, start)
        if index >= 0:
            return index + len(needle)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            fail(f"timed out waiting for the {label!r} prompt")
        if read_chunk(master, output, remaining) is None:
            fail(f"the CLI exited before printing the {label!r} prompt")


def drain(master, output, timeout):
    deadline = time.monotonic() + timeout
    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            fail("timed out waiting for the CLI to finish")
        if read_chunk(master, output, remaining) is None:
            return


def write_verifier(path, line):
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    except OSError as exc:
        fail(f"cannot open the output file: {exc.strerror}")
    try:
        os.fchmod(fd, 0o600)
        os.write(fd, line.encode("ascii") + b"\n")
    finally:
        os.close(fd)


def main():
    parser = argparse.ArgumentParser(description="Generate the web password verifier.")
    parser.add_argument("--web-dir", required=True)
    parser.add_argument("--password-file", required=True)
    parser.add_argument("--output-file", required=True)
    args = parser.parse_args()

    web_dir = os.path.abspath(args.web_dir)
    if not os.path.isfile(os.path.join(web_dir, CLI)):
        fail(f"{CLI} not found under {web_dir}")
    node = shutil.which("node")
    if node is None:
        fail("node is not on PATH")
    password = read_password(args.password_file, args.output_file)

    master = slave = None
    proc = None
    output = bytearray()
    status = None
    try:
        master, slave = pty.openpty()
        tty.setraw(slave)
        proc = subprocess.Popen(
            [node, CLI],
            cwd=web_dir,
            stdin=slave,
            stdout=slave,
            stderr=slave,
            start_new_session=True,
        )
        os.close(slave)
        slave = None

        start = 0
        for prompt in PROMPTS:
            start = wait_for(master, output, prompt, start, PROMPT_TIMEOUT)
            try:
                os.write(master, password + b"\r")
            except OSError:
                fail("the CLI closed the terminal before the password was sent")

        drain(master, output, EXIT_TIMEOUT)
        try:
            status = proc.wait(timeout=EXIT_TIMEOUT)
        except subprocess.TimeoutExpired:
            fail("timed out waiting for the CLI to exit")
    finally:
        if slave is not None:
            os.close(slave)
        if master is not None:
            try:
                os.close(master)
            except OSError:
                pass
        if proc is not None and proc.poll() is None:
            proc.kill()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                pass

    if status != 0:
        fail(f"the CLI exited with status {status}")

    matches = []
    for line in output.decode("utf-8", "replace").split("\n"):
        candidate = line.rstrip("\r")
        if VERIFIER.match(candidate):
            matches.append(candidate)
    if len(matches) != 1:
        fail(f"expected exactly one verifier line, found {len(matches)}")

    write_verifier(args.output_file, matches[0])
    print(f"wrote the verifier to {args.output_file}")


if __name__ == "__main__":
    main()
