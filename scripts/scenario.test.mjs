// Tests of the scenario check of spec 12: node --test scripts/scenario.test.mjs
// The pure parts of scenario-lib.mjs, and scenario.mjs against a fake helper and a fake address
// service on named pipes SplitLane.Test.<number>. Test data uses documentation addresses only.
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import http from 'node:http';
import net from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  addressBook, buildLayout, coreAliveThroughout, createAllDirectTracker, createLineReader, createRpc,
  laneOf, lanesKnown, learn, mask, maskText, parseSample, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault,
  withLane, ENABLE_WAIT_MS,
} from './scenario-lib.mjs';

const SCENARIO = fileURLToPath(new URL('./scenario.mjs', import.meta.url));

// runNode runs this node with args and gives the exit code and the output.
function runNode(args) {
  return new Promise((resolve) => {
    execFile(process.execPath, args, { timeout: 20000, windowsHide: true }, (err, stdout, stderr) => {
      resolve({ code: err ? err.code : 0, stdout, stderr });
    });
  });
}

function listen(server, pipe) {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(pipe, resolve);
  });
}

const VPN = '198.51.100.7';
const DIRECT = '203.0.113.9';

// stunSuccess is a Binding success response to tx: a SOFTWARE attribute of 5 bytes (padded to 8)
// before an XOR-MAPPED-ADDRESS, IPv4 for an ip, IPv6 when ip is null.
function stunSuccess(tx, ip, port) {
  const software = Buffer.alloc(12);
  software.writeUInt16BE(0x8022, 0);
  software.writeUInt16BE(5, 2);
  software.write('tests', 4, 'latin1');
  let mapped;
  if (ip) {
    mapped = Buffer.alloc(12);
    mapped.writeUInt16BE(0x0020, 0);
    mapped.writeUInt16BE(8, 2);
    mapped[5] = 0x01;
    mapped.writeUInt16BE(port ^ 0x2112, 6);
    const n = ip.split('.').reduce((a, octet) => a * 256 + Number(octet), 0);
    mapped.writeUInt32BE((n ^ 0x2112a442) >>> 0, 8);
  } else {
    mapped = Buffer.alloc(24);
    mapped.writeUInt16BE(0x0020, 0);
    mapped.writeUInt16BE(20, 2);
    mapped[5] = 0x02;
  }
  const head = Buffer.alloc(20);
  head.writeUInt16BE(0x0101, 0);
  head.writeUInt16BE(software.length + mapped.length, 2);
  head.writeUInt32BE(0x2112a442, 4);
  tx.copy(head, 8);
  return Buffer.concat([head, software, mapped]);
}

test('stun request and response', () => {
  const tx = Buffer.from('0123456789ab');
  const req = stunRequest(tx);
  assert.equal(req.length, 20);
  assert.equal(req.readUInt16BE(0), 0x0001, 'Binding request');
  assert.equal(req.readUInt16BE(2), 0, 'no attributes');
  assert.equal(req.readUInt32BE(4), 0x2112a442, 'magic cookie');
  assert.deepEqual(req.subarray(8), tx);
  assert.throws(() => stunRequest(Buffer.alloc(11)), /12 bytes/);

  const res = stunSuccess(tx, '203.0.113.7', 54321);
  assert.deepEqual(parseStunResponse(res, tx), { ip: '203.0.113.7', port: 54321 });
  assert.equal(parseStunResponse(res, Buffer.from('ba9876543210')), null, 'another transaction');
  assert.equal(parseStunResponse(res.subarray(0, 30), tx), null, 'cut short');
  const failure = Buffer.from(res);
  failure.writeUInt16BE(0x0111, 0);
  assert.equal(parseStunResponse(failure, tx), null, 'an error response');
  assert.equal(parseStunResponse(stunSuccess(tx, null, 0), tx), null, 'IPv6 only');
});

test('line reader splits and joins chunks', () => {
  const lines = [];
  const reader = createLineReader((line) => lines.push(line));
  reader.push(Buffer.from('{"a":1}\n{"b"'));
  reader.push(Buffer.from(':2}\r\n\n'));
  const zhe = Buffer.from('"\u0416"\n', 'utf8'); // a two-byte character split between two chunks
  reader.push(zhe.subarray(0, 2));
  reader.push(zhe.subarray(2));
  reader.push(Buffer.from('{"c":3}\n{"d":4}\n'));
  assert.deepEqual(lines, ['{"a":1}', '{"b":2}', '"\u0416"', '{"c":3}', '{"d":4}']);

  const small = createLineReader(() => {}, 8);
  small.push(Buffer.from('12345678\n'));
  assert.throws(() => small.push(Buffer.from('123456789')), /longer than 8/);
});

