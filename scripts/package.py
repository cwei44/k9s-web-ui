#!/usr/bin/env python3
"""Create a credential-free native package from an explicit allowlist."""
import gzip
import hashlib
from pathlib import Path
import shutil
import tarfile

root = Path(__file__).resolve().parents[1]
dist = root / 'dist'
bundle = dist / 'k9s-web-ui-linux-amd64'
if bundle.exists():
    shutil.rmtree(bundle)
bundle.mkdir(parents=True)
for name in ('k9s-web-ui', 'README.md', 'LICENSE', 'NOTICE', 'apache-example.conf'):
    shutil.copyfile(root / name, bundle / name)
(bundle / 'k9s-web-ui').chmod(0o755)
shutil.copytree(root / 'licenses', bundle / 'licenses')
# Include the source of this executable; no cluster settings, credentials,
# downloaded kubectl, package caches, or local test output enter the archive.
source = bundle / 'src'
source.mkdir()
for file in [root / 'go.mod', *root.glob('*.go')]:
    shutil.copyfile(file, source / file.name)
shutil.copytree(root / 'web', source / 'web')
files = sorted(p for p in bundle.rglob('*') if p.is_file())
(bundle / 'SHA256SUMS').write_text(''.join(
    hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + str(p.relative_to(bundle)) + '\n'
    for p in files
))
def normalized(info):
    info.uid = info.gid = 0
    info.uname = info.gname = 'root'
    info.mtime = 0
    return info
archive = dist / (bundle.name + '.tar.gz')
with archive.open('wb') as raw:
    with gzip.GzipFile(filename='', fileobj=raw, mode='wb', mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w|') as tar:
            tar.add(bundle, arcname=bundle.name, filter=normalized)
checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
(archive.with_name(archive.name + '.sha256')).write_text(checksum + '  ' + archive.name + '\n')
print(archive)
