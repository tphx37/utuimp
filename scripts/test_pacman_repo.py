"""Integration check for the signed static pacman repository builder."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest


@unittest.skipUnless(all(shutil.which(c) for c in ('gpg', 'gpgconf', 'repo-add', 'bsdtar')), 'needs Arch packaging tools and GnuPG')
class PacmanRepositoryTest(unittest.TestCase):
    def test_signed_repository_and_static_aliases(self):
        with tempfile.TemporaryDirectory(prefix='utuimp-repo-test-') as tmp:
            root = Path(tmp)
            home = root / 'gnupg'
            home.mkdir(mode=0o700)
            env = dict(os.environ, GNUPGHOME=str(home))
            def run(*args):
                result = subprocess.run(args, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, f'{args[0]}: {result.stdout}\n{result.stderr}')
                return result.stdout
            try:
                run('gpg', '--batch', '--pinentry-mode', 'loopback', '--passphrase', '', '--quick-generate-key',
                    'utuimp Package Test <test@invalid>', 'ed25519', 'sign', '1d')
                raw = run('gpg', '--with-colons', '--list-secret-keys')
                key = next(line.split(':')[9] for line in raw.splitlines() if line.startswith('fpr:'))
                env['UTUIMP_SIGNING_KEY'] = key
                package_root = root / 'pkg'
                package_root.mkdir()
                (package_root / '.PKGINFO').write_text(
                    'pkgname = utuimp\npkgbase = utuimp\npkgver = 4.0.1-1\npkgdesc = Test package\n'
                    'url = https://github.com/tphx37/utuimp\nbuilddate = ' + str(int(time.time())) + '\n'
                    'packager = Test\nsize = 1\narch = x86_64\nlicense = GPL-3.0-or-later\n')
                package = root / 'utuimp_4.0.1_amd64.pkg.tar.zst'
                run('bsdtar', '--zstd', '-cf', str(package), '-C', str(package_root), '.PKGINFO')
                site = root / 'site'
                script = str(Path(__file__).with_name('build-pacman-repo.sh').resolve())
                for _ in range(2):
                    run('sh', script, str(site), str(package))
                    for name in ('utuimp.db', 'utuimp.files', package.name):
                        path = site / 'x86_64' / name
                        self.assertFalse(path.is_symlink())
                        run('gpg', '--verify', str(path) + '.sig', str(path))
                self.assertTrue((site / 'utuimp-repo.asc').is_file())
                self.assertTrue((site / '.nojekyll').is_file())
            finally:
                subprocess.run(['gpgconf', '--kill', 'gpg-agent'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == '__main__':
    unittest.main()
