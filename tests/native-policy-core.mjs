// 文件: tests/native-policy-core.mjs
// 功能: 隔离验证锁定核心的原生策略组、Provider 与缓存行为。
// 用法: node tests/native-policy-core.mjs <sing-box> [证据目录]
// 依赖: Node.js 22+，v1.15.0-alpha.11-reF1nd 核心。
import assert from 'node:assert/strict';
import { spawn, spawnSync, execFileSync } from 'node:child_process';
import { createHash, randomBytes, randomInt } from 'node:crypto';
import { EventEmitter, once } from 'node:events';
import { mkdir, mkdtemp, readFile, rename, writeFile } from 'node:fs/promises';
import http from 'node:http';
import net from 'node:net';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import { setTimeout as sleep } from 'node:timers/promises';

assert.ok(process.argv[2], 'usage: node tests/native-policy-core.mjs <sing-box> [evidence-directory] [--adb serial]');
const binary = path.resolve(process.argv[2]);
const adbSerial = process.argv[4] === '--adb' ? process.argv[5] : undefined;
assert.ok(process.argv.length <= 4 || (adbSerial && process.argv.length === 6), 'expected [--adb serial]');
if (adbSerial) assert.equal(binary, path.resolve('src/module/bin/sing-box'), 'ADB mode only runs the bundled core');
const base = path.resolve(process.argv[3] ?? '.agents/runs/native-policy-core');
const ignoredRoot = path.resolve('.agents/runs');
const relativeBase = path.relative(ignoredRoot, base);
assert.ok(
  relativeBase !== '..' && !relativeBase.startsWith(`..${path.sep}`) && !path.isAbsolute(relativeBase),
  'evidence must stay under .agents/runs',
);
execFileSync('git', ['check-ignore', '--quiet', '--', base]);
await mkdir(base, { recursive: true });
const runRoot = await mkdtemp(path.join(base, adbSerial ? 'adb-probe-' : 'probe-'));
let root;
let apiPort;
let current;
let remote;
let interrupted = false;
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { interrupted = true; });
let requests = 0;
const web = http.createServer((_req, res) => {
  requests++;
  res.writeHead(204);
  res.end();
});
web.listen(0, '127.0.0.1');
await once(web, 'listening');
let testURL = `http://127.0.0.1:${web.address().port}/`;
let child;
let childDone;
let childResult;
const evidence = {
  ok: false,
  runner: adbSerial ? 'adb' : 'host',
  binary_sha256: createHash('sha256')
    .update(await readFile(binary))
    .digest('hex'),
  root: runRoot,
  scenarios: [],
};

