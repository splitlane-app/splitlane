// Pure parts of the scenario check of spec 12 (scripts/scenario.mjs): STUN messages (RFC 5389), the
// line framing of the pipe and the matching of replies to requests (spec 5.1), the lane addresses
// and their masks, the layout of the renamed copies (spec 8.1), the verdicts of the steps and the
// parsing of what tailscale.exe and tasklist.exe print. Nothing here does I/O.
import { randomUUID } from 'node:crypto';
import { win32 } from 'node:path';
import { StringDecoder } from 'node:string_decoder';

export const STUN_MAGIC = 0x2112a442;
const STUN_BINDING_REQUEST = 0x0001;
const STUN_BINDING_SUCCESS = 0x0101;
const STUN_XOR_MAPPED_ADDRESS = 0x0020;
const STUN_FAMILY_IPV4 = 0x01;

// stunRequest is a Binding request without attributes; txId is the 12-byte transaction id.
export function stunRequest(txId) {
  if (txId.length !== 12) throw new Error('a STUN transaction id is 12 bytes');
  const b = Buffer.alloc(20);
  b.writeUInt16BE(STUN_BINDING_REQUEST, 0);
  b.writeUInt16BE(0, 2);
  b.writeUInt32BE(STUN_MAGIC, 4);
  Buffer.from(txId).copy(b, 8);
  return b;
}

// parseStunResponse gives {ip, port} of the IPv4 XOR-MAPPED-ADDRESS in a Binding success response
// to txId, or null for anything else: another transaction, an error response, a cut message.
export function parseStunResponse(buf, txId) {
  if (buf.length < 20 || buf.readUInt16BE(0) !== STUN_BINDING_SUCCESS || buf.readUInt32BE(4) !== STUN_MAGIC) {
    return null;
  }
  if (!buf.subarray(8, 20).equals(Buffer.from(txId))) return null;
  const end = 20 + buf.readUInt16BE(2);
  if (end > buf.length) return null;
  for (let at = 20; at + 4 <= end;) {
    const type = buf.readUInt16BE(at);
    const len = buf.readUInt16BE(at + 2);
    const value = at + 4;
    if (value + len > end) return null;
    if (type === STUN_XOR_MAPPED_ADDRESS && len >= 8 && buf[value + 1] === STUN_FAMILY_IPV4) {
      const port = buf.readUInt16BE(value + 2) ^ (STUN_MAGIC >>> 16);
      const addr = (buf.readUInt32BE(value + 4) ^ STUN_MAGIC) >>> 0;
      return { ip: [addr >>> 24, (addr >>> 16) & 255, (addr >>> 8) & 255, addr & 255].join('.'), port };
    }
    at = value + len + ((4 - (len % 4)) % 4); // attributes are padded to 4 bytes
  }
  return null;
}

// MAX_LINE is the longest message line of the pipe (spec 5.1: 1 MiB).
export const MAX_LINE = 1 << 20;

// createLineReader frames a byte stream into lines: push takes the chunks as they come; a line may
// span chunks, a chunk may hold many lines, a character split between chunks is joined. A trailing
// \r is dropped and empty lines are skipped; a line longer than max characters throws.
export function createLineReader(onLine, max = MAX_LINE) {
  const decoder = new StringDecoder('utf8');
  let rest = '';
  const tooLong = () => new Error(`a line longer than ${max} characters`);
  return {
    push(chunk) {
      rest += typeof chunk === 'string' ? chunk : decoder.write(chunk);
      for (let nl = rest.indexOf('\n'); nl >= 0; nl = rest.indexOf('\n')) {
        const line = rest.slice(0, nl).replace(/\r$/, '');
        rest = rest.slice(nl + 1);
        if (line.length > max) throw tooLong();
        if (line !== '') onLine(line);
      }
      if (rest.length > max) throw tooLong();
    },
  };
}

