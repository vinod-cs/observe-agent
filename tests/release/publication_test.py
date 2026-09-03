# AGENTV1 FILE START: offline release publication model; gh/network never executed.
import hashlib
import contextlib
import io
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("publisher", ROOT / "scripts/publish-release.py")
pub = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pub)
TAG = "v0.1.0-canary.20260903.6"


class FakeCLI:
    def __init__(self):
        self.release = None
        self.data = {}
        self.calls = []
        self.fail_upload = None
        self.keep_draft = False
        self.api_failure = False

    def __call__(self, *args):
        self.calls.append(args)
        if args[0] == "api":
            if self.api_failure:
                raise RuntimeError("API failed")
            assert args[1] == f"repos/{pub.REPOSITORY}/releases?per_page=100"
            assert args[2:] == ("--paginate", "--slurp")
            return json.dumps([[], [self.release] if self.release else []])
        assert args[0] == "release" and args[2] == TAG
        assert args[args.index("--repo") + 1] == pub.REPOSITORY
        assert "--clobber" not in args
        op = args[1]
        if op == "create":
            assert self.release is None
            assert "--verify-tag" in args and "--draft" in args and "--prerelease" in args and "--latest=false" in args
            self.release = {"tag_name": TAG, "draft": True, "prerelease": True,
                            "body": args[args.index("--notes") + 1], "assets": [],
                            "html_url": f"https://github.com/{pub.REPOSITORY}/releases/tag/{TAG}"}
        elif op == "upload":
            p = Path(args[3])
            if self.fail_upload == p.name:
                raise RuntimeError("Interrupted fixture upload")
            assert p.name not in self.data
            self.data[p.name] = p.read_bytes()
            self.release["assets"].append({"name": p.name, "state": "uploaded", "size": p.stat().st_size})
        elif op == "download":
            name = args[args.index("--pattern") + 1]
            p = Path(args[args.index("--dir") + 1]) / name
            assert not p.exists()
            p.write_bytes(self.data[name])
        elif op == "edit":
            assert "--draft=false" in args and "--prerelease" in args and "--latest=false" in args
            if not self.keep_draft:
                self.release["draft"] = False
        else:
            raise AssertionError(f"Unexpected CLI operation: {op}")
        return ""


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="observe-publish-test-")
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)
        self.deb = f"observe-agent_{TAG[1:]}_amd64.deb"
        self.alias = "observe-agent_linux_amd64.deb"
        for name in [self.deb, self.alias]:
            (self.directory / name).write_bytes(b"fixture only, not an installable DEB")
        self.manifest()
        self.cli = FakeCLI()

    def manifest(self):
        (self.directory / "SHA256SUMS").write_text("".join(
            hashlib.sha256((self.directory / n).read_bytes()).hexdigest() + "  " + n + "\n"
            for n in [self.deb, self.alias]), encoding="utf-8", newline="\n")

    def publish(self):
        # Suppress fake Release URLs so offline test logs cannot be mistaken for publication.
        with contextlib.redirect_stdout(io.StringIO()):
            return pub.publish(TAG, self.directory, self.cli)

    def mutations(self):
        return [c for c in self.cli.calls if c[0] == "release" and c[1] in ["create", "upload", "edit"]]

    def test_new_prerelease_and_identical_rerun(self):
        url, names = self.publish()
        self.assertEqual(set(names), {self.deb, self.alias, "SHA256SUMS"})
        self.assertTrue(url.endswith("/releases/tag/" + TAG))
        self.assertFalse(self.cli.release["draft"])
        self.assertTrue(self.cli.release["prerelease"])
        before = self.mutations()
        self.publish()
        self.assertEqual(before, self.mutations())

    def test_interrupted_owned_draft_resumes_only_missing_assets(self):
        self.cli.fail_upload = "SHA256SUMS"
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertTrue(self.cli.release["draft"])
        self.cli.fail_upload = None
        self.publish()
        self.assertEqual(sum(c[1] == "create" for c in self.mutations()), 1)
        self.assertEqual(sum(c[1] == "upload" and Path(c[3]).name == self.deb for c in self.mutations()), 1)

    def test_missing_asset_fails_before_cli(self):
        (self.directory / self.alias).unlink()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(self.cli.calls, [])

    def test_wrong_filename_fails_before_cli(self):
        (self.directory / self.deb).rename(self.directory / "observe-agent_wrong_amd64.deb")
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(self.cli.calls, [])

    def test_extra_local_asset_fails(self):
        (self.directory / "unexpected.zip").write_bytes(b"not supported")
        with self.assertRaises(RuntimeError):
            self.publish()

    def test_bad_checksum_fails(self):
        (self.directory / self.deb).write_bytes(b"corrupt")
        with self.assertRaises(RuntimeError):
            self.publish()

    def test_wrong_alias_even_with_valid_hash_fails(self):
        (self.directory / self.alias).write_bytes(b"different package")
        self.manifest()
        with self.assertRaises(RuntimeError):
            self.publish()

    def test_duplicate_or_traversal_manifest_fails(self):
        p = self.directory / "SHA256SUMS"
        original = p.read_text()
        for contents in [original + original, "0" * 64 + "  ../../outside.deb\n"]:
            p.write_text(contents)
            with self.assertRaises(RuntimeError):
                self.publish()

    def test_changed_remote_bytes_never_overwritten(self):
        self.publish()
        self.cli.data[self.deb] = b"different remote bytes"
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_unrelated_remote_asset_never_deleted(self):
        self.publish()
        self.cli.release["assets"].append({"name": "unrelated.txt", "state": "uploaded", "size": 1})
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_unknown_draft_not_adopted(self):
        self.publish()
        self.cli.release.update(draft=True, body="Manually created draft")
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_stable_release_never_changed(self):
        self.publish()
        self.cli.release["prerelease"] = False
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_api_failure_not_treated_as_missing(self):
        self.cli.api_failure = True
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(self.mutations(), [])

    def test_successful_edit_that_leaves_draft_fails(self):
        self.cli.keep_draft = True
        with self.assertRaisesRegex(RuntimeError, "Publication not confirmed"):
            self.publish()

    def test_published_missing_asset_not_modified(self):
        self.publish()
        self.cli.release["assets"].pop()
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_stable_and_unsafe_tags_rejected(self):
        for tag in ["v0.1.0", "../tag", "v0.1.0;echo"]:
            with self.assertRaises(RuntimeError):
                pub.publish(tag, self.directory, self.cli)
        self.assertEqual(self.cli.calls, [])

    def test_local_entry_point_refuses_publication(self):
        with patch.dict(pub.os.environ, {}, clear=True), patch.object(pub, "gh") as cli:
            with self.assertRaises(RuntimeError):
                pub.main()
            cli.assert_not_called()

    def test_cli_error_does_not_echo_token(self):
        with patch.object(pub.subprocess, "run") as run:
            run.return_value.returncode = 1
            run.return_value.stderr = "secret-token-fixture"
            with self.assertRaises(RuntimeError) as error:
                pub.gh("api", "fixture")
            self.assertNotIn("secret-token-fixture", str(error.exception))

    def test_workflow_summary_contains_verified_release_and_asset_links(self):
        output = self.directory / "outputs"
        summary = self.directory / "summary"
        url = f"https://github.com/{pub.REPOSITORY}/releases/tag/{TAG}"
        names = [self.deb, self.alias, "SHA256SUMS"]
        env = {"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": pub.REPOSITORY,
               "GITHUB_REF": "refs/tags/" + TAG, "RELEASE_TAG": TAG, "GH_TOKEN": "fake-test-only",
               "GITHUB_OUTPUT": str(output), "GITHUB_STEP_SUMMARY": str(summary)}
        with patch.dict(pub.os.environ, env, clear=True), patch.object(pub, "publish", return_value=(url, names)):
            pub.main()
        self.assertEqual(output.read_text(), "release_url=" + url + "\n")
        for name in names:
            self.assertIn("/releases/download/" + TAG + "/" + name, summary.read_text())
        self.assertIn("approval gate, not the release", summary.read_text())
        self.assertNotIn("fake-test-only", summary.read_text())

    def test_incomplete_remote_upload_is_not_overwritten(self):
        self.publish()
        self.cli.release.update(draft=True)
        self.cli.release["assets"][0]["state"] = "starter"
        before = self.mutations()
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertEqual(before, self.mutations())


if __name__ == "__main__":
    unittest.main()
# AGENTV1 FILE END
