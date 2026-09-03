# AGENTV1 FILE START: Actions-only publication; no Agent/runtime or package mutation.
"""Publish verified canary assets using gh and the workflow-provided token only."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

REPOSITORY = "vinod-cs/observe-agent"
TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-canary\.[0-9A-Za-z.]+")
MARKER = "<!-- observe-agent-validated-canary-v1 -->"


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def assets_for(tag, directory):
    require(TAG.fullmatch(tag), "Only explicit vMAJOR.MINOR.PATCH-canary.SUFFIX tags may publish")
    names = [f"observe-agent_{tag[1:]}_amd64.deb", "observe-agent_linux_amd64.deb", "SHA256SUMS"]
    directory = Path(directory)
    require(directory.is_dir(), "Validated artifact directory is missing")
    require({p.name for p in directory.iterdir()} == set(names), "Missing, extra or incorrectly named release assets")
    for name in names:
        p = directory / name
        require(not p.is_symlink() and p.is_file() and 0 < p.stat().st_size <= 150 * 1024 * 1024, "Unsafe, empty or oversized release asset")
    require((directory / "SHA256SUMS").stat().st_size <= 65536, "Checksum manifest too large")
    entries = {}
    for line in (directory / "SHA256SUMS").read_text(encoding="utf-8").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  (observe-agent_[A-Za-z0-9._-]+\.deb)", line)
        require(match is not None, "Invalid checksum entry")
        value, name = match.groups()
        require(name not in entries, "Duplicate checksum entry")
        entries[name] = value
    hashes = {name: digest(directory / name) for name in names}
    require(entries == {name: hashes[name] for name in names[:2]}, "Checksum mismatch or unexpected checksum filenames")
    require(hashes[names[0]] == hashes[names[1]], "Stable-name alias is not identical to the versioned DEB")
    return names, hashes


def gh(*args):
    result = subprocess.run(["gh", *args], capture_output=True, text=True, timeout=300)
    # Never echo token-bearing environment or uncontrolled CLI/API responses.
    require(result.returncode == 0, "GitHub CLI operation failed; no fallback create/overwrite attempted")
    return result.stdout


def find_release(tag, cli):
    # A successful paginated list distinguishes absence from authentication/API failure.
    pages = json.loads(cli("api", f"repos/{REPOSITORY}/releases?per_page=100", "--paginate", "--slurp"))
    matches = [r for page in pages for r in page if r["tag_name"] == tag]
    require(len(matches) <= 1, "Ambiguous release identity")
    return matches[0] if matches else None


def verify_remote(release, tag, names, hashes, cli):
    remote = release["assets"]
    remote_names = [a["name"] for a in remote]
    require(len(remote_names) == len(set(remote_names)), "Duplicate remote assets; refusing mutation")
    require(set(remote_names) <= set(names), "Unrelated remote assets found; refusing mutation")
    require(all(a.get("state") == "uploaded" for a in remote), "Incomplete remote upload; operator review required")
    require(all(0 < a.get("size", 0) <= 150 * 1024 * 1024 for a in remote), "Unexpected remote asset size")
    # Download only exact allowlisted names into a new private directory. No --clobber.
    with tempfile.TemporaryDirectory(prefix="observe-release-verify-") as tmp:
        for name in remote_names:
            cli("release", "download", tag, "--repo", REPOSITORY, "--pattern", name, "--dir", tmp)
            p = Path(tmp) / name
            require(p.is_file() and not p.is_symlink() and digest(p) == hashes[name], "Existing release asset differs; retain original validated artifact or use a new tag; never overwrite")
    return [name for name in names if name not in remote_names]


def publish(tag, directory, cli=gh):
    names, hashes = assets_for(tag, directory)
    release = find_release(tag, cli)
    if release is None:
        cli("release", "create", tag, "--repo", REPOSITORY, "--verify-tag", "--draft", "--prerelease", "--latest=false",
            "--title", f"Observe Agent {tag[1:]} (canary)", "--notes",
            MARKER + "\nTest-only Linux AMD64 DEB. Installation does not start the service. Configure and validate before starting. No production-readiness claim.")
        release = find_release(tag, cli)
    require(release is not None and release.get("prerelease") is True, "Expected canary prerelease missing (stable releases are never converted)")
    if release["draft"]:
        require(MARKER in (release.get("body") or ""), "Existing draft is not owned by this publisher; explicit operator review required")
    missing = verify_remote(release, tag, names, hashes, cli)
    if not release["draft"]:
        require(not missing, "Published release is incomplete; refusing to mutate published assets")
    else:
        for name in missing:
            cli("release", "upload", tag, str(Path(directory) / name), "--repo", REPOSITORY)
        release = find_release(tag, cli)
        require(release is not None and not verify_remote(release, tag, names, hashes, cli), "Draft is incomplete; not publishing")
        cli("release", "edit", tag, "--repo", REPOSITORY, "--draft=false", "--prerelease", "--latest=false")
    final = find_release(tag, cli)
    require(final is not None and final.get("draft") is False and final.get("prerelease") is True,
            "Publication not confirmed: release must be non-draft and prerelease")
    require(not verify_remote(final, tag, names, hashes, cli), "Published release assets missing")
    url = f"https://github.com/{REPOSITORY}/releases/tag/{tag}"
    require(final.get("html_url") == url, "Unexpected published release URL")
    print(f"Verified GitHub prerelease: {url}")
    return url, names


def main():
    tag = os.environ.get("RELEASE_TAG", "")
    require(os.environ.get("GITHUB_ACTIONS") == "true" and os.environ.get("GITHUB_REPOSITORY") == REPOSITORY
            and os.environ.get("GITHUB_REF") == f"refs/tags/{tag}", "Publication is restricted to the repository tag workflow")
    require(bool(os.environ.get("GH_TOKEN")), "Workflow GITHUB_TOKEN is required")
    url, names = publish(tag, "release-assets")
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as out:
            out.write(f"release_url={url}\n")
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as out:
            out.write(f"## Verified GitHub prerelease\n\n[{tag}]({url})\n\nThe canary-release environment is an approval gate, not the release itself.\n\n")
            for name in names:
                out.write(f"- [{name}](https://github.com/{REPOSITORY}/releases/download/{tag}/{name})\n")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as error:
        print(f"Release publication failed: {error}" if isinstance(error, RuntimeError)
              else "Release publication failed: invalid response, local I/O or CLI execution error", file=sys.stderr)
        sys.exit(1)
# AGENTV1 FILE END
