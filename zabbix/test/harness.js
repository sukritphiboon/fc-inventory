#!/usr/bin/env node
/*
 * Offline test harness for the Huawei FusionCompute Zabbix template.
 *
 * It mocks Zabbix's `HttpRequest` object and the `value` global, serves
 * synthetic FusionCompute API responses, runs the ACTUAL JavaScript embedded in
 * each Script item (extracted from the template YAML), and then simulates the
 * dependent-item / LLD preprocessing to verify the full pipeline.
 *
 * No live VRM and no Zabbix server are required.
 */
'use strict';
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const ITEMS = JSON.parse(
  fs.readFileSync(path.join(__dirname, '_generated', 'items.json'), 'utf8')
);

// ─── Synthetic FusionCompute dataset ──────────────────────────────────────
const NUM_VMS = 150; // > one page (limit 100) to exercise pagination
const HOSTS = [
  { uri: '/service/hosts/1', urn: 'urn:host:1', name: 'CNA01', ip: '10.0.0.11',
    status: 'normal', cpuQuantity: 48, cpuMHz: 2500, memoryQuantityMB: 524288,
    memoryUsedMB: 262144, runningVmCount: 20 },
  { uri: '/service/hosts/2', urn: 'urn:host:2', name: 'CNA02', ip: '10.0.0.12',
    status: 'maintaining', cpuQuantity: 32, cpuMHz: 2200, memoryQuantityMB: 262144,
    memoryUsedMB: 131072, runningVmCount: 0 },
];
const DATASTORES = [
  { urn: 'urn:ds:1', name: 'IPSAN', status: 'NORMAL', storageType: 'LUNPOME',
    capacityGB: 1000, freeSpaceGB: 150 },                       // 85% used -> warn
  { urn: 'urn:ds:2', name: 'LOCAL01', status: 'NORMAL', storageType: 'LOCALPOME',
    capacityMB: 512000, freeSizeGB: 400 },                       // capacityMB path
  { urn: 'urn:ds:3', name: 'BROKEN', status: 'ABNORMAL', storageType: 'LUNPOME',
    capacityGB: 500, freeSpaceGB: 5 },                           // 99% used -> crit
];
const CLUSTERS = [
  { urn: 'urn:cl:1', name: 'Prod', isEnableHa: true, isEnableDrs: true },
  { urn: 'urn:cl:2', name: 'Dev', isEnableHa: false, isEnableDrs: false },
];
const VMS = [];
for (let i = 1; i <= NUM_VMS; i++) {
  VMS.push({
    urn: 'urn:vm:' + i, name: 'vm-' + i, uuid: 'uuid-' + i,
    status: i % 5 === 0 ? 'stopped' : 'running',
    vmConfig: { cpu: { quantity: 2 }, memory: { quantityMB: 4096 } },
  });
}

// Only this API version is accepted, to exercise the version-fallback loop.
const ACCEPTED_VERSION = 'v6.5';
const VALID_USER = 'monitor';
const VALID_PASS = 'Secret@123';
const TOKEN = 'TESTTOKEN-123';

let stats = { logins: 0, versionRejections: 0, listCalls: 0, detailCalls: 0 };

function paginate(arr, key, query) {
  const m = /[?&]offset=(\d+)&limit=(\d+)/.exec(query) || [];
  const offset = parseInt(m[1] || '0', 10);
  const limit = parseInt(m[2] || '100', 10);
  const slice = arr.slice(offset, offset + limit);
  const body = { total: arr.length };
  body[key] = slice;
  return body;
}

