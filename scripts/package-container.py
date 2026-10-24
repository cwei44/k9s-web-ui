#!/usr/bin/env python3
"""Create an offline Docker deployment archive without host credentials."""
import gzip
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

root = Path(__file__).resolve().parents[1]
image = os.environ.get('K9S_WEB_UI_IMAGE', 'k9s-web-ui:local')
if any(c not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-/:@' for c in image):
    raise SystemExit('Unsupported image name')
architecture = subprocess.check_output(['docker', 'image', 'inspect', '--format', '{{.Architecture}}', image], text=True).strip()
if architecture != 'amd64':
    raise SystemExit('This package target currently requires an amd64 image')
dist = root / 'dist'
bundle = dist / 'k9s-web-ui-container'
if bundle.exists():
    shutil.rmtree(bundle)
(bundle / 'scripts').mkdir(parents=True)
for name in ('README.md', 'LICENSE', 'NOTICE'):
    shutil.copyfile(root / name, bundle / name)
shutil.copytree(root / 'licenses', bundle / 'licenses')
for script in ('install-container.sh', 'update-container.sh'):
    shutil.copyfile(root / 'scripts' / script, bundle / 'scripts' / script)
    (bundle / 'scripts' / script).chmod(0o755)
source = bundle / 'src'
source.mkdir()
for file in [root / 'Dockerfile', root / '.dockerignore', root / 'go.mod', root / 'LICENSE', root / 'NOTICE', *root.glob('*.go')]:
    shutil.copyfile(file, source / file.name)
shutil.copytree(root / 'web', source / 'web')
shutil.copytree(root / 'licenses', source / 'licenses')
with gzip.GzipFile(filename=str(bundle / 'image.tar.gz'), mode='wb', mtime=0) as output:
    process = subprocess.Popen(['docker', 'save', image], stdout=subprocess.PIPE)
    shutil.copyfileobj(process.stdout, output)
    process.stdout.close()
    if process.wait() != 0:
        raise SystemExit('docker save failed')
(bundle / 'install.sh').write_text('''#!/bin/sh
set -eu
cd -- "$(dirname -- "$0")"
[ "$(id -u)" = 0 ] || { echo 'Run as root on the Docker host.' >&2; exit 1; }
sha256sum -c SHA256SUMS
docker load -i image.tar.gz
K9S_WEB_UI_SKIP_BUILD=1 K9S_WEB_UI_IMAGE=''' + "'" + image + "'" + ''' ./scripts/install-container.sh "$@"
''')
(bundle / 'install.sh').chmod(0o755)
(bundle / 'update.sh').write_text((bundle / 'install.sh').read_text().replace('./scripts/install-container.sh', './scripts/update-container.sh'))
(bundle / 'update.sh').chmod(0o755)
files = sorted(p for p in bundle.rglob('*') if p.is_file())
(bundle / 'SHA256SUMS').write_text(''.join(
    hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + str(p.relative_to(bundle)) + '\n'
    for p in files
))
archive = dist / 'k9s-web-ui-container-linux-amd64.tar.gz'
def normalized(info):
    info.uid = info.gid = 0
    info.uname = info.gname = 'root'
    info.mtime = 0
    return info
with tarfile.open(archive, 'w:gz') as tar:
    tar.add(bundle, arcname=bundle.name, filter=normalized)
checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_name(archive.name + '.sha256').write_text(checksum + '  ' + archive.name + '\n')
print(archive)