// createRpc numbers the requests (spec 5.1: {id, method, params}) and matches each reply to its
// request by id. send writes one line. call resolves with the reply, {id, result} or {id, error};
// it rejects only when no reply comes within timeoutMs or the connection ends (fail).
export function createRpc(send) {
  let next = 1;
  const pending = new Map();
  return {
    call(method, params, timeoutMs = 30000) {
      const id = next++;
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          pending.delete(id);
          reject(new Error(`no reply to ${method} within ${timeoutMs} ms`));
        }, timeoutMs);
        pending.set(id, { resolve, reject, timer });
        send(JSON.stringify(params === undefined ? { id, method } : { id, method, params }) + '\n');
      });
    },
    // handle takes one parsed message and says whether it was the reply to a pending call.
    handle(msg) {
      if (msg === null || typeof msg !== 'object' || 'event' in msg) return false;
      const p = pending.get(msg.id);
      if (!p) return false;
      pending.delete(msg.id);
      clearTimeout(p.timer);
      p.resolve(msg);
      return true;
    },
    fail(err) {
      for (const p of pending.values()) {
        clearTimeout(p.timer);
        p.reject(err);
      }
      pending.clear();
    },
    pending: () => pending.size,
  };
}

export function isIPv4(s) {
  return typeof s === 'string' && /^\d{1,3}(\.\d{1,3}){3}$/.test(s) && s.split('.').every((n) => Number(n) <= 255);
}

// addressBook keeps every lane address the helper has reported in state.addresses (spec 5.2);
// learn adds the addresses of one state and returns the book.
export function addressBook() {
  return { vpn: [], direct: [] };
}

export function learn(book, addresses) {
  for (const lane of ['vpn', 'direct']) {
    const ip = addresses?.[lane];
    if (isIPv4(ip) && !book[lane].includes(ip)) book[lane].push(ip);
  }
  return book;
}

// laneOf names the lane of an address: vpn, direct, other, or none when there was no reply.
export function laneOf(ip, book) {
  if (!ip) return 'none';
  if (book.direct.includes(ip)) return 'direct';
  if (book.vpn.includes(ip)) return 'vpn';
  return 'other';
}

export function mask(ip, book) {
  return `<${laneOf(ip, book)}>`;
}

// maskText replaces every IPv4 address in a text by its lane: the output never carries an address.
export function maskText(text, book) {
  return String(text).replace(/\b\d{1,3}(?:\.\d{1,3}){3}\b/g, (ip) => mask(ip, book));
}

// lanesKnown: step 1 needs both lane addresses, and different ones (spec 12).
export function lanesKnown(addresses) {
  return isIPv4(addresses?.vpn) && isIPv4(addresses?.direct) && addresses.vpn !== addresses.direct;
}

// verdictLane judges the lanes of the replies of one copy (laneOf of each try), like slctl
// expect-lanes: a copy of the VPN lane fails at once on a reply from the direct address; otherwise
// the first reply is right, or two of the first three are (the exit node's external address may
// change). done is false while more tries are needed; six tries at most.
export function verdictLane(seen, want) {
  const answers = seen.filter((lane) => lane !== 'none');
  if (want === 'vpn' && answers.includes('direct')) return { ok: false, done: true };
  if (answers.length > 0 && answers[0] === want) return { ok: true, done: true };
  if (answers.length >= 3) return { ok: answers.slice(0, 3).filter((lane) => lane === want).length >= 2, done: true };
  if (seen.length >= 6) return { ok: false, done: true };
  return { ok: false, done: false };
}