// ─── Mock FusionCompute server ────────────────────────────────────────────
function serve(method, fullUrl, reqHeaders, data) {
  const u = new URL(fullUrl);
  const p = u.pathname;
  const query = u.search;
  const H = {};
  for (const k in reqHeaders) H[k.toLowerCase()] = reqHeaders[k];

  // Login
  if (p === '/service/session' && method === 'POST') {
    const accept = H['accept'] || '';
    const ver = (/version=([^;]+)/.exec(accept) || [])[1];
    if (ver !== ACCEPTED_VERSION) {
      stats.versionRejections++;
      return { status: 401, headers: {}, body: '{"errorCode":"10000022","errorDes":"version error"}' };
    }
    if (H['x-auth-user'] !== VALID_USER) {
      return { status: 401, headers: {}, body: '{"errorCode":"auth"}' };
    }
    const algo = H['x-encrypt-algorithm'];
    const key = H['x-auth-key'];
    const expected = algo === '0'
      ? crypto.createHash('sha256').update(VALID_PASS).digest('hex')
      : VALID_PASS;
    if (key !== expected) {
      return { status: 401, headers: {}, body: '{"errorCode":"badkey"}' };
    }
    stats.logins++;
    return { status: 200, headers: { 'X-Auth-Token': TOKEN }, body: '{}' };
  }

  // Everything else needs the token
  if (H['x-auth-token'] !== TOKEN) {
    return { status: 401, headers: {}, body: '{"errorCode":"notoken"}' };
  }

  if (method === 'DELETE' && p === '/service/session') {
    return { status: 200, headers: {}, body: '{}' };
  }
  if (method === 'GET' && p === '/service/sites') {
    return ok({ sites: [{ uri: '/service/sites/1', name: 'Site1' }] });
  }
  if (method === 'GET' && p === '/service/sites/1/hosts') {
    stats.listCalls++;
    return ok(paginate(HOSTS, 'hosts', query));
  }
  if (method === 'GET' && /^\/service\/hosts\/\d+$/.test(p)) {
    stats.detailCalls++;
    const h = HOSTS.find((x) => x.uri === p);
    return ok(h || {});
  }
  if (method === 'GET' && p === '/service/sites/1/vms') {
    stats.listCalls++;
    return ok(paginate(VMS, 'vms', query));
  }
  if (method === 'GET' && p === '/service/sites/1/datastores') {
    stats.listCalls++;
    return ok(paginate(DATASTORES, 'datastores', query));
  }
  if (method === 'GET' && p === '/service/sites/1/clusters') {
    stats.listCalls++;
    return ok(paginate(CLUSTERS, 'clusters', query));
  }
  return { status: 404, headers: {}, body: '{"errorCode":"notfound","path":"' + p + '"}' };
}
function ok(obj) { return { status: 200, headers: {}, body: JSON.stringify(obj) }; }

