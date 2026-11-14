// The scenario check of spec 12: a second client of the helper's pipe (spec 5.1). It pins renamed
// copies of curl.exe and node.exe to the two lanes, enables routing, checks both lanes over TCP and
// UDP, moves a copy between the lanes, changes the main lane and "all direct", checks nested rules,
// watches the VPN copy of curl for leaks the whole time and disables routing again (steps 1-8).
// It is run by hand next to a running helper (plan 1B, task 16); the tests run only its refusal
// and its child. Addresses are never printed: the lane addresses of state.addresses become <vpn>
// and <direct>, any other address <other>.
//
// usage: node scripts/scenario.mjs [--pipe \\.\pipe\SplitLane.Helper] [--heartbeat FILE] [--keep]
//   --heartbeat FILE  every 30 s write the deadline of the plan 1A guard to FILE as Set-P1Heartbeat
//                     does (spike\plan1\p1lib.ps1); the guard reads spike\plan1\guard\deadline.txt
//   --keep            leave the layout of the scenario in place instead of the previous one
// Output: SCENARIO <n> PASS|FAIL <text> for each step (n 0: the preparation failed and nothing was
// changed), SCENARIO-RESTORE, then SCENARIO-DONE pass=<k> fail=<m>. The exit code is 0 only when
// every step passed and the previous layout came back.
//
// The same file is the child that the node copies run, so that the core sees the copy:
//   --child stun HOST PORT      one STUN Binding request (three tries), the mapped IPv4 address
//   --child keep URL MS [PIPE]  URL over one keep-alive connection every 250 ms for MS milliseconds;
//                               with PIPE the requests go to that named pipe (the tests)
// Each line of a child is "<unix ms> <ip>", or "<unix ms> -" when there was no reply.
import { execFile, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import dgram from 'node:dgram';
import { once } from 'node:events';
import { copyFileSync, existsSync, mkdirSync, realpathSync, renameSync, statSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

import {
  ENABLE_WAIT_MS, addressBook, buildLayout, createAllDirectTracker, createLineReader, createRpc, heartbeatText, isIPv4,
  laneOf, lanesKnown, learn, maskText, parseSample, parseStunResponse, parseTasklist, sameSystem,
  scenarioPaths, stunRequest, systemExitNode, verdictLane, verdictLeak, verdictSwitch, withDefault, withLane,
} from './scenario-lib.mjs';

const USAGE = 'usage: node scripts/scenario.mjs [--pipe \\\\.\\pipe\\SplitLane.Helper] [--heartbeat FILE] [--keep]';
const PIPE = '\\\\.\\pipe\\SplitLane.Helper';
const ADDRESS_URL = 'https://api.ipify.org'; // the address service of the helper (store.DefaultProbeURL)
const STUN_HOST = 'stun.cloudflare.com'; // answers on the VPN lane up to 1424 bytes (spec, appendix A)
const STUN_PORT = 3478;
const SYSTEM32 = path.join(process.env.SystemRoot || 'C:\\Windows', 'System32');
const CURL = path.join(SYSTEM32, 'curl.exe');
const TASKLIST = path.join(SYSTEM32, 'tasklist.exe');
const POWERSHELL = path.join(SYSTEM32, 'WindowsPowerShell', 'v1.0', 'powershell.exe');
const TAILSCALE = path.join(process.env.ProgramFiles || 'C:\\Program Files', 'Tailscale', 'tailscale.exe');
const SELF = fileURLToPath(import.meta.url);

const ENABLE_WAIT = ENABLE_WAIT_MS; // step 1
const DISABLE_WAIT = 30000; // step 8
const SWITCH_LIMIT = 3000; // step 4
const RETURN_LIMIT = 5000; // step 5: the 3 s check of spec 8.3 and one more request
const ALL_DIRECT_WAIT = 10000;
const LEAK_EVERY = 500; // step 7
const CORE_EVERY = 500;
const HEARTBEAT_EVERY = 30000;
const KEEP_MS = '60000'; // the longest life of a keep child; the scenario stops it earlier

const book = addressBook();

// say prints one line; every address in it becomes its lane.
function say(line) {
  process.stdout.write(maskText(line, book) + '\n');
}

const sec = (ms) => (ms / 1000).toFixed(1);

async function sleepUntil(t) {
  const ms = t - Date.now();
  if (ms > 0) await sleep(ms);
}

async function waitUntil(pred, timeoutMs) {
  const end = Date.now() + timeoutMs;
  while (!pred()) {
    if (Date.now() >= end) return false;
    await sleep(50);
  }
  return true;
}

// run starts a program without a window. It gives the exit code (-1 when the program could not
// start or was stopped by the time limit) and the output, and never throws.
function run(file, args, timeoutMs = 15000) {
  return new Promise((resolve) => {
    execFile(file, args, { timeout: timeoutMs, windowsHide: true, maxBuffer: 4 << 20 }, (err, stdout) => {
      resolve({ code: err ? (Number.isInteger(err.code) ? err.code : -1) : 0, stdout: String(stdout ?? '') });
    });
  });
}

// egress asks the address service with a copy of curl.exe, past any proxy (as slctl expect-lanes).
async function egress(exe) {
  const r = await run(exe, ['-4', '-s', '--noproxy', '*', '--max-time', '8', ADDRESS_URL], 12000);
  const ip = r.stdout.trim();
  return r.code === 0 && isIPv4(ip) ? ip : null;
}

// The children get no proxy and no NODE_OPTIONS: they talk to the services themselves.
function childEnv() {
  const env = { ...process.env };
  for (const k of Object.keys(env)) {
    if (/^(node_options|node_use_env_proxy|https?_proxy|all_proxy)$/i.test(k)) delete env[k];
  }
  return env;
}

// startChild runs this file as a child of a node copy; onSample gets each line. ended resolves when
// the child is gone; stop ends it.
function startChild(exe, args, onSample) {
  const child = spawn(exe, [SELF, '--child', ...args], { windowsHide: true, env: childEnv(), stdio: ['ignore', 'pipe', 'ignore'] });
  const reader = createLineReader((line) => onSample(parseSample(line)));
  child.stdout?.on('data', (chunk) => reader.push(chunk));
  const ended = new Promise((resolve) => {
    child.once('close', resolve);
    child.once('error', resolve);
  });
  return {
    ended,
    stop: async () => {
      child.kill();
      await ended;
    },
  };
}

// stunAddress asks the STUN server with a node copy (steps 3 and 5).
async function stunAddress(exe) {
  let ip = null;
  const c = startChild(exe, ['stun', STUN_HOST, String(STUN_PORT)], (s) => {
    ip = s.ip;
  });
  const timer = setTimeout(() => c.stop(), 20000);
  await c.ended;
  clearTimeout(timer);
  return ip;
}

// checkLane asks with one copy until verdictLane decides; the text names the copy and the lane of
// every reply.
async function checkLane(exe, want, ask = egress) {
  const seen = [];
  for (;;) {
    seen.push(laneOf(await ask(exe), book));
    const v = verdictLane(seen, want);
    if (v.done) return { ok: v.ok, text: `${path.basename(exe, '.exe')}=${seen.map((lane) => `<${lane}>`).join(',')}` };
  }
}

// errText names an error reply by its code (and the reasons of rule_rejected) only: the message
// may carry paths of this computer.
function errText(reply) {
  const e = reply.error ?? {};
  const rules = Array.isArray(e.rules) ? ` (${e.rules.map((r) => r.reason).join(', ')})` : '';
  return `${e.code ?? 'no result'}${rules}`;
}

// Helper is the connection to the pipe: replies go to their requests; state events update state and
// go to onState and to the waiters.
class Helper {
  static async connect(name) {
    const socket = net.connect(name);
    await once(socket, 'connect');
    return new Helper(socket);
  }

  constructor(socket) {
    this.socket = socket;
    this.state = null;
    this.onState = () => {};
    this.waiters = new Set();
    this.rpc = createRpc((line) => socket.write(line));
    const reader = createLineReader((line) => this.message(line));
    socket.on('data', (chunk) => {
      try {
        reader.push(chunk);
      } catch (err) {
        socket.destroy(err);
      }
    });
    socket.on('error', () => {});
    socket.on('close', () => this.rpc.fail(new Error('the pipe closed')));
  }

  message(line) {
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      return;
    }
    if (this.rpc.handle(msg) || msg?.event !== 'state') return;
    this.update(msg.data);
  }

  update(state) {
    this.state = state;
    this.onState(state, Date.now());
    for (const w of [...this.waiters]) w(state);
  }

  call(method, params, timeoutMs = 15000) {
    if (this.socket.destroyed) return Promise.reject(new Error('the pipe closed'));
    return this.rpc.call(method, params, timeoutMs);
  }

  // waitState resolves with the first state, the current one included, for which pred is true, or
  // with null after timeoutMs.
  waitState(pred, timeoutMs) {
    if (this.state && pred(this.state)) return Promise.resolve(this.state);
    return new Promise((resolve) => {
      const finish = (s) => {
        clearTimeout(timer);
        this.waiters.delete(check);
        resolve(s);
      };
      const check = (s) => {
        if (pred(s)) finish(s);
      };
      const timer = setTimeout(() => finish(null), timeoutMs);
      this.waiters.add(check);
    });
  }

  close() {
    this.socket.end();
  }
}

// writeAtomic writes a temporary file in the same folder and renames it, again while a reader holds
// the target (as Write-P1Atomic in p1lib.ps1 does, with another temporary name).
async function writeAtomic(file, text) {
  const tmp = `${file}.mjstmp`;
  let last;
  for (let attempt = 1; attempt <= 20; attempt++) {
    try {
      writeFileSync(tmp, text);
      renameSync(tmp, file);
      return;
    } catch (err) {
      last = err;
      await sleep(100);
    }
  }
  throw last;
}

// startHeartbeat keeps the guard of plan 1A armed for the whole run (Set-P1Heartbeat).
function startHeartbeat(file) {
  mkdirSync(path.dirname(file), { recursive: true });
  let writing = Promise.resolve();
  const beat = () => {
    writing = writing
      .then(() => writeAtomic(file, heartbeatText(Date.now())))
      .catch((err) => say(`SCENARIO-HEARTBEAT FAIL ${err.code ?? err.message}`));
  };
  beat();
  const timer = setInterval(beat, HEARTBEAT_EVERY);
  return async () => {
    clearInterval(timer);
    await writing;
  };
}

// corePids lists the processes of sing-box.exe; null when tasklist failed.
async function corePids() {
  const r = await run(TASKLIST, ['/fi', 'imagename eq sing-box.exe', '/fo', 'csv', '/nh']);
  return r.code === 0 ? parseTasklist(r.stdout) : null;
}

// watchCore looks at the core process every 500 ms for step 7; stop takes one more look.
function watchCore(phaseOf) {
  const looks = [];
  let stopped = false;
  const look = async () => {
    const pids = await corePids();
    const phase = phaseOf();
    looks.push({ t: Date.now(), pids, live: phase === 'on' || phase === 'degraded' || phase === 'blocked' });
  };
  const loop = (async () => {
    while (!stopped) {
      await look();
      await sleep(CORE_EVERY);
    }
  })();
  let stopping = null;
  return {
    looks,
    stop: () => (stopping ??= (async () => {
      stopped = true;
      await loop;
      await look();
    })()),
  };
}

// watchLeaks asks with the VPN copy of curl every 500 ms for the whole scenario (step 7).
function watchLeaks(exe) {
  const samples = [];
  let stopped = false;
  const loop = (async () => {
    while (!stopped) {
      const sent = Date.now();
      const ip = await egress(exe);
      samples.push({ sent, done: Date.now(), ip });
      await sleepUntil(sent + LEAK_EVERY);
    }
  })();
  let stopping = null;
  return {
    samples,
    stop: () => (stopping ??= (async () => {
      stopped = true;
      await loop;
    })()),
  };
}

// systemTailscale reads the exit node and the LAN flag of the system client (step 8); the values
// are only compared, never printed. null: the output could not be read.
async function systemTailscale() {
  if (!existsSync(TAILSCALE)) return { installed: false };
  const status = await run(TAILSCALE, ['status', '--json']);
  const prefs = await run(TAILSCALE, ['debug', 'prefs']);
  return systemExitNode(status.stdout, prefs.stdout);
}

// adapterGone: neither the adapter SplitLane nor a route through it (step 8).
async function adapterGone() {
  const r = await run(POWERSHELL, ['-NoProfile', '-NonInteractive', '-Command',
    '$n = @(Get-NetAdapter -Name SplitLane -ErrorAction SilentlyContinue).Count + '
    + '@(Get-NetRoute -InterfaceAlias SplitLane -ErrorAction SilentlyContinue).Count; \'SPLITLANE-OBJECTS \' + $n'], 30000);
  return r.stdout.trim() === 'SPLITLANE-OBJECTS 0';
}

// prepareCopies puts the copies of spec 12 under %TEMP%\splitlane-scenario, by its long path: the
// core matches the real path of a process. A copy of the same size stays from an earlier run.
function prepareCopies() {
  const p = scenarioPaths(path.join(realpathSync.native(os.tmpdir()), 'splitlane-scenario'));
  const copy = (from, to) => {
    mkdirSync(path.dirname(to), { recursive: true });
    if (!existsSync(to) || statSync(to).size !== statSync(from).size) copyFileSync(from, to);
  };
  for (const k of ['curlVpn', 'curlDirect', 'curlFree', 'curlOuter', 'curlNest']) copy(CURL, p[k]);
  for (const k of ['nodeVpn', 'nodeDirect', 'nodeFree']) copy(process.execPath, p[k]);
  return p;
}

function switchText(v) {
  return v.ok ? `first reply from the new lane after ${v.firstMs} ms, none from the old lane after it` : v.why;
}

// Step 1: routing.enable, phase on within 180 s, both lane addresses known and different.
async function stepEnable(helper) {
  const t = Date.now();
  const r = await helper.call('routing.enable');
  if (r.error) return { ok: false, text: `routing.enable: ${errText(r)}` };
  const s = await helper.waitState((st) => st.phase === 'on' || st.phase === 'error', ENABLE_WAIT);
  if (!s) return { ok: false, text: `no phase on within ${ENABLE_WAIT / 1000} s (phase ${helper.state?.phase})` };
  if (s.phase !== 'on') return { ok: false, text: `phase ${s.phase}, reason ${s.reason?.code ?? 'none'}` };
  const ms = Date.now() - t;
  if (!lanesKnown(s.addresses)) return { ok: false, text: `phase on after ${sec(ms)} s, but the lane addresses are missing or equal` };
  return { ok: true, text: `phase on after ${sec(ms)} s; the lane addresses are known and different` };
}

// Step 2: TCP, each pinned copy of curl gets the address of its lane.
async function stepTcp({ p }) {
  const vpn = await checkLane(p.curlVpn, 'vpn');
  const direct = await checkLane(p.curlDirect, 'direct');
  return { ok: vpn.ok && direct.ok, text: `${vpn.text} ${direct.text}` };
}

// Step 3: UDP, each pinned copy of node gets the address of its lane in the STUN reply.
async function stepUdp({ p }) {
  const vpn = await checkLane(p.nodeVpn, 'vpn', stunAddress);
  const direct = await checkLane(p.nodeDirect, 'direct', stunAddress);
  return { ok: vpn.ok && direct.ok, text: `${vpn.text} ${direct.text}` };
}

// Step 4: the VPN copy of node keeps one connection and asks every 250 ms; lanes.set moves it to
// direct and back. Each time the first reply from the new lane comes within 3 s after lanes.set
// and no reply from the old lane follows it; the core process stays the same.
async function stepMove({ helper, p, layout }) {
  const { vpn, direct } = helper.state.addresses;
  const pidsBefore = await corePids();
  const samples = [];
  const keep = startChild(p.nodeVpn, ['keep', ADDRESS_URL, KEEP_MS], (s) => samples.push(s));
  let there;
  let back;
  let tThere;
  let tBack;
  try {
    await waitUntil(() => samples.filter((s) => s.ip === vpn).length >= 2, 15000);
    tThere = Date.now();
    there = await helper.call('lanes.set', withLane(layout, p.nodeVpn, 'direct'));
    await sleepUntil(tThere + SWITCH_LIMIT + 1000);
    tBack = Date.now();
    back = await helper.call('lanes.set', layout);
    await sleepUntil(tBack + SWITCH_LIMIT + 1000);
  } finally {
    await keep.stop();
  }
  const pidsAfter = await corePids();
  const toDirect = verdictSwitch(samples.filter((s) => s.t <= tBack), tThere, vpn, direct, SWITCH_LIMIT);
  const toVpn = verdictSwitch(samples, tBack, direct, vpn, SWITCH_LIMIT);
  const samePid = pidsBefore?.length === 1 && pidsAfter?.length === 1 && pidsBefore[0] === pidsAfter[0];
  return {
    ok: !there.error && !back.error && toDirect.ok && toVpn.ok && samePid,
    text: [
      `node-vpn to direct: ${there.error ? `lanes.set ${errText(there)}` : switchText(toDirect)}`,
      `back to vpn: ${back.error ? `lanes.set ${errText(back)}` : switchText(toVpn)}`,
      `core process ${samePid ? 'the same' : 'changed or not found'}`,
    ].join('; '),
  };
}

// Step 5: the main lane and "all direct". The free copies follow the main lane while the VPN copy
// of curl stays on VPN; in "all direct" the free copy goes direct and the VPN copy of node opens a
// connection; after the return that connection is closed (its replies come from VPN within 5 s and
// never from direct again) and the VPN copy of curl is on VPN.
async function stepMain({ helper, p, layout, tracker }) {
  const { vpn, direct } = helper.state.addresses;
  const parts = [];
  let ok = true;
  const fail = (text) => {
    ok = false;
    parts.push(text);
  };
  const check = async (label, checks) => {
    const texts = [];
    for (const [exe, want, ask] of checks) {
      const r = await checkLane(exe, want, ask);
      ok &&= r.ok;
      texts.push(r.text);
    }
    parts.push(`${label}: ${texts.join(' ')}`);
  };
  const lanes = async (label, params) => {
    const r = await helper.call('lanes.set', params);
    if (r.error) fail(`${label}: lanes.set ${errText(r)}`);
    return !r.error;
  };

  await check('main vpn', [[p.curlFree, 'vpn']]);
  if (await lanes('main direct', withDefault(layout, 'direct'))) {
    await check('main direct', [[p.curlFree, 'direct'], [p.nodeFree, 'direct', stunAddress], [p.curlVpn, 'vpn']]);
  }
  if (await lanes('main vpn again', layout)) {
    await check('main vpn again', [[p.curlFree, 'vpn'], [p.nodeFree, 'vpn', stunAddress]]);
  }

  tracker.requested(true, Date.now());
  const on = await helper.call('routing.allDirect', { on: true });
  if (on.error) {
    tracker.requested(false, Date.now());
    fail(`all direct: routing.allDirect ${errText(on)}`);
    return { ok, text: parts.join('; ') };
  }
  const isOn = await helper.waitState((s) => s.allDirect === true, ALL_DIRECT_WAIT);
  if (isOn) await check('all direct', [[p.curlFree, 'direct']]);
  else fail(`all direct: not on within ${ALL_DIRECT_WAIT / 1000} s`);
  const samples = [];
  const keep = isOn ? startChild(p.nodeVpn, ['keep', ADDRESS_URL, KEEP_MS], (s) => samples.push(s)) : null;
  let tOff;
  let off;
  let returned = null;
  try {
    if (keep) await waitUntil(() => samples.filter((s) => s.ip === direct).length >= 2, 15000);
    tOff = Date.now();
    tracker.requested(false, tOff);
    off = await helper.call('routing.allDirect', { on: false });
    if (!off.error) returned = await helper.waitState((s) => s.allDirect === false, ALL_DIRECT_WAIT);
    if (keep) await sleepUntil(tOff + RETURN_LIMIT + 1000);
  } finally {
    await keep?.stop();
  }
  if (off.error) fail(`back: routing.allDirect ${errText(off)}`);
  else if (!returned) fail(`back: all direct still on after ${ALL_DIRECT_WAIT / 1000} s`);
  if (keep) {
    const v = verdictSwitch(samples, tOff, direct, vpn, RETURN_LIMIT);
    ok &&= v.ok;
    parts.push(`after all direct: node-vpn ${switchText(v)}`);
  }
  await check('after all direct', [[p.curlVpn, 'vpn']]);
  return { ok, text: parts.join('; ') };
}

// Step 6: nested rules. The folder nest is pinned direct and nest\inner\curl-nest.exe vpn: the inner
// copy goes by VPN, the copy elsewhere in the folder directly.
async function stepNested({ p }) {
  const inner = await checkLane(p.curlNest, 'vpn');
  const outer = await checkLane(p.curlOuter, 'direct');
  return { ok: inner.ok && outer.ok, text: `${inner.text} ${outer.text}` };
}

// Step 8: routing.disable, phase off within 30 s; the system exit node and its LAN flag as before
// step 1, no core process, neither the adapter SplitLane nor a route through it.
async function stepDisable(helper, before) {
  const t = Date.now();
  const r = await helper.call('routing.disable', {});
  if (r.error) return { ok: false, text: `routing.disable: ${errText(r)}` };
  const off = await helper.waitState((s) => s.phase === 'off', DISABLE_WAIT);
  const ms = Date.now() - t;
  const system = sameSystem(before, await systemTailscale());
  const pids = await corePids();
  const gone = await adapterGone();
  let core = 'no sing-box.exe';
  if (pids === null) core = 'tasklist failed';
  else if (pids.length > 0) core = 'sing-box.exe still runs';
  return {
    ok: Boolean(off) && system && pids?.length === 0 && gone,
    text: [
      off ? `phase off after ${sec(ms)} s`
        : `no phase off within ${DISABLE_WAIT / 1000} s (phase ${helper.state?.phase}, reason ${helper.state?.reason?.code ?? 'none'})`,
      `system exit node and LAN flag ${system ? 'equal' : 'different or unreadable'}`,
      core,
      gone ? 'no SplitLane adapter or route' : 'SplitLane adapter or route still there',
    ].join('; '),
  };
}

async function disableQuietly(helper) {
  try {
    await helper.call('routing.disable', {});
    await helper.waitState((s) => s.phase === 'off', DISABLE_WAIT);
  } catch {
    // The pipe is gone: the guard of plan 1A and slctl are the way back.
  }
}

// restoreLayout gives the helper the layout it had before the scenario, unless --keep.
async function restoreLayout(helper, previous, keep) {
  if (keep) {
    say('SCENARIO-RESTORE PASS the layout of the scenario stays (--keep)');
    return true;
  }
  try {
    const r = await helper.call('lanes.set', previous);
    say(`SCENARIO-RESTORE ${r.error ? `FAIL ${errText(r)}` : 'PASS'} previous layout of ${previous.apps.length} rules`);
    return !r.error;
  } catch (err) {
    say(`SCENARIO-RESTORE FAIL ${err.message}`);
    return false;
  }
}

// withHelper is the scenario on a connected pipe; it gives whether the previous layout came back.
async function withHelper(helper, opts, report) {
  const hello = await helper.call('hello', { protocol: 1 });
  if (hello.result?.protocol !== 1) {
    report(0, false, `hello: ${errText(hello)}`);
    return true;
  }
  const first = await helper.call('state.get');
  if (!first.result) {
    report(0, false, `state.get: ${errText(first)}`);
    return true;
  }
  helper.update(first.result);
  if (first.result.phase !== 'off') {
    report(0, false, `the helper is in phase ${first.result.phase}; the scenario starts from phase off`);
    return true;
  }
  const previous = { apps: first.result.apps ?? [], defaultLane: first.result.defaultLane === 'direct' ? 'direct' : 'vpn' };
  const before = await systemTailscale();
  const p = prepareCopies();
  const layout = buildLayout(p);
  const set = await helper.call('lanes.set', layout);
  if (set.error) {
    report(0, false, `lanes.set of the scenario layout: ${errText(set)}`);
    return true;
  }
  say(`SCENARIO-PREP ${layout.apps.length} rules for the copies in %TEMP%\\splitlane-scenario; `
    + `the previous layout has ${previous.apps.length} rules`);

  const tracker = createAllDirectTracker();
  helper.onState = (s, t) => {
    learn(book, s.addresses);
    tracker.observed(s.allDirect === true, t);
  };
  const ctx = { helper, p, layout, tracker };
  const core = watchCore(() => helper.state?.phase);
  const leaks = watchLeaks(p.curlVpn);
  let routing = true; // from routing.enable until the disable of step 8
  let restored = false;
  try {
    const one = await stepEnable(helper);
    report(1, one.ok, one.text);
    for (const [n, step] of [[2, stepTcp], [3, stepUdp], [4, stepMove], [5, stepMain], [6, stepNested]]) {
      if (!one.ok) {
        report(n, false, 'not run: routing is not on');
        continue;
      }
      const r = await step(ctx);
      report(n, r.ok, r.text);
    }
    const eight = await stepDisable(helper, before);
    routing = false;
    await leaks.stop();
    await core.stop();
    const v = verdictLeak(leaks.samples, core.looks, tracker.windows(), book);
    report(7, v.ok, `${v.counted} replies of curl-vpn counted, ${v.ignored} not counted (no core or all direct), `
      + `${v.leaks} of them from <direct>`);
    report(8, eight.ok, eight.text);
  } finally {
    if (routing) await disableQuietly(helper);
    await leaks.stop();
    await core.stop();
    restored = await restoreLayout(helper, previous, opts.keep);
  }
  return restored;
}

async function scenario(opts) {
  const stopHeartbeat = opts.heartbeat ? startHeartbeat(opts.heartbeat) : async () => {};
  const passed = [];
  const failed = [];
  const report = (n, ok, text) => {
    (ok ? passed : failed).push(n);
    say(`SCENARIO ${n} ${ok ? 'PASS' : 'FAIL'} ${text}`);
  };
  let restored = true;
  let helper = null;
  try {
    try {
      helper = await Helper.connect(opts.pipe);
    } catch (err) {
      report(0, false, `no pipe ${opts.pipe}: ${err.code ?? err.message}`);
    }
    if (helper) restored = await withHelper(helper, opts, report);
  } catch (err) {
    failed.push('abort');
    say(`SCENARIO-ABORT ${err.message}`);
  } finally {
    helper?.close();
    await stopHeartbeat();
  }
  say(`SCENARIO-DONE pass=${passed.length} fail=${failed.length}`);
  return failed.length === 0 && restored ? 0 : 1;
}

// childStun sends one Binding request to host:port, tried three times, 2 s each (steps 3 and 5).
async function childStun(host, port) {
  const socket = dgram.createSocket('udp4');
  socket.on('error', () => {});
  try {
    for (let attempt = 0; attempt < 3; attempt++) {
      const tx = randomBytes(12);
      const got = await new Promise((resolve) => {
        const finish = (v) => {
          clearTimeout(timer);
          socket.off('message', onMessage);
          resolve(v);
        };
        const onMessage = (msg) => {
          const a = parseStunResponse(msg, tx);
          if (a) finish(a);
        };
        const timer = setTimeout(() => finish(null), 2000);
        socket.on('message', onMessage);
        socket.send(stunRequest(tx), port, host, (err) => {
          if (err) finish(null);
        });
      });
      if (got) {
        process.stdout.write(`${Date.now()} ${got.ip}\n`);
        return;
      }
    }
    process.stdout.write(`${Date.now()} -\n`);
  } finally {
    socket.close();
  }
}

// ask is one request of the keep child: {ip, conn}, the address in the body (or null) and the
// number that numberOf gave its connection (0: it got none).
function ask(mod, target, agent, pipe, numberOf) {
  return new Promise((resolve) => {
    let conn = 0;
    const done = (ip) => resolve({ ip, conn });
    const req = mod.get(target, pipe ? { agent, socketPath: pipe } : { agent }, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (d) => {
        body += d;
      });
      res.on('end', () => done(isIPv4(body.trim()) ? body.trim() : null));
      res.on('error', () => done(null));
      res.on('close', () => done(null)); // after end this changes nothing
    });
    req.on('socket', (socket) => {
      conn = numberOf(socket);
    });
    req.setTimeout(5000, () => req.destroy(new Error('timeout')));
    req.on('error', () => done(null));
  });
}