test('requests match replies by id', async () => {
  const sent = [];
  const rpc = createRpc((line) => sent.push(JSON.parse(line)));
  const hello = rpc.call('hello', { protocol: 1 });
  const state = rpc.call('state.get');
  assert.deepEqual(sent, [{ id: 1, method: 'hello', params: { protocol: 1 } }, { id: 2, method: 'state.get' }]);
  assert.equal(rpc.handle({ event: 'state', data: { phase: 'on' } }), false, 'an event');
  assert.equal(rpc.handle({ id: 9, result: {} }), false, 'nobody asked');
  assert.equal(rpc.handle({ id: 2, result: { phase: 'off' } }), true);
  assert.equal(rpc.handle({ id: 1, error: { code: 'protocol_mismatch', message: 'm', protocol: 2 } }), true);
  assert.equal(rpc.handle({ id: 1, result: {} }), false, 'answered once');
  assert.deepEqual(await state, { id: 2, result: { phase: 'off' } });
  assert.equal((await hello).error.code, 'protocol_mismatch');

  await assert.rejects(rpc.call('routing.enable', undefined, 20), /no reply to routing.enable within 20 ms/);
  const pending = rpc.call('lanes.set', {});
  rpc.fail(new Error('the pipe closed'));
  await assert.rejects(pending, /the pipe closed/);
  assert.equal(rpc.pending(), 0);
});

test('mask hides addresses', () => {
  const book = addressBook();
  learn(book, { vpn: VPN, direct: DIRECT });
  learn(book, { vpn: null, direct: null });
  learn(book, { vpn: '198.51.100.8', direct: DIRECT });
  assert.deepEqual(book, { vpn: [VPN, '198.51.100.8'], direct: [DIRECT] });
  assert.equal(laneOf('198.51.100.8', book), 'vpn');
  assert.equal(laneOf(DIRECT, book), 'direct');
  assert.equal(laneOf('192.0.2.1', book), 'other');
  assert.equal(laneOf(null, book), 'none');
  assert.equal(mask(DIRECT, book), '<direct>');
  assert.equal(
    maskText(`vpn ${VPN}, direct ${DIRECT}, other 192.0.2.1, core 1.14.4`, book),
    'vpn <vpn>, direct <direct>, other <other>, core 1.14.4');
  assert.equal(maskText(`before the state: ${DIRECT}`, addressBook()), 'before the state: <other>');
});

test('lane verdict wants the own address', () => {
  assert.deepEqual(verdictLane(['vpn'], 'vpn'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['direct'], 'vpn'), { ok: false, done: true });
  assert.deepEqual(verdictLane(['none', 'other'], 'vpn'), { ok: false, done: false });
  assert.deepEqual(verdictLane(['other', 'vpn', 'vpn'], 'vpn'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['other', 'vpn', 'direct'], 'vpn'), { ok: false, done: true });
  assert.deepEqual(verdictLane(['vpn', 'direct', 'direct'], 'direct'), { ok: true, done: true });
  assert.deepEqual(verdictLane(['vpn', 'other', 'direct'], 'direct'), { ok: false, done: true });
  assert.deepEqual(verdictLane(Array(6).fill('none'), 'direct'), { ok: false, done: true });
  assert.equal(lanesKnown({ vpn: VPN, direct: DIRECT }), true);
  assert.equal(lanesKnown({ vpn: VPN, direct: VPN }), false);
  assert.equal(lanesKnown({ vpn: null, direct: DIRECT }), false);
  assert.equal(lanesKnown(undefined), false);
});