// verdictSwitch is step 4 (and the return from "all direct" in step 5): samples are {t, ip, conn}
// replies of one keep-alive copy, conn the number of its connection; tSet is when the switch was
// sent, and a reply stamped tSet came before it. Before the switch the copy answered from oldIp
// at least twice over one connection (it keeps the connection); the first reply from newIp comes
// within limitMs after tSet, and no reply from oldIp follows it. firstMs is the time from tSet to
// that first reply.
export function verdictSwitch(samples, tSet, oldIp, newIp, limitMs = 3000) {
  const sorted = [...samples].sort((a, b) => a.t - b.t);
  const old = sorted.filter((s) => s.t <= tSet && s.ip === oldIp);
  if (old.length === 0) return { ok: false, firstMs: null, why: 'no reply from the old lane before the switch' };
  if (!old.some((s, i) => old.findIndex((o) => o.conn === s.conn) !== i)) {
    return { ok: false, firstMs: null, why: 'the copy did not keep its connection before the switch' };
  }
  const first = sorted.find((s) => s.t > tSet && s.ip === newIp);
  if (!first) return { ok: false, firstMs: null, why: 'no reply from the new lane' };
  const firstMs = first.t - tSet;
  if (firstMs > limitMs) {
    return { ok: false, firstMs, why: `the first reply from the new lane came later than ${limitMs} ms` };
  }
  if (sorted.some((s) => s.t > first.t && s.ip === oldIp)) {
    return { ok: false, firstMs, why: 'a reply from the old lane after the first one from the new lane' };
  }
  return { ok: true, firstMs, why: '' };
}

// createAllDirectTracker keeps the windows of "all direct" for step 7. The scenario tells when it
// asks for the switch (requested) and what each state event says (observed). A window opens at the
// request to switch on, or at a state with allDirect true. It closes at a state with allDirect
// false that comes after a state with true or after the request to switch off: the states sent
// while the switch is still being applied (up to 3 s, spec 8.3) carry the old false.
export function createAllDirectTracker() {
  const windows = [];
  let open = null;
  const begin = (t, seenOn) => {
    open = { from: t, to: Infinity, seenOn, offAsked: false };
    windows.push(open);
  };
  return {
    requested(on, t) {
      if (on && !open) begin(t, false);
      else if (!on && open) open.offAsked = true;
    },
    observed(allDirect, t) {
      if (allDirect) {
        if (open) open.seenOn = true;
        else begin(t, true);
      } else if (open && (open.seenOn || open.offAsked)) {
        open.to = t;
        open = null;
      }
    },
    windows: () => windows.map(({ from, to }) => ({ from, to })),
  };
}

// coreAliveThroughout: core is the list of looks at the core process, {t, alive} (alive null when
// the look failed). True when the last look at or before from and every look up to the first one
// at or after to saw the core.
export function coreAliveThroughout(core, from, to) {
  let start = -1;
  for (let i = 0; i < core.length; i++) {
    if (core[i].t <= from) start = i;
  }
  if (start < 0) return false;
  for (let i = start; i < core.length; i++) {
    if (core[i].alive !== true) return false;
    if (core[i].t >= to) return true;
  }
  return false;
}

// verdictLeak is step 7: samples are {sent, done, ip} replies of the VPN copy of curl during the
// whole scenario. A reply counts when the core lived through the request and the request did not
// overlap a window of "all direct"; a counted reply from a direct address is a leak. With no
// counted reply there is nothing to judge, and the step fails.
export function verdictLeak(samples, core, windows, book) {
  let counted = 0;
  let ignored = 0;
  let leaks = 0;
  for (const s of samples) {
    const allDirect = windows.some((w) => s.sent <= w.to && s.done >= w.from);
    if (allDirect || !coreAliveThroughout(core, s.sent, s.done)) {
      ignored++;
      continue;
    }
    counted++;
    if (laneOf(s.ip, book) === 'direct') leaks++;
  }
  return { ok: counted > 0 && leaks === 0, counted, ignored, leaks };
}

// scenarioPaths are the copies of spec 12 under root (%TEMP%\splitlane-scenario): a folder for each
// lane and one for the free copies; for step 6 the folder nest, pinned direct as a whole, with
// nest\curl-outer.exe and nest\inner\curl-nest.exe (pinned vpn).
export function scenarioPaths(root) {
  const at = (...parts) => win32.join(root, ...parts);
  return {
    root,
    curlVpn: at('vpn', 'curl-vpn.exe'),
    nodeVpn: at('vpn', 'node-vpn.exe'),
    curlDirect: at('direct', 'curl-direct.exe'),
    nodeDirect: at('direct', 'node-direct.exe'),
    curlFree: at('free', 'curl-free.exe'),
    nodeFree: at('free', 'node-free.exe'),
    curlOuter: at('nest', 'curl-outer.exe'),
    curlNest: at('nest', 'inner', 'curl-nest.exe'),
  };
}

