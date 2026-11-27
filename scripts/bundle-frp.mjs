import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const version = '0.72.0';
const checksums = {
  'darwin_amd64.tar.gz': '60439dc6bf794c4182a1a4cac147d642f839d55201607208979eb23aecd09be9',
  'darwin_arm64.tar.gz': '6e8576c5b6b9914a86850f6ff4fb81c5a09e706f91f70f64d5f398558bdc9db8',
  'linux_amd64.tar.gz': 'daf719b32ca2778770bb92955e231a07414d754378b9af328aa2642862479804',
  'linux_arm64.tar.gz': '02180b857480302fd934e9ddbd50fe2f73229f99062f88756abc6de9df5e95cf',
  'windows_amd64.zip': '5197c1b69564089cdc229fc1260a0e25db1e722b1b7db45fe1542c3c43edf9ae',
  'windows_arm64.zip': 'e4b83b66c4045971dd9a47cdef16dd02af471aaa6667dd303e63b1a669162927',
};

const windows = process.platform === 'win32';
const platform = windows ? 'windows' : process.platform;
const architecture = process.arch === 'x64' ? 'amd64' : process.arch;
const extension = windows ? 'zip' : 'tar.gz';
const target = platform + '_' + architecture + '.' + extension;
const checksum = checksums[target];
if (!checksum) throw new Error('Unsupported frp build target: ' + target);

const root = fileURLToPath(new URL('..', import.meta.url));
const folder = 'frp_' + version + '_' + platform + '_' + architecture;
const archive = folder + '.' + extension;
const endpoint = 'https://github.com/fatedier/frp/releases/download/v' + version + '/' + archive;
const response = await fetch(endpoint, { signal: AbortSignal.timeout(90_000) });
if (!response.ok) throw new Error('frp download failed: HTTP ' + response.status);
const data = Buffer.from(await response.arrayBuffer());
if (createHash('sha256').update(data).digest('hex') !== checksum) {
  throw new Error('frp archive checksum mismatch for ' + archive);
}

const temporary = await mkdtemp(join(tmpdir(), 'minecraft-proxy-frp-'));
try {
  const archived = join(temporary, archive);
  const client = windows ? 'frpc.exe' : 'frpc';
  const binaryDirectory = join(root, 'internal', 'relay', 'bin');
  await writeFile(archived, data);
  execFileSync(windows ? 'tar.exe' : 'tar', [
    '-xf', archived, '-C', temporary, folder + '/' + client, folder + '/LICENSE',
  ]);
  await mkdir(binaryDirectory, { recursive: true });
  await mkdir(join(root, 'build'), { recursive: true });
  await copyFile(join(temporary, folder, client), join(binaryDirectory, client));
  await copyFile(join(temporary, folder, 'LICENSE'), join(root, 'build', 'frp-LICENSE'));
  console.log('Bundled ' + client + ' ' + version + ' for ' + platform + '/' + architecture);
} finally {
  await rm(temporary, { recursive: true, force: true });
}