test('switch verdict passes and fails', () => {
  const at = (t, ip, conn = 1) => ({ t, ip, conn });
  const good = [at(100, VPN), at(350, VPN), at(600, VPN), at(1100, VPN), at(1400, null), at(1950, DIRECT, 2), at(1700, DIRECT, 2)];
  assert.deepEqual(verdictSwitch(good, 1000, VPN, DIRECT), { ok: true, firstMs: 700, why: '' });

  const late = [at(350, VPN), at(600, VPN), at(4100, DIRECT, 2)];
  assert.deepEqual(verdictSwitch(late, 1000, VPN, DIRECT),
    { ok: false, firstMs: 3100, why: 'the first reply from the new lane came later than 3000 ms' });
  assert.equal(verdictSwitch(late, 1000, VPN, DIRECT, 5000).ok, true, 'a longer limit');

  assert.deepEqual(verdictSwitch([at(350, VPN), at(600, VPN), at(1500, DIRECT, 2), at(1750, VPN, 3)], 1000, VPN, DIRECT),
    { ok: false, firstMs: 500, why: 'a reply from the old lane after the first one from the new lane' });
  assert.equal(verdictSwitch([at(1500, DIRECT)], 1000, VPN, DIRECT).why, 'no reply from the old lane before the switch');
  assert.equal(verdictSwitch([at(350, VPN, 1), at(600, VPN, 2), at(1500, DIRECT, 3)], 1000, VPN, DIRECT).why,
    'the copy did not keep its connection before the switch');
  assert.deepEqual(verdictSwitch([at(350, VPN), at(1000, VPN), at(1000, DIRECT, 2), at(1300, DIRECT, 2)], 1000, VPN, DIRECT),
    { ok: true, firstMs: 300, why: '' }, 'a reply stamped with the time of the switch came before it');
  assert.equal(verdictSwitch([at(350, VPN), at(600, VPN), at(1500, VPN)], 1000, VPN, DIRECT).why, 'no reply from the new lane');
  assert.equal(verdictSwitch([at(350, VPN), at(600, VPN), at(800, DIRECT, 2), at(1500, VPN, 3), at(1600, DIRECT, 4)], 1000,
    VPN, DIRECT).firstMs, 600, 'a reply from before the switch is not the first one');
});

test('leak verdict ignores all-direct', () => {
  const book = learn(addressBook(), { vpn: VPN, direct: DIRECT });
  const tracker = createAllDirectTracker();
  tracker.requested(true, 1000);
  tracker.observed(false, 1200); // a state sent while the switch is applied still says false
  tracker.observed(true, 2500);
  tracker.requested(false, 5000);
  tracker.observed(true, 5100); // the switch back is applied
  tracker.observed(false, 7000);
  tracker.observed(false, 7500);
  tracker.observed(true, 12000); // switched on by another client
  tracker.observed(false, 12500);
  const windows = tracker.windows();
  assert.deepEqual(windows, [{ from: 1000, to: 7000 }, { from: 12000, to: 12500 }]);
  const open = createAllDirectTracker();
  open.requested(true, 100);
  open.observed(false, 200);
  assert.deepEqual(open.windows(), [{ from: 100, to: Infinity }], 'not closed before it was on');

  const core = [{ t: 0, alive: false }];
  for (let t = 400; t <= 8900; t += 500) core.push({ t, alive: true });
  core.push({ t: 9400, alive: false }, { t: 9900, alive: false });
  const s = (sent, done, ip) => ({ sent, done, ip });
  const samples = [
    s(100, 300, DIRECT), // no core yet
    s(500, 700, VPN),
    s(900, 1100, DIRECT), // all direct begins during the request
    s(3000, 3200, DIRECT), // all direct
    s(6900, 7100, DIRECT), // all direct ends during the request
    s(7600, 7800, VPN),
    s(8000, 8200, null), // no reply: counted, not a leak
    s(8800, 9200, DIRECT), // the core may have ended during the request
    s(9500, 9700, DIRECT), // no core any more
  ];
  assert.deepEqual(verdictLeak(samples, core, windows, book), { ok: true, counted: 3, ignored: 6, leaks: 0 });
  assert.deepEqual(verdictLeak([...samples, s(8100, 8300, DIRECT)], core, windows, book),
    { ok: false, counted: 4, ignored: 6, leaks: 1 });
  assert.equal(verdictLeak([s(100, 300, VPN)], core, windows, book).ok, false, 'nothing counted proves nothing');

  assert.equal(coreAliveThroughout(core, 500, 700), true);
  assert.equal(coreAliveThroughout(core, 300, 500), false);
  assert.equal(coreAliveThroughout([{ t: 0, alive: true }], 100, 200), false, 'no look after the reply');
  assert.equal(coreAliveThroughout([{ t: 0, alive: true }, { t: 300, alive: null }], 100, 200), false, 'tasklist failed');
});