// childKeep asks url over one keep-alive connection every 250 ms for ms milliseconds (steps 4, 5).
// A request on a connection that the helper closes fails ("-"); the next one opens a new connection.
// Each line carries the number of the connection, so that the parent sees the connection kept.
async function childKeep(url, ms, pipe) {
  const target = new URL(url);
  const mod = target.protocol === 'http:' ? http : https;
  const agent = new mod.Agent({ keepAlive: true, maxSockets: 1 });
  const numbers = new WeakMap();
  let opened = 0;
  const numberOf = (socket) => {
    if (!numbers.has(socket)) numbers.set(socket, ++opened);
    return numbers.get(socket);
  };
  const end = Date.now() + ms;
  while (Date.now() < end) {
    const started = Date.now();
    const { ip, conn } = await ask(mod, target, agent, pipe, numberOf);
    process.stdout.write(`${Date.now()} ${ip ?? '-'} ${conn}\n`);
    await sleepUntil(started + 250);
  }
  agent.destroy();
}

let args;
try {
  args = parseArgs({
    options: {
      pipe: { type: 'string', default: PIPE },
      heartbeat: { type: 'string' },
      keep: { type: 'boolean', default: false },
      child: { type: 'string' },
    },
    allowPositionals: true,
  });
} catch (err) {
  process.stderr.write(`${err.message}\n${USAGE}\n`);
  process.exit(2);
}
const { values, positionals } = args;
let code = 0;
if (values.child === 'stun') await childStun(positionals[0], Number(positionals[1]));
else if (values.child === 'keep') await childKeep(positionals[0], Number(positionals[1]), positionals[2]);
else if (values.child || positionals.length > 0) {
  process.stderr.write(`${USAGE}\n`);
  code = 2;
} else code = await scenario(values);
process.stdout.write('', () => process.exit(code));