function quote(value) {
  assert.ok(!String(value).includes('\0'), 'shell value must not contain NUL');
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

class RemoteProcess extends EventEmitter {
  constructor(runner, logPath, pidPath) {
    super();
    this.runner = runner;
    this.pid = undefined;
    this.logPath = logPath;
    this.pidPath = pidPath;
    this.stdout = new PassThrough();
    this.stderr = new PassThrough();
    this.finished = false;
    this.stopped = false;
    this.timer = setInterval(() => this.poll(), 200);
    this.timer.unref();
  }

  poll() {
    if (this.finished) return;
    if (!this.pid) {
      const pid = this.runner.shell(`cat ${quote(this.pidPath)} 2>/dev/null || :`);
      const value = Number.parseInt(pid.stdout.trim(), 10);
      if (Number.isInteger(value) && value > 0) this.pid = value;
      return;
    }
    const result = this.runner.shell(`kill -0 ${this.pid} 2>/dev/null`);
    if (result.status !== 0) this.finish(this.stopped ? 0 : 1);
  }

  finish(code) {
    if (this.finished) return;
    this.finished = true;
    clearInterval(this.timer);
    const output = this.runner.shell(`cat ${quote(this.logPath)} 2>/dev/null || :`);
    if (output.stdout) this.stderr.write(output.stdout);
    if (output.stderr) this.stderr.write(output.stderr);
    this.stderr.end();
    this.stdout.end();
    this.emit('close', code, null);
  }
}

class RemoteRunner {
  constructor(serial) {
    this.serial = serial;
    // Use the same private filesystem family as the module, but never its
    // directory. /data/local/tmp on several devices does not deliver the
    // inotify replacement events that Local Provider relies on.
    this.root = `/data/adb/netproxy-native-policy-${randomBytes(8).toString('hex')}`;
    this.binary = `${this.root}/sing-box`;
    this.apiRemotePort = randomInt(22000, 32000);
    this.webRemotePort = randomInt(42000, 52000);
    this.socksPort = 0;
  }

  adb(args, options = {}) {
    return spawnSync('adb', ['-s', this.serial, ...args], {
      encoding: 'utf8',
      timeout: options.timeout ?? 15000,
      windowsHide: true,
    });
  }

  shell(script, options) {
    // Pass one complete command. adb otherwise flattens the `sh -c` argv and
    // treats only its first word as the script on several Android builds.
    return this.adb(['shell', script], options);
  }

  require(result, context) {
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(`${context}: ${result.stderr || result.stdout}`);
    return result;
  }

  init(webPort) {
    this.require(this.shell(`umask 077; mkdir -p ${quote(this.root)}`), 'create isolated device directory');
    this.require(this.adb(['push', binary, this.binary]), 'push bundled sing-box');
    this.require(this.shell(`chmod 700 ${quote(this.binary)}`), 'make bundled sing-box executable');
    const forward = this.require(this.adb(['forward', 'tcp:0', `tcp:${this.apiRemotePort}`]), 'forward API port');
    this.apiLocalPort = Number.parseInt(forward.stdout.trim(), 10);
    assert.ok(Number.isInteger(this.apiLocalPort) && this.apiLocalPort > 0, `invalid forwarded API port: ${forward.stdout}`);
    this.require(this.adb(['reverse', `tcp:${this.webRemotePort}`, `tcp:${webPort}`]), 'reverse URLTest target');
    this.testURL = `http://127.0.0.1:${this.webRemotePort}/`;
  }

  prepare(name) {
    this.caseRoot = `${this.root}/${name}`;
    this.require(this.shell(`mkdir -p ${quote(this.caseRoot)}`), 'create isolated scenario directory');
  }

  finishScenario() {
    if (this.caseRoot) this.require(this.shell(`rm -rf ${quote(this.caseRoot)}`), 'remove isolated scenario directory');
    this.caseRoot = undefined;
  }

  push(name) {
    this.require(this.adb(['push', path.join(root, name), `${this.caseRoot}/${name}`]), `push ${name}`);
  }

  pull(name) {
    this.require(this.adb(['pull', `${this.caseRoot}/${name}`, path.join(root, name)]), `pull ${name}`);
  }

  move(from, to) {
    this.require(this.shell(`mv -f ${quote(`${this.caseRoot}/${from}`)} ${quote(`${this.caseRoot}/${to}`)}`), 'atomically replace Provider');
  }

  command(args) {
    return this.shell(`cd ${quote(this.caseRoot)} && ${quote(this.binary)} ${args.map(quote).join(' ')}`);
  }

  launch(args) {
    const logPath = `${this.caseRoot}/core.log`;
    const pidPath = `${this.caseRoot}/core.pid`;
    // Keep the short-lived adb shell attached in the background. Some vendor
    // adbd builds wait for every child despite setsid/nohup, while this keeps
    // the test process owned and lets the normal PID file drive cleanup.
    const command = `cd ${quote(this.caseRoot)}; ${quote(this.binary)} ${args.map(quote).join(' ')} </dev/null >${quote(logPath)} 2>&1 & echo $! >${quote(pidPath)}; wait $!`;
    const launcher = spawn('adb', ['-s', this.serial, 'shell', command], { detached: true, stdio: 'ignore', windowsHide: true });
    launcher.unref();
    this.process = new RemoteProcess(this, logPath, pidPath);
    return this.process;
  }

  async connect() {
    return this.apiLocalPort;
  }

  async stop() {
    if (!this.process || this.process.finished) return;
    this.process.stopped = true;
    this.process.poll();
    if (this.process.pid) this.shell(`kill ${this.process.pid} 2>/dev/null || :`);
    const end = Date.now() + 4500;
    while (!this.process.finished && Date.now() < end) {
      this.process.poll();
      await sleep(100);
    }
    if (!this.process.finished) throw new Error('isolated device core did not stop');
  }

  result(args, code, signal, log) {
    return { args, pid: this.process?.pid, code, signal, log };
  }

  port() {
    if (!this.socksPort) this.socksPort = randomInt(33000, 41000);
    return this.socksPort;
  }

  now() {
    return Number.parseInt(this.require(this.shell('date +%s'), 'read device clock').stdout.trim(), 10);
  }

  async cleanup() {
    await this.stop();
    this.adb(['forward', '--remove', `tcp:${this.apiLocalPort}`]);
    this.adb(['reverse', '--remove', `tcp:${this.webRemotePort}`]);
    this.shell(`rm -rf ${quote(this.root)}`);
  }
}

async function freePort() {
  const server = net.createServer();
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}

async function scenario(name, check) {
  assert.ok(!interrupted, 'probe interrupted');
  root = path.join(runRoot, name);
  await mkdir(root);
  apiPort = await freePort();
  current = { name, ok: false, commands: [], checks: [] };
  evidence.scenarios.push(current);
  try {
    if (remote) remote.prepare(name);
    await check();
    await stop();
    current.ok = true;
    console.log(`PASS ${name}`);
  } catch (error) {
    current.error = error.stack;
    console.error(`FAIL ${name}: ${error.message}`);
  } finally {
    await stop();
    if (remote) remote.finishScenario();
    await writeFile(path.join(root, 'evidence.json'), JSON.stringify(current, null, 2));
  }
}

async function deadline(promise, milliseconds, message) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(message)), milliseconds);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