test('layout has unique uuids and file rules', () => {
  const p = scenarioPaths('C:\\Temp\\splitlane-scenario');
  assert.equal(p.curlNest, 'C:\\Temp\\splitlane-scenario\\nest\\inner\\curl-nest.exe');
  assert.equal(p.curlOuter, 'C:\\Temp\\splitlane-scenario\\nest\\curl-outer.exe');
  const exes = Object.values(p).filter((x) => x.endsWith('.exe'));
  assert.deepEqual(exes.map((x) => x.split('\\').pop()).sort(), ['curl-direct.exe', 'curl-free.exe', 'curl-nest.exe',
    'curl-outer.exe', 'curl-vpn.exe', 'node-direct.exe', 'node-free.exe', 'node-vpn.exe']);
  const folders = new Set([p.curlVpn, p.curlDirect, p.curlFree].map((x) => x.slice(0, x.lastIndexOf('\\'))));
  assert.equal(folders.size, 3, 'one folder per lane');

  const layout = buildLayout(p);
  assert.equal(layout.defaultLane, 'vpn');
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
  assert.ok(layout.apps.every((a) => uuid.test(a.id)), 'UUIDs');
  assert.equal(new Set(layout.apps.map((a) => a.id)).size, layout.apps.length, 'unique');
  assert.notEqual(buildLayout(p).apps[0].id, layout.apps[0].id, 'new ids on every run');
  assert.deepEqual(layout.apps.map((a) => [a.exe, a.matcher.kind, a.matcher.value, a.lane]), [
    [p.curlVpn, 'file', '', 'vpn'],
    [p.nodeVpn, 'file', '', 'vpn'],
    [p.curlDirect, 'file', '', 'direct'],
    [p.nodeDirect, 'file', '', 'direct'],
    [p.curlOuter, 'folder', '', 'direct'],
    [p.curlNest, 'file', '', 'vpn'],
  ]);
  assert.deepEqual(layout.apps.map((a) => a.name), ['scenario curl-vpn', 'scenario node-vpn', 'scenario curl-direct',
    'scenario node-direct', 'scenario nest', 'scenario curl-nest']);

  const moved = withLane(layout, p.nodeVpn, 'direct');
  assert.equal(moved.apps.find((a) => a.exe === p.nodeVpn).lane, 'direct');
  assert.equal(layout.apps.find((a) => a.exe === p.nodeVpn).lane, 'vpn', 'the original stays');
  assert.deepEqual(moved.apps.map((a) => a.id), layout.apps.map((a) => a.id), 'the same ids');
  assert.equal(withDefault(layout, 'direct').defaultLane, 'direct');
  assert.equal(layout.defaultLane, 'vpn');
});

test('system output is parsed and compared', () => {
  const status = JSON.stringify({ BackendState: 'Running',
    ExitNodeStatus: { ID: 'n1', Online: true, TailscaleIPs: ['fd7a:115c:a1e0::5/128', '100.64.0.5/32'] } });
  const on = systemExitNode(status, '{"ExitNodeAllowLANAccess":true}');
  assert.deepEqual(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: true });
  assert.deepEqual(systemExitNode('{"BackendState":"Running","ExitNodeStatus":null}', '{}'),
    { installed: true, on: false, ip: null, allowLan: false });
  assert.deepEqual(systemExitNode(JSON.stringify({ BackendState: 'Stopped', ExitNodeStatus: { TailscaleIPs: ['100.64.0.5/32'] } }),
    '{"ExitNodeAllowLANAccess":true}'), { installed: true, on: false, ip: null, allowLan: true });
  assert.equal(systemExitNode('', '{}'), null);

  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: true }), true);
  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.5', allowLan: false }), false);
  assert.equal(sameSystem(on, { installed: true, on: true, ip: '100.64.0.6', allowLan: true }), false);
  assert.equal(sameSystem({ installed: false }, { installed: false }), true);
  assert.equal(sameSystem(null, null), false, 'unreadable is never equal');

  assert.deepEqual(parseTasklist('"sing-box.exe","1234","Services","0","25,000 K"\r\n"Sing-Box.exe","88","Console","1","1,024 K"\r\n'),
    [1234, 88]);
  assert.deepEqual(parseTasklist('INFO: No tasks are running which match the specified criteria.\r\n'), []);
});

