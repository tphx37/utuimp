"""Check release package metadata without network or package manager access."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location("package_manifests", Path(__file__).with_name("package-manifests.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PackageManifestsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.assets = self.root / "assets"
        self.assets.mkdir()
        sums = []
        for name in ("utuimp_linux_amd64.tar.gz", "utuimp_windows_amd64.zip"):
            data = name.encode()
            (self.assets / name).write_bytes(data)
            sums.append(f"{hashlib.sha256(data).hexdigest()}  {name}")
        (self.assets / "checksums.txt").write_text("\n".join(sums))
        self.output = self.root / "output"

    def test_generates_installable_metadata(self):
        module.generate("v4.1.0", self.assets, self.output)
        with zipfile.ZipFile(self.output / "packaging.zip") as archive:
            name = "winget/manifests/t/tphx37/utuimp/4.1.0/tphx37.utuimp.installer.yaml"
            manifest = json.loads(archive.read(name))
            self.assertEqual(manifest["NestedInstallerType"], "portable")
            self.assertEqual(manifest["NestedInstallerFiles"][0]["RelativeFilePath"], "utuimp.exe")
            digest = hashlib.sha256((self.assets / "utuimp_windows_amd64.zip").read_bytes()).hexdigest().upper()
            self.assertEqual(manifest["Installers"][0]["InstallerSha256"], digest)
            self.assertIn("/v4.1.0/", manifest["Installers"][0]["InstallerUrl"])
            self.assertIn("aur/utuimp-bin/.SRCINFO", archive.namelist())
        self.assertIn("pkgver=4.1.0", (self.output / "aur/utuimp-bin/PKGBUILD").read_text())

    def test_rejects_tampered_archive(self):
        (self.assets / "utuimp_windows_amd64.zip").write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            module.generate("v4.1.0", self.assets, self.output)
        self.assertFalse(self.output.exists())

    def test_rejects_unsafe_or_prerelease_tag(self):
        for tag in ("v4.1.0-rc1", "../4.1.0", "v4.1.0;command"):
            with self.assertRaises(ValueError):
                module.generate(tag, self.assets, self.output)


if __name__ == "__main__":
    unittest.main()
