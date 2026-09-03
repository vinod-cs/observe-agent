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
TAG = "v0.1.0-canary.20260903.7"


class FakeCLI:
    def __init__(self):
        self.release = None
        self.data = {}
        self.calls = []
        self.fail_upload = None
        self.keep_draft = False
        self.api_failure = False
        self.created_prerelease = True
        self.created_draft = True
        self.pending_tag = TAG
        self.view_override = {}
        self.keep_prerelease_false = False
        self.missing_reads = 0
        self.hide_after_create = False
        self.become_stable_after_upload = False

    def __call__(self, *args):
        self.calls.append(args)
        if args[0] == "api":
            if self.api_failure:
                raise RuntimeError("API failed")
            assert args[1] == "graphql" and "tag=" + TAG in args
            assert "owner=vinod-cs" in args and "name=observe-agent" in args
            found = {"databaseId": 42} if self.release else None
            if self.release and self.missing_reads:
                self.missing_reads -= 1
                found = None
            return json.dumps({"data": {"repository": {"release": found}}})
        assert args[0] == "release" and args[2] == TAG
        assert args[args.index("--repo") + 1] == pub.REPOSITORY
        assert "--clobber" not in args
        op = args[1]
        if op == "create":
            assert self.release is None
            assert "--verify-tag" in args and "--draft" in args and "--prerelease" in args and "--latest=false" in args
            self.release = {"tag_name": self.pending_tag, "draft": self.created_draft, "prerelease": self.created_prerelease,
                            "body": args[args.index("--notes") + 1], "assets": [],
                            "html_url": f"https://github.com/{pub.REPOSITORY}/releases/tag/{TAG}"}
            if self.hide_after_create:
                self.missing_reads = self.hide_after_create
        elif op == "view":
            assert args[args.index("--json") + 1] == "databaseId,apiUrl,isDraft,isPrerelease,tagName,body,assets,url"
            r = self.release
            value = {"databaseId": 42, "apiUrl": f"https://api.github.com/repos/{pub.REPOSITORY}/releases/42",
                     "isDraft": r["draft"], "isPrerelease": r["prerelease"], "tagName": r["tag_name"],
                     "body": r["body"], "assets": r["assets"], "url": r["html_url"]}
            value.update(self.view_override)
            return json.dumps(value)
        elif op == "upload":
            p = Path(args[3])
            if self.fail_upload == p.name:
                raise RuntimeError("Interrupted fixture upload")
            assert p.name not in self.data
            self.data[p.name] = p.read_bytes()
            self.release["assets"].append({"name": p.name, "state": "uploaded", "size": p.stat().st_size})
            if self.become_stable_after_upload and p.name == "SHA256SUMS":
                self.release.update(draft=False, prerelease=False)
        elif op == "download":
            name = args[args.index("--pattern") + 1]
            p = Path(args[args.index("--dir") + 1]) / name
            assert not p.exists()
            p.write_bytes(self.data[name])
        elif op == "edit":
            assert "--draft=false" in args and "--prerelease" in args and "--latest=false" in args
            if not self.keep_draft:
                self.release["draft"] = False
                self.release["tag_name"] = TAG
            self.release["prerelease"] = not self.keep_prerelease_false
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
        self.sleep = patch.object(pub.time, "sleep").start()
        self.addCleanup(patch.stopall)

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

    def test_hosted_failure_owned_draft_false_prerelease_now_publishes(self):
        # Exact old predicate raised the reported error for a draft, before its final edit.
        draft = {"draft": True, "prerelease": False, "body": pub.MARKER}
        with self.assertRaisesRegex(RuntimeError, "Expected canary prerelease missing"):
            pub.require(draft is not None and draft.get("prerelease") is True,
                        "Expected canary prerelease missing (stable releases are never converted)")
        self.cli.created_prerelease = False
        self.publish()
        self.assertFalse(self.cli.release["draft"])
        self.assertTrue(self.cli.release["prerelease"])
        self.assertEqual(len(self.cli.release["assets"]), 3)

    def test_owned_nonprerelease_draft_resumes_without_duplicate_create(self):
        self.cli.fail_upload = "SHA256SUMS"
        with self.assertRaises(RuntimeError):
            self.publish()
        self.cli.release["prerelease"] = False
        self.cli.fail_upload = None
        self.publish()
        self.assertEqual(sum(c[1] == "create" for c in self.mutations()), 1)
        self.assertTrue(self.cli.release["prerelease"])

    def test_new_create_unexpectedly_published_stable_is_never_converted(self):
        self.cli.created_prerelease = False
        self.cli.created_draft = False
        with self.assertRaisesRegex(RuntimeError, "Published stable release exists"):
            self.publish()
        self.assertEqual([c[1] for c in self.mutations()], ["create"])

    def test_unrelated_nonprerelease_draft_fails_without_mutation(self):
        self.publish()
        self.cli.release.update(draft=True, prerelease=False, body="unrelated")
        before = self.mutations()
        with self.assertRaisesRegex(RuntimeError, "not owned"):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_final_edit_loses_prerelease_flag_fails_and_no_stable_repair(self):
        self.cli.keep_prerelease_false = True
        with self.assertRaisesRegex(RuntimeError, "Publication not confirmed"):
            self.publish()
        before = self.mutations()
        with self.assertRaisesRegex(RuntimeError, "Published stable release exists"):
            self.publish()
        self.assertEqual(before, self.mutations())

    def test_pending_draft_tag_not_visible_to_old_rest_tag_filter(self):
        self.cli.pending_tag = "untagged-fixture-pending"
        # GraphQL resolves the requested pending tag to ID 42; CLI view preserves that ID.
        self.publish()
        self.assertEqual(self.cli.release["tag_name"], TAG)
        self.assertEqual(sum(c[1] == "create" for c in self.mutations()), 1)
        self.assertFalse(any("releases?per_page" in str(c) for c in self.cli.calls))

    def test_missing_after_create_has_own_error_and_no_second_create(self):
        self.cli.hide_after_create = 3
        with self.assertRaisesRegex(RuntimeError, "Release missing during after-create"):
            self.publish()
        self.assertEqual([c[1] for c in self.mutations()], ["create"])
        self.assertEqual(self.sleep.call_count, 2)

    def test_transient_missing_after_create_retries_lookup_only(self):
        self.cli.hide_after_create = 2
        self.publish()
        self.assertEqual(sum(c[1] == "create" for c in self.mutations()), 1)
        self.assertEqual(self.sleep.call_count, 2)

    def test_cli_boolean_schema_is_not_rest_or_truthy_string(self):
        for value in [None, "false", "true", 0, 1]:
            with self.subTest(value=value):
                self.cli = FakeCLI()
                self.cli.view_override = {"isPrerelease": value, "prerelease": True}
                with self.assertRaisesRegex(RuntimeError, "JSON schema invalid"):
                    self.publish()
                self.assertEqual([c[1] for c in self.mutations()], ["create"])

    def test_wrong_repository_or_release_id_is_rejected(self):
        for override in [{"apiUrl": "https://api.github.com/repos/other/repo/releases/42"}, {"databaseId": 43}]:
            with self.subTest(override=override):
                self.cli = FakeCLI()
                self.cli.view_override = override
                with self.assertRaisesRegex(RuntimeError, "identity mismatch"):
                    self.publish()
                self.assertEqual([c[1] for c in self.mutations()], ["create"])

    def test_wrong_published_tag_is_rejected(self):
        self.cli.view_override = {"tagName": "v0.1.0-canary.other"}
        with self.assertRaisesRegex(RuntimeError, "Published release tag"):
            self.publish()

    def test_becomes_stable_during_upload_is_not_converted(self):
        self.cli.become_stable_after_upload = True
        with self.assertRaisesRegex(RuntimeError, "Published stable release exists"):
            self.publish()
        self.assertFalse(any(c[1] == "edit" for c in self.mutations()))


if __name__ == "__main__":
    unittest.main()
# AGENTV1 FILE END