test('scenario refuses a helper that is not off', async () => {
  const pipe = `\\\\.\\pipe\\SplitLane.Test.${process.pid}1`;
  const methods = [];
  const server = net.createServer((conn) => {
    conn.on('error', () => {});
    const reader = createLineReader((line) => {
      const req = JSON.parse(line);
      methods.push(req.method);
      const result = req.method === 'hello'
        ? { protocol: 1, helper: '0.0.0', core: '1.14.4' }
        : { phase: 'on', reason: null, allDirect: false, defaultLane: 'vpn', apps: [], addresses: { vpn: VPN, direct: DIRECT } };
      conn.write(JSON.stringify({ id: req.id, result }) + '\n');
    });
    conn.on('data', (chunk) => reader.push(chunk));
  });
  await listen(server, pipe);
  const dir = mkdtempSync(join(tmpdir(), 'scenario-test-'));
  const heartbeat = join(dir, 'guard', 'deadline.txt');
  try {
    const r = await runNode([SCENARIO, '--pipe', pipe, '--heartbeat', heartbeat]);
    assert.deepEqual(r.stdout.trim().split(/\r?\n/), [
      'SCENARIO 0 FAIL the helper is in phase on; the scenario starts from phase off',
      'SCENARIO-DONE pass=0 fail=1',
    ], r.stderr);
    assert.equal(r.code, 1);
    assert.deepEqual(methods, ['hello', 'state.get'], 'nothing is changed');
    const text = readFileSync(heartbeat, 'utf8');
    assert.match(text, /^\d+$/);
    const ahead = Number(text) - Math.floor(Date.now() / 1000);
    assert.ok(ahead > 380 && ahead <= 400, `the deadline is ${ahead} s ahead`);
  } finally {
    server.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test('keep child reuses one connection and reconnects', async () => {
  const pipe = `\\\\.\\pipe\\SplitLane.Test.${process.pid}2`;
  const conns = new Map();
  const server = http.createServer((req, res) => {
    const c = conns.get(req.socket);
    c.requests++;
    if (c.n === 1 && c.requests === 3) {
      req.socket.destroy(); // like the helper closing a connection on the wrong lane (spec 8.3)
      return;
    }
    res.end(c.n === 1 ? '192.0.2.1' : '192.0.2.2');
  });
  server.on('connection', (socket) => conns.set(socket, { n: conns.size + 1, requests: 0 }));
  await listen(server, pipe);
  try {
    const r = await runNode([SCENARIO, '--child', 'keep', 'http://scenario.test/', '1800', pipe]);
    assert.equal(r.code, 0, r.stderr);
    const samples = r.stdout.trim().split(/\r?\n/).map(parseSample);
    const ips = samples.map((s) => s.ip);
    assert.deepEqual(ips.slice(0, 4), ['192.0.2.1', '192.0.2.1', null, '192.0.2.2']);
    assert.ok(ips.slice(3).every((ip) => ip === '192.0.2.2'), ips.join(' '));
    const numbers = samples.map((s) => s.conn);
    assert.deepEqual(numbers.slice(0, 4), [1, 1, 1, 2], 'the number of the connection of each reply');
    assert.ok(numbers.slice(3).every((n) => n === 2), numbers.join(' '));
    assert.ok(samples.length >= 4 && samples.length <= 9, `${samples.length} replies in 1.8 s`);
    assert.ok(samples.every((s, i) => Number.isInteger(s.t) && (i === 0 || s.t >= samples[i - 1].t)));
    assert.equal(conns.size, 2, 'one connection until it was closed, then one more');
  } finally {
    server.close();
  }
});

test('the wait for step 1 covers the tunnel retry of spec 5.3 (review F7-1)', () => {
  // first start and proof 45 s + stop + second start and proof 45 s, and the early-exit retry
  assert.ok(ENABLE_WAIT_MS >= 180000, `ENABLE_WAIT_MS ${ENABLE_WAIT_MS} is below 180000`);
});