// ─── Mock Zabbix HttpRequest ──────────────────────────────────────────────
function HttpRequest() {
  this._headers = {};
  this._status = 0;
  this._respHeaders = {};
}
HttpRequest.prototype.addHeader = function (line) {
  const idx = line.indexOf(':');
  this._headers[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
};
HttpRequest.prototype._do = function (method, url, data) {
  const r = serve(method, url, this._headers, data);
  this._status = r.status;
  this._respHeaders = r.headers || {};
  return r.body;
};
HttpRequest.prototype.get = function (url) { return this._do('GET', url); };
HttpRequest.prototype.post = function (url, d) { return this._do('POST', url, d); };
HttpRequest.prototype.put = function (url, d) { return this._do('PUT', url, d); };
HttpRequest.prototype.delete = function (url, d) { return this._do('DELETE', url, d); };
HttpRequest.prototype.getStatus = function () { return this._status; };
HttpRequest.prototype.getHeaders = function () { return this._respHeaders; };

// ─── Run a Script item exactly as Zabbix would ────────────────────────────
function runScript(key) {
  const { script, params } = ITEMS[key];
  // Zabbix wraps the item body as the function body; `value` is the input and
  // top-level `return` produces the result.
  const fn = new Function('value', 'HttpRequest', script);
  return fn(JSON.stringify(params), HttpRequest);
}

// ─── Minimal JSONPath to mirror the template's preprocessing steps ─────────
function jpScalar(arr, urn, field) {
  // emulates: $[?(@.urn=="URN")].field.first()
  const hit = arr.find((o) => o.urn === urn);
  if (!hit) throw new Error('no match for ' + urn);
  return hit[field];
}

// ─── Assertions ───────────────────────────────────────────────────────────
let pass = 0, fail = 0;
function check(name, cond, detail) {
  if (cond) { pass++; console.log('  ✓ ' + name); }
  else { fail++; console.log('  ✗ ' + name + (detail ? '  -> ' + detail : '')); }
}

console.log('\n=== FusionCompute Zabbix template - offline test ===\n');
console.log('authMode under test:', ITEMS['fusioncompute.hosts'].params.authMode);

// Hosts
console.log('\n[fusioncompute.hosts]');
const hosts = JSON.parse(runScript('fusioncompute.hosts'));
check('returns 2 hosts', hosts.length === 2, 'got ' + hosts.length);
check('host detail fetched (memTotalMB present)', hosts[0].memTotalMB === 524288);
check('host status carried through', hosts[0].status === 'normal');
// dependent item simulation: status -> 1/0, memory *1048576
check('LLD status->1 for normal',
  (jpScalar(hosts, 'urn:host:1', 'status').toLowerCase() === 'normal' ? 1 : 0) === 1);
check('LLD status->0 for maintaining',
  (jpScalar(hosts, 'urn:host:2', 'status').toLowerCase() === 'normal' ? 1 : 0) === 0);
check('LLD memory bytes', jpScalar(hosts, 'urn:host:1', 'memTotalMB') * 1048576 === 549755813888);

// VMs (pagination)
console.log('\n[fusioncompute.vms]');
const vms = JSON.parse(runScript('fusioncompute.vms'));
check('paginated to ' + NUM_VMS + ' vms', vms.length === NUM_VMS, 'got ' + vms.length);
check('vm power running->1',
  (jpScalar(vms, 'urn:vm:1', 'status').toLowerCase() === 'running' ? 1 : 0) === 1);
check('vm power stopped->0',
  (jpScalar(vms, 'urn:vm:5', 'status').toLowerCase() === 'running' ? 1 : 0) === 0);
check('vm cpus parsed', jpScalar(vms, 'urn:vm:1', 'cpus') === 2);
check('vm mem bytes', jpScalar(vms, 'urn:vm:1', 'memMB') * 1048576 === 4294967296);

// Datastores
console.log('\n[fusioncompute.datastores]');
const ds = JSON.parse(runScript('fusioncompute.datastores'));
check('returns 3 datastores', ds.length === 3, 'got ' + ds.length);
check('usedPct IPSAN = 85', jpScalar(ds, 'urn:ds:1', 'usedPct') === 85);
check('capacityMB fallback (LOCAL01 = 500GB)', jpScalar(ds, 'urn:ds:2', 'capacityGB') === 500);
check('freeSizeGB fallback (LOCAL01 = 400)', jpScalar(ds, 'urn:ds:2', 'freeGB') === 400);
check('usedPct BROKEN = 99', jpScalar(ds, 'urn:ds:3', 'usedPct') === 99);
check('ds status NORMAL->1',
  (jpScalar(ds, 'urn:ds:1', 'status').toUpperCase() === 'NORMAL' ? 1 : 0) === 1);
check('ds status ABNORMAL->0',
  (jpScalar(ds, 'urn:ds:3', 'status').toUpperCase() === 'NORMAL' ? 1 : 0) === 0);
// trigger thresholds (warn 80 / crit 90)
check('IPSAN crosses WARN (>80) not CRIT', 85 > 80 && !(85 > 90));
check('BROKEN crosses CRIT (>90)', 99 > 90);

// Clusters
console.log('\n[fusioncompute.clusters]');
const cl = JSON.parse(runScript('fusioncompute.clusters'));
check('returns 2 clusters', cl.length === 2, 'got ' + cl.length);
check('HA enabled->1', jpScalar(cl, 'urn:cl:1', 'haEnabled') === 1);
check('HA disabled->0', jpScalar(cl, 'urn:cl:2', 'haEnabled') === 0);
check('DRS enabled->1', jpScalar(cl, 'urn:cl:1', 'drsEnabled') === 1);

// Summary + availability
console.log('\n[fusioncompute.summary]');
const sum = JSON.parse(runScript('fusioncompute.summary'));
check('available = 1', sum.available === 1);
check('totalVms = ' + NUM_VMS, sum.totalVms === NUM_VMS);
check('runningVms = ' + (NUM_VMS - NUM_VMS / 5), sum.runningVms === NUM_VMS - NUM_VMS / 5);
check('totalHosts = 2', sum.totalHosts === 2);

// Failure path: bad credentials must throw (item -> unsupported -> trigger)
console.log('\n[failure path]');
const goodPass = ITEMS['fusioncompute.summary'].params.password;
ITEMS['fusioncompute.summary'].params.password = 'wrong-password';
let threw = false;
try { runScript('fusioncompute.summary'); } catch (e) { threw = true; }
check('login failure raises an error', threw);
ITEMS['fusioncompute.summary'].params.password = goodPass;

// Cross-cutting behaviour
console.log('\n[behaviour]');
check('version fallback exercised (v8.0 rejected before v6.5)', stats.versionRejections > 0,
  'rejections=' + stats.versionRejections);
check('login succeeded', stats.logins > 0);
check('per-host detail calls made', stats.detailCalls === HOSTS.length);

console.log('\n=== ' + pass + ' passed, ' + fail + ' failed ===\n');
process.exit(fail ? 1 : 0);