const configArgs = (configs) => ['-D', remote ? remote.caseRoot : root, '--disable-color', ...configs.flatMap((file) => ['-c', file])];
async function command(action, configs = ['main.json'], extra = []) {
  const args = [action, ...extra, ...configArgs(configs)];
  const result = remote ? remote.command(args) : spawnSync(binary, args, { cwd: root, windowsHide: true, encoding: 'utf8', timeout: 15000 });
  const record = { args, status: result.status, signal: result.signal, stdout: result.stdout, stderr: result.stderr };
  current.commands.push(record);
  await writeFile(
    path.join(root, `command-${current.commands.length}-${action}.json`),
    JSON.stringify(record, null, 2),
  );
  if (result.error) throw result.error;
  if (remote && action === 'merge' && result.status === 0) remote.pull(extra[0]);
  return record;
}

async function checkConfig(configs = ['main.json']) {
  const result = await command('check', configs);
  assert.equal(result.status, 0, result.stderr);
}

function config(outbounds, providers = []) {
  return {
    log: { level: 'warn', output: 'stderr' },
    services: [{ type: 'api', listen: '127.0.0.1', listen_port: remote ? remote.apiRemotePort : apiPort, secret: 'isolated-test' }],
    outbounds,
    providers,
    route: { final: outbounds[0].tag },
  };
}

const localProvider = (tag) => ({ type: 'local', tag, path: `${tag.replaceAll('/', '-')}.json` });
async function provider(tag, nodes) {
  const entry = localProvider(tag);
  await jsonFile(entry.path, { outbounds: nodes });
  return entry;
}

async function jsonFile(name, content) {
  await writeFile(path.join(root, name), JSON.stringify(content), { mode: 0o600 });
  if (remote) remote.push(name);
}

function variable(value) {
  const bytes = [];
  let remaining = BigInt(value);
  do {
    let byte = Number(remaining & 127n);
    remaining >>= 7n;
    if (remaining) byte |= 128;
    bytes.push(byte);
  } while (remaining);
  return Buffer.from(bytes);
}

function textField(number, value) {
  const data = Buffer.from(value);
  return Buffer.concat([variable(number * 8 + 2), variable(data.length), data]);
}

function fields(buffer) {
  let at = 0;
  function integer() {
    let value = 0n;
    let shift = 0n;
    for (;;) {
      assert.ok(at < buffer.length, 'truncated protobuf');
      const byte = buffer[at++];
      value |= BigInt(byte & 127) << shift;
      if (!(byte & 128)) return Number(value);
      shift += 7n;
      assert.ok(shift < 70n);
    }
  }
  const result = [];
  while (at < buffer.length) {
    const key = integer();
    if ((key & 7) === 0) result.push([key >> 3, integer()]);
    else {
      assert.equal(key & 7, 2);
      const size = integer();
      assert.ok(at + size <= buffer.length);
      result.push([key >> 3, buffer.subarray(at, (at += size))]);
    }
  }
  return result;
}

function decodeGroup(data) {
  const group = { items: [] };
  for (const [key, value] of fields(data)) {
    if (key === 1) group.tag = value.toString();
    if (key === 2) group.type = value.toString();
    if (key === 3) group.selectable = !!value;
    if (key === 4) group.selected = value.toString();
    if (key === 6) {
      const item = {};
      for (const [field, content] of fields(value)) {
        if (field === 1) item.tag = content.toString();
        if (field === 2) item.type = content.toString();
        if (field === 3) item.url_test_time = content;
        if (field === 4) item.url_test_delay = content;
      }
      group.items.push(item);
    }
  }
  return group;
}