// buildLayout is the lanes.set of the scenario (spec 8.1): new UUIDs, an empty matcher.value (the
// helper computes it), kind file for each pinned copy and kind folder for nest; the free copies
// are not pinned; the main lane is vpn.
export function buildLayout(p) {
  const rule = (exe, kind, lane) => ({
    id: randomUUID(),
    name: `scenario ${kind === 'folder' ? win32.basename(win32.dirname(exe)) : win32.basename(exe, '.exe')}`,
    exe,
    matcher: { kind, value: '' },
    lane,
  });
  return {
    apps: [
      rule(p.curlVpn, 'file', 'vpn'),
      rule(p.nodeVpn, 'file', 'vpn'),
      rule(p.curlDirect, 'file', 'direct'),
      rule(p.nodeDirect, 'file', 'direct'),
      rule(p.curlOuter, 'folder', 'direct'),
      rule(p.curlNest, 'file', 'vpn'),
    ],
    defaultLane: 'vpn',
  };
}

// withLane is a copy of the layout in which the rule of exe goes to lane, with the same ids.
export function withLane(layout, exe, lane) {
  return { ...layout, apps: layout.apps.map((a) => (a.exe === exe ? { ...a, matcher: { ...a.matcher }, lane } : a)) };
}

export function withDefault(layout, lane) {
  return { ...layout, defaultLane: lane };
}

// parseSample reads a line of a child: "<unix ms> <ip>", or "<unix ms> -" when there was no reply;
// the keep child adds the number of the connection: "<unix ms> <ip> <conn>".
export function parseSample(line) {
  const [t, ip, conn] = String(line).trim().split(/\s+/);
  return { t: Number(t), ip: isIPv4(ip) ? ip : null, conn: conn === undefined ? null : Number(conn) };
}

// systemExitNode reads the system Tailscale client as Get-P1SystemExitNode does (spike\plan1\p1lib.ps1):
// the exit node from `tailscale status --json` (ExitNodeStatus.TailscaleIPs, the IPv4 one without its
// prefix length, only while BackendState is Running), the LAN flag from `tailscale debug prefs`
// (ExitNodeAllowLANAccess). null when either output is not JSON.
export function systemExitNode(statusText, prefsText) {
  let status;
  let prefs;
  try {
    status = JSON.parse(statusText);
    prefs = JSON.parse(prefsText);
  } catch {
    return null;
  }
  let ip = null;
  if (status?.BackendState === 'Running' && status.ExitNodeStatus) {
    const ips = (status.ExitNodeStatus.TailscaleIPs ?? []).map((a) => String(a).split('/')[0]);
    ip = ips.find((a) => !a.includes(':')) ?? null;
  }
  return { installed: true, on: ip !== null, ip, allowLan: Boolean(prefs?.ExitNodeAllowLANAccess) };
}

// sameSystem compares two readings of the system client (step 8); an unreadable one (null) never
// equals anything.
export function sameSystem(a, b) {
  return a !== null && b !== null && ['installed', 'on', 'ip', 'allowLan'].every((k) => a[k] === b[k]);
}

// parseTasklist gives the process ids of sing-box.exe in `tasklist /fo csv /nh` output.
export function parseTasklist(text) {
  const pids = [];
  for (const line of String(text).split(/\r?\n/)) {
    const m = /^"([^"]+)","(\d+)"/.exec(line.trim());
    if (m && m[1].toLowerCase() === 'sing-box.exe') pids.push(Number(m[2]));
  }
  return pids;
}

// heartbeatText is what Set-P1Heartbeat (spike\plan1\p1lib.ps1) writes to guard\deadline.txt:
// the unix time in seconds 400 s from now, no newline.
export function heartbeatText(nowMs) {
  return String(Math.floor(nowMs / 1000) + 400);
}
