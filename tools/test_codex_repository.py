"""Exercise the launcher with real Git remotes and a local fake Codex binary."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parent / "codex-repository.sh"


class CodexRepositoryLauncherTest(unittest.TestCase):
    def test_remote_cd_credentials_and_absent_repository(self):
        with tempfile.TemporaryDirectory(prefix="sub2api-repository-") as directory:
            root = Path(directory)
            project = root / "project with spaces"
            project.mkdir()
            subprocess.run(["git", "init", "-q", str(project)], check=True)
            subprocess.run([
                "git", "-C", str(project), "remote", "add", "origin",
                "https://test-user:test-secret@github.com/Owner/Repo.git?token=test-secret#branch",
            ], check=True)
            binary = root / "codex"
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "${SUB2API_REPOSITORY-unset}"\nprintf "%s\\n" "$@"\n')
            binary.chmod(0o755)
            env = {**os.environ, "PATH": str(root) + os.pathsep + os.environ["PATH"], "SUB2API_REPOSITORY": "stale"}

            for cd_args in (["-C", str(project)], ["--cd=" + str(project)]):
                result = subprocess.run(["sh", str(SCRIPT), *cd_args, "--model", "test-model"], cwd=root, env=env, capture_output=True, text=True, check=True)
                lines = result.stdout.splitlines()
                self.assertEqual(lines[0], "https://github.com/Owner/Repo.git")
                self.assertEqual(lines[1:], [*cd_args, "--model", "test-model"])

            result = subprocess.run(["sh", str(SCRIPT)], cwd=root, env=env, capture_output=True, text=True, check=True)
            self.assertEqual(result.stdout.strip(), "unset")
            subprocess.run(["git", "-C", str(project), "remote", "remove", "origin"], check=True)
            result = subprocess.run(["sh", str(SCRIPT)], cwd=project, env=env, capture_output=True, text=True, check=True)
            self.assertEqual(result.stdout.strip(), "unset")


if __name__ == "__main__":
    unittest.main()