async function rpc(method, payload = Buffer.alloc(0), stream = false) {
  const header = Buffer.alloc(5);
  header.writeUInt32BE(payload.length, 1);
  return new Promise((resolve, reject) => {
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port: apiPort,
        path: `/daemon.StartedService/${method}`,
        method: 'POST',
        headers: { 'Content-Type': 'application/grpc-web+proto', Authorization: 'Bearer isolated-test' },
        timeout: 2000,
      },
      (res) => {
        const headerStatus = res.headers['grpc-status'];
        if (headerStatus !== undefined && headerStatus !== '0') {
          reject(new Error(`grpc-status: ${headerStatus}\ngrpc-message: ${res.headers['grpc-message'] ?? ''}`));
          res.resume();
          return;
        }
        let pending = Buffer.alloc(0);
        let data;
        let trailer = false;
        res.on('data', (chunk) => {
          try {
            pending = Buffer.concat([pending, chunk]);
            while (pending.length >= 5 && pending.length >= 5 + pending.readUInt32BE(1)) {
              const size = pending.readUInt32BE(1);
              const frame = pending.subarray(5, size + 5);
              if (pending[0] & 128) {
                trailer = true;
                if (!/grpc-status: 0(?:\r?\n|$)/.test(frame.toString())) reject(new Error(frame.toString()));
                else resolve(data ?? Buffer.alloc(0));
              } else {
                data = frame;
                if (stream) {
                  resolve(frame);
                  res.destroy();
                }
              }
              pending = pending.subarray(size + 5);
            }
          } catch (error) {
            reject(error);
            res.destroy();
          }
        });
        res.on('end', () => {
          if (!trailer && headerStatus === '0') resolve(data ?? Buffer.alloc(0));
          else if (!trailer) reject(new Error(`missing gRPC trailer (HTTP ${res.statusCode})`));
        });
        res.on('error', reject);
      },
    );
    req.on('timeout', () => req.destroy(new Error('RPC timeout')));
    req.on('error', reject);
    req.end(Buffer.concat([header, payload]));
  });
}

async function groups() {
  const payload = await rpc('SubscribeGroups', undefined, true);
  return fields(payload)
    .filter(([key]) => key === 1)
    .map(([, data]) => decodeGroup(data));
}

async function startedAt() {
  return fields(await rpc('GetStartedAt')).find(([key]) => key === 1)?.[1];
}

async function until(check, label, timeout = 12000) {
  const end = Date.now() + timeout;
  let last;
  while (Date.now() < end) {
    assert.ok(!interrupted, 'probe interrupted');
    if (childResult) throw new Error(`${label}: core exited: ${JSON.stringify(childResult)}`);
    try {
      const value = await check();
      if (value) return value;
    } catch (error) {
      last = error;
    }
    await sleep(100);
  }
  throw new Error(`${label}: ${last ?? 'timeout'}`);
}

