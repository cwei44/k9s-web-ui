#!/usr/bin/env python3
"""Exercise the image's port 80 server and real Unix bridge with a fake host CLI."""
import base64
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid

root = Path(__file__).resolve().parents[1]
image = os.environ.get('K9S_WEB_UI_IMAGE', 'k9s-web-ui:local')
name = 'k9s-ui-smoke-' + uuid.uuid4().hex[:12]
with tempfile.TemporaryDirectory(prefix='k9s-ui-smoke-') as directory:
    temp = Path(directory)
    (temp / 'socket').mkdir()
    (temp / 'state').mkdir()
    (temp / 'state' / 'password').write_text('smoke-test-password')
    (temp / 'current-context').write_text('first-cluster')
    client = temp / 'kubectl'
    client.write_text('#!' + sys.executable + '\n' + '''import json, os, sys
from pathlib import Path
args = sys.argv[1:]
if 'current-context' in args:
    print((Path(__file__).parent / 'current-context').read_text())
elif '/version' in args:
    print(json.dumps({'gitVersion':'v1.35.0'}))
elif 'logs' in args:
    print('host kubectl log output')
elif 'yaml' in args:
    print('apiVersion: v1')
else:
    print(json.dumps({'items':[]}))
''')
    client.chmod(0o755)
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    helper = subprocess.Popen([str(root / 'k9s-web-ui'), 'host-bridge', '--kubectl', str(client),
        '--socket', str(temp / 'socket' / 'bridge.sock')])
    def request(path, authenticated=True):
        req = urllib.request.Request(f'http://127.0.0.1:{port}/k9s/' + path)
        if authenticated:
            encoded = base64.b64encode(b'admin:smoke-test-password').decode()
            req.add_header('Authorization', 'Basic ' + encoded)
        with urllib.request.urlopen(req, timeout=30) as response:
            return response.read()
    try:
        for _ in range(100):
            if (temp / 'socket' / 'bridge.sock').exists():
                break
            if helper.poll() is not None:
                raise RuntimeError('host helper exited')
            time.sleep(.1)
        subprocess.run(['docker', 'run', '-d', '--name', name, '--read-only', '--cap-drop', 'ALL',
            '--cap-add', 'NET_BIND_SERVICE', '--security-opt', 'no-new-privileges',
            '-p', f'127.0.0.1:{port}:80',
            '--mount', f'type=bind,src={temp / "socket"},dst=/host,readonly',
            '--mount', f'type=bind,src={temp / "state"},dst=/state', image,
            'serve', '--host-bridge', '/host/bridge.sock', '--listen', '0.0.0.0:80',
            '--password-file', '/state/password'], check=True, stdout=subprocess.DEVNULL)
        for attempt in range(100):
            try:
                page = request('')
                break
            except (OSError, urllib.error.URLError):
                if attempt == 99:
                    raise
                time.sleep(.1)
        assert b'/k9s/gpu.js' in page
        try:
            request('', authenticated=False)
            raise AssertionError('unauthenticated access accepted')
        except urllib.error.HTTPError as error:
            assert error.code == 401
        snapshot = json.loads(request('api/snapshot'))
        assert snapshot['connected'] and snapshot['context'] == 'first-cluster', snapshot
        assert not snapshot['errors'], snapshot
        assert request('api/logs?kind=Pods&namespace=default&name=test-pod')
        assert request('api/manifest?kind=Pods&namespace=default&name=test-pod')
        assert json.loads(request('api/gpu')) is not None
        (temp / 'current-context').write_text('second-cluster')
        time.sleep(5.1)
        snapshot = json.loads(request('api/snapshot'))
        assert snapshot['connected'] and snapshot['context'] == 'second-cluster', snapshot
        print('PASS: container port 80, authentication, host kubectl bridge, read-only views, context switching')
    finally:
        subprocess.run(['docker', 'logs', name], check=False)
        subprocess.run(['docker', 'rm', '-f', name], check=False, stdout=subprocess.DEVNULL)
        helper.terminate()
        helper.wait(timeout=25)