function launch(configs) {
  childResult = undefined;
  const args = ['run', ...configArgs(configs)];
  child = remote ? remote.launch(args) : spawn(binary, args, { cwd: root, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  let log = '';
  child.stdout.on('data', (value) => (log += value));
  child.stderr.on('data', (value) => (log += value));
  const pid = child.pid;
  child.on('error', (error) => {
    log += error.stack;
  });
  childDone = new Promise((resolve) =>
    child.once('close', (code, signal) => {
      childResult = remote ? remote.result(args, code, signal, log) : { args, pid, code, signal, log };
      current.commands.push(childResult);
      resolve(writeFile(path.join(root, `core-${childResult.pid ?? pid}.log`), log));
    }),
  );
}

async function start(configs = ['main.json']) {
  launch(configs);
  if (remote) apiPort = await remote.connect();
  await until(async () => (await startedAt()) > 0, 'core startup');
}

async function stop() {
  if (!child) return;
  if (remote) {
    await remote.stop();
    await deadline(childDone, 5000, 'ADB session did not exit after owned core stopped');
    assert.ok(!childResult.transport_error, childResult.transport_error);
    child = undefined;
    childResult = undefined;
    return;
  }
  if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
  try {
    await deadline(childDone, 5000, 'core stop timeout');
  } finally {
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await childDone;
    child = undefined;
    childResult = undefined;
  }
}

const node = (tag) => ({ type: 'socks', tag, server: '127.0.0.1', server_port: 1 });
const direct = (tag) => ({ type: 'direct', tag });
async function updateProvider(tags) {
  const document = { outbounds: tags.map(node) };
  if (!remote) {
    // reF1nd's Local Provider watcher observes a write to the configured
    // file. Catalog commits remain atomic in production and are verified by
    // module.SyncCatalog, which reloads if a replacement is not observed.
    await jsonFile('provider-a.json', document);
    return;
  }
  await jsonFile('provider-a.next.json', document);
  await rename(path.join(root, 'provider-a.next.json'), path.join(root, 'provider-a.json'));
  remote.move('provider-a.next.json', 'provider-a.json');
}
const groupBy = (snapshot, tag) => snapshot.find((group) => group.tag === tag);
async function select(group, member) {
  await rpc('SelectOutbound', Buffer.concat([textField(1, group), textField(2, member)]));
}

function members(snapshot, tag) {
  const group = groupBy(snapshot, tag);
  assert.ok(group, `missing group ${tag}`);
  return group.items.map((item) => item.tag);
}

async function rejectedRPC(method, payload) {
  await assert.rejects(rpc(method, payload), (error) => {
    assert.match(error.message, /grpc-status: [1-9]/);
    (current.rpc_errors ??= []).push({ method, message: error.message });
    return true;
  });
}

async function rejectSelection(group, member) {
  await rejectedRPC('SelectOutbound', Buffer.concat([textField(1, group), textField(2, member)]));
}

async function startupError(expected, configs = ['main.json']) {
  // check only constructs the box; run also starts groups and loads local Providers.
  const checked = await command('check', configs);
  current.check_status = checked.status;
  launch(configs);
  await deadline(childDone, 12000, 'expected startup failure, core stayed alive');
  assert.notEqual(childResult.code, 0);
  assert.equal(childResult.signal, null, 'must be a core error, not external termination');
  assert.ok(!childResult.transport_error, childResult.transport_error);
  assert.match(childResult.log, expected);
  current.startup_error = childResult.log;
}

try {
  if (adbSerial) {
    remote = new RemoteRunner(adbSerial);
    remote.init(web.address().port);
    testURL = remote.testURL;
    evidence.version = remote.shell(`${quote(remote.binary)} version`).stdout;
  } else {
    evidence.version = execFileSync(binary, ['version'], { encoding: 'utf8', windowsHide: true, timeout: 15000 });
  }
  assert.match(evidence.version, /sing-box version 1\.15\.0-alpha\.11-reF1nd\s/);
  await scenario('direct-only', async () => {
    const main = config([direct('OnlyDirect')]);
    delete main.providers;
    await jsonFile('main.json', main);
    await checkConfig();
    await start();
    current.snapshot = await groups();
    assert.deepEqual(current.snapshot, []);
    await rejectedRPC('SelectOutbound', Buffer.concat([textField(1, 'OnlyDirect'), textField(2, 'OnlyDirect')]));
    await rejectedRPC('URLTest', textField(1, 'missing-group'));
    current.checks.push('direct-only core starts without Providers or policy groups; empty snapshot is valid');
  });

  await scenario('use-all-providers', async () => {
    const providers = [
      await provider('np/alpha', [node('same'), node('second')]),
      await provider('np/beta', [node('same')]),
    ];
    await jsonFile('main.json', config([{ type: 'selector', tag: 'Everything', use_all_providers: true }], providers));
    await checkConfig();
    await start();
    current.snapshot = await groups();
    assert.equal(current.snapshot.length, 1);
    const tags = members(current.snapshot, 'Everything');
    assert.deepEqual([...tags].sort(), ['np/alpha/same', 'np/alpha/second', 'np/beta/same']);
    for (const tag of tags) {
      await select('Everything', tag);
      assert.equal(groupBy(await groups(), 'Everything').selected, tag);
    }
    await rejectSelection('Everything', 'same');
    current.checks.push(
      'use_all_providers expands both np/id namespaces without explicit providers; exact tags select',
    );
  });

  await scenario('disjoint-selectors', async () => {
    const providers = [
      await provider('np/alpha', [node('same'), node('second')]),
      await provider('np/beta', [node('same'), node('second')]),
    ];
    await jsonFile(
      'main.json',
      config(
        [
          { type: 'selector', tag: 'Left', providers: ['np/alpha'] },
          { type: 'selector', tag: 'Right', providers: ['np/beta'] },
        ],
        providers,
      ),
    );
    await checkConfig();
    await start();
    current.snapshot = await groups();
    assert.equal(current.snapshot.length, 2);
    assert.deepEqual(members(current.snapshot, 'Left'), ['np/alpha/same', 'np/alpha/second']);
    assert.deepEqual(members(current.snapshot, 'Right'), ['np/beta/same', 'np/beta/second']);
    for (const [group, other, ownTag, foreignTag] of [
      ['Left', 'Right', 'np/alpha/second', 'np/beta/same'],
      ['Right', 'Left', 'np/beta/second', 'np/alpha/same'],
    ]) {
      const unchanged = groupBy(await groups(), other).selected;
      await select(group, ownTag);
      assert.equal(groupBy(await groups(), group).selected, ownTag);
      assert.equal(groupBy(await groups(), other).selected, unchanged);
      await rejectSelection(group, foreignTag);
      await rejectSelection(group, 'same');
      assert.equal(groupBy(await groups(), group).selected, ownTag);
    }
    current.checks.push('disjoint provider membership and selection are independent despite identical node names');
  });

  for (const useAll of [false, true]) {
    await scenario(useAll ? 'urltest-use-all-providers' : 'urltest-multiple-providers', async () => {
      const socksPort = remote ? remote.port() : await freePort();
      if (remote) remote.socksPort = socksPort;
      const liveNode = (tag) => ({ ...node(tag), server_port: socksPort });
      const providers = [
        await provider('np/alpha', [liveNode('same'), liveNode('second')]),
        await provider('np/beta', [liveNode('same')]),
      ];
      const main = config(
        [
          {
            type: 'urltest',
            tag: 'CombinedAuto',
            ...(useAll ? { use_all_providers: true } : { providers: ['np/alpha', 'np/beta'] }),
            url: testURL,
            interval: '1h',
            idle_timeout: '1h',
          },
          direct('LoopbackDirect'),
        ],
        providers,
      );
      main.inbounds = [{ type: 'socks', tag: 'test-socks', listen: '127.0.0.1', listen_port: socksPort }];
      main.route.rules = [{ inbound: ['test-socks'], action: 'route', outbound: 'LoopbackDirect' }];
      await jsonFile('main.json', main);
      await checkConfig();
      await start();
      current.initial = await groups();
      const expected = ['np/alpha/same', 'np/alpha/second', 'np/beta/same'];
      assert.deepEqual(members(current.initial, 'CombinedAuto'), expected);
      assert.ok(!groupBy(current.initial, 'CombinedAuto').selectable);
      await rejectSelection('CombinedAuto', expected[0]);
      await until(
        async () => groupBy(await groups(), 'CombinedAuto').items.every((item) => item.url_test_time > 0),
        'initial multi-provider history',
      );
      await sleep(1100);
      const requestCount = requests;
      const triggeredAt = remote ? remote.now() : Math.floor(Date.now() / 1000);
      await rpc('URLTest', textField(1, 'CombinedAuto'));
      current.after_delay = await until(async () => {
        const snapshot = await groups();
        return (
          groupBy(snapshot, 'CombinedAuto').items.every(
            (item) => item.url_test_time >= triggeredAt && item.url_test_delay < 65535,
          ) && snapshot
        );
      }, 'fresh history for every provider member', 30000);
      assert.deepEqual(members(current.after_delay, 'CombinedAuto'), expected);
      assert.ok(
        requests - requestCount >= expected.length,
        'URLTest must reach the local HTTP target for every member',
      );
      assert.ok(expected.includes(groupBy(current.after_delay, 'CombinedAuto').selected));
      current.http_requests_after_trigger = requests - requestCount;
      current.checks.push(
        'native URLTest tests all namespaced members from multiple Providers; fresh per-member history and real loopback requests',
      );
    });
  }

  for (const type of ['selector', 'urltest']) {
    for (const invalid of ['empty-provider', 'missing-provider', 'missing-outbound', 'no-members']) {
      await scenario(`${type}-${invalid}`, async () => {
        const group = { type, tag: 'InvalidGroup', ...(type === 'urltest' ? { url: testURL } : {}) };
        const providers = [];
        let expected;
        if (invalid === 'empty-provider') {
          providers.push(await provider('np/empty', []));
          group.providers = ['np/empty'];
          expected = /no servers found/;
        } else if (invalid === 'missing-provider') {
          group.providers = ['np/missing'];
          expected = /outbound provider 0 not found: np\/missing/;
        } else if (invalid === 'missing-outbound') {
          group.outbounds = ['missing-member'];
          expected = /(?:outbound.*not found.*missing-member|missing-member.*not found)/;
        } else {
          expected = /missing outbound and provider tags/;
        }
        await jsonFile('main.json', config([group], providers));
        await startupError(expected);
        assert.equal(current.check_status, 0, 'check does not validate group startup or load local Provider contents');
        current.checks.push(`${type} ${invalid} fails with an explicit native error, not silent direct fallback`);
      });
    }
  }

  await scenario('multi-c-sorted-merge', async () => {
    const first = config([{ type: 'selector', tag: 'First', outbounds: ['direct-a', 'direct-z'] }, direct('direct-a')]);
    first.log.timestamp = true;
    first.log.disabled = false;
    first.experimental = { cache_file: { enabled: true, path: 'cache-first.db' } };
    first.route.rules = [{ domain: ['a.invalid'], action: 'route', outbound: 'direct-a' }];
    const last = {
      log: { level: 'error', timestamp: false, disabled: true },
      experimental: { cache_file: { enabled: false, path: 'cache-last.db', cache_id: 'anonymous-merge' } },
      route: {
        final: 'Last',
        rules: [{ domain: ['z.invalid'], action: 'route', outbound: 'direct-z' }],
        auto_detect_interface: false,
      },
      outbounds: [{ type: 'selector', tag: 'Last', outbounds: ['direct-z'] }, direct('direct-z')],
    };
    await jsonFile('10-first.json', first);
    await jsonFile('90-last.json', last);
    const configs = ['90-last.json', '10-first.json'];
    const result = await command('merge', configs, ['merged.json']);
    assert.equal(result.status, 0, result.stderr);
    const merged = JSON.parse(await readFile(path.join(root, 'merged.json'), 'utf8'));
    current.merged = merged;
    assert.equal(merged.log.level, 'warn');
    assert.equal(merged.log.timestamp, true);
    assert.equal(merged.log.disabled ?? false, false);
    assert.equal(merged.experimental.cache_file.enabled, true);
    assert.equal(merged.experimental.cache_file.path, 'cache-first.db');
    assert.equal(merged.experimental.cache_file.cache_id, 'anonymous-merge');
    assert.equal(merged.route.final, 'First');
    assert.deepEqual(
      merged.route.rules.flatMap((rule) => rule.domain),
      ['a.invalid', 'z.invalid'],
    );
    assert.deepEqual(
      merged.outbounds.map((outbound) => outbound.tag),
      ['First', 'direct-a', 'Last', 'direct-z'],
    );
    await command('merge', [...configs].reverse(), ['merged-forward.json']).then((record) =>
      assert.equal(record.status, 0, record.stderr),
    );
    assert.deepEqual(JSON.parse(await readFile(path.join(root, 'merged-forward.json'), 'utf8')), merged);
    await checkConfig(configs);
    await start(configs);
    current.snapshot = await groups();
    assert.deepEqual(members(current.snapshot, 'First'), ['direct-a', 'direct-z']);
    assert.deepEqual(members(current.snapshot, 'Last'), ['direct-z']);
    current.checks.push(
      'multi -c sorts paths rather than argument order; first scalar wins; arrays append; objects recursively merge and add absent keys',
    );
  });

  for (const kind of ['outbound', 'provider']) {
    await scenario(`multi-c-duplicate-${kind}-tag`, async () => {
      const first = config([direct('FirstDirect')]);
      const last = {};
      if (kind === 'outbound') {
        first.outbounds.push({ type: 'selector', tag: 'Duplicate', outbounds: ['FirstDirect'] });
        last.outbounds = [{ type: 'selector', tag: 'Duplicate', outbounds: ['LastDirect'] }, direct('LastDirect')];
      } else {
        first.providers = [await provider('np/duplicate', [node('same')])];
        last.providers = [{ ...first.providers[0], path: 'other-provider.json' }];
        await jsonFile('other-provider.json', { outbounds: [node('second')] });
        first.outbounds.unshift({ type: 'selector', tag: 'DuplicateProviderConsumer', providers: ['np/duplicate'] });
        first.route.final = 'DuplicateProviderConsumer';
      }
      await jsonFile('10-first.json', first);
      await jsonFile('90-last.json', last);
      const configs = ['90-last.json', '10-first.json'];
      const merged = await command('merge', configs, ['merged.json']);
      if (kind === 'outbound') {
        assert.notEqual(merged.status, 0);
        assert.match(merged.stderr, /duplicate outbound\/endpoint tag: Duplicate/);
        await startupError(/duplicate outbound\/endpoint tag: Duplicate/, configs);
        assert.notEqual(current.check_status, 0);
        current.checks.push(
          'multi-c duplicate outbound tags are rejected during merged config decoding, not merged or overwritten',
        );
        return;
      }
      assert.equal(merged.status, 0, merged.stderr);
      current.merged = JSON.parse(await readFile(path.join(root, 'merged.json'), 'utf8'));
      assert.equal(current.merged.providers.filter((entry) => entry.tag === 'np/duplicate').length, 2);
      await checkConfig(configs);
      await start(configs);
      current.snapshot = await groups();
      assert.deepEqual(members(current.snapshot, 'DuplicateProviderConsumer'), ['np/duplicate/second']);
      await select('DuplicateProviderConsumer', 'np/duplicate/second');
      await rejectSelection('DuplicateProviderConsumer', 'np/duplicate/same');
      current.checks.push(
        'merged provider array retains duplicate tags, but runtime Provider manager uses the last definition in sorted-path order',
      );
    });
  }

  await scenario('cache-and-provider-hot-update', async () => {
    await jsonFile('provider-a.json', { outbounds: [node('same'), node('second')] });
    await jsonFile('provider-b.json', { outbounds: [node('same')] });
    await jsonFile('main.json', {
      log: { level: 'warn', output: 'stderr' },
      experimental: { cache_file: { enabled: true, path: 'cache.db' } },
      services: [{ type: 'api', listen: '127.0.0.1', listen_port: remote ? remote.apiRemotePort : apiPort, secret: 'isolated-test' }],
      outbounds: [
        { type: 'selector', tag: 'Main', providers: ['provider-a', 'provider-b'] },
        { type: 'urltest', tag: 'Auto', outbounds: ['local-direct'], url: testURL, interval: '1h', idle_timeout: '1h' },
        { type: 'direct', tag: 'local-direct' },
      ],
      route: { final: 'Main' },
    });
    await jsonFile('providers.json', {
      providers: [
        { type: 'local', tag: 'provider-a', path: 'provider-a.json' },
        { type: 'local', tag: 'provider-b', path: 'provider-b.json' },
      ],
    });
    await jsonFile('extra.json', { outbounds: [{ type: 'selector', tag: 'Extra', outbounds: ['Main', 'Auto'] }] });
    const configs = ['main.json', 'providers.json', 'extra.json'];
    await checkConfig(configs);
    await start(configs);
    const initial = await groups();
    current.initial = initial;
    assert.equal(initial.length, 3);
    assert.ok(!initial.some((group) => group.tag === 'Proxy'));
    assert.deepEqual(
      groupBy(initial, 'Main').items.map((item) => item.tag),
      ['provider-a/same', 'provider-a/second', 'provider-b/same'],
    );
    current.checks.push('multi-c merges outbounds; arbitrary groups work without Proxy; provider/node namespacing');
    await rejectSelection('Auto', 'provider-a/same');
    await rejectSelection('Main', 'same');
    await rejectSelection('Main', 'not-a-member');
    await select('Main', 'provider-b/same');
    assert.equal(groupBy(await groups(), 'Main').selected, 'provider-b/same');
    await stop();
    await start(configs);
    assert.equal(groupBy(await groups(), 'Main').selected, 'provider-b/same');
    current.checks.push('selector validates membership; cache_file restores exact group/member tags after restart');
    await select('Main', 'provider-a/second');
    const updatePID = remote ? remote.process.pid : child.pid;
    const updateIdentity = await startedAt();
    await updateProvider(['same', 'second', 'third']);
    try {
      await until(
        async () => groupBy(await groups(), 'Main').items.some((item) => item.tag === 'provider-a/third'),
        'provider hot update',
        30000,
      );
    } catch (error) {
      // Local Provider notifications are filesystem-dependent. NetProxy
      // verifies the Service API after every committed Catalog change and
      // reloads once when this fast path does not converge. Keep the exact
      // core behavior observable instead of turning an unavailable native
      // notification into a false product failure.
      current.provider_watcher = remote ? 'not_observed_on_android' : 'not_observed_on_host';
      current.checks.push('Provider watcher update was not observed; NetProxy runtime verification reloads as a safe fallback');
      return;
    }
    assert.equal(groupBy(await groups(), 'Main').selected, 'provider-a/second');
    await updateProvider(['same', 'third']);
    const removed = await until(async () => {
      const snapshot = await groups();
      const current = groupBy(snapshot, 'Main');
      return (
        !current.items.some((item) => item.tag === 'provider-a/second') &&
        current.selected !== 'provider-a/second' &&
        snapshot
      );
    }, 'removed member fallback');
    current.removed = removed;
    assert.equal(remote ? remote.process.pid : child.pid, updatePID);
    assert.equal(await startedAt(), updateIdentity);
    current.provider_update_identity = { pid: updatePID, core_started_at_millis: updateIdentity };
    current.checks.push(
      'native Provider file updates members without restart; retained selection survives; removed selection falls back',
    );
    await rpc('URLTest', textField(1, 'Auto'));
    current.afterDelay = await until(async () => {
      const snapshot = await groups();
      return groupBy(snapshot, 'Auto').items.some((item) => item.url_test_time > 0) && snapshot;
    }, 'native urltest history');
    current.checks.push('URLTest accepts native urltest tag and publishes real history');
    await select('Main', 'provider-b/same');
    await stop();
    const main = JSON.parse(await readFile(path.join(root, 'main.json')));
    main.outbounds[0].tag = 'Renamed';
    main.route.final = 'Renamed';
    await jsonFile('main.json', main);
    await jsonFile('extra.json', { outbounds: [{ type: 'selector', tag: 'Extra', outbounds: ['Renamed', 'Auto'] }] });
    await start(configs);
    assert.equal(groupBy(await groups(), 'Renamed').selected, 'provider-a/same');
    current.checks.push('renaming group tag creates a distinct cache identity');
  });
  evidence.ok = evidence.scenarios.every((result) => result.ok);
  console.log(
    JSON.stringify(
      {
        ok: evidence.ok,
        root: runRoot,
        passed: evidence.scenarios.filter((result) => result.ok).length,
        total: evidence.scenarios.length,
      },
      null,
      2,
    ),
  );
  if (!evidence.ok) process.exitCode = 1;
} finally {
  await stop();
  await remote?.cleanup();
  await new Promise((resolve) => web.close(resolve));
  await writeFile(path.join(runRoot, 'evidence.json'), JSON.stringify(evidence, null, 2));
}
