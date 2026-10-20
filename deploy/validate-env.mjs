// Resolved-environment checks for deploy/validate-env.sh.
//
// Input: `docker compose config --format json` on stdin.
// The compose source arrives base64 in HELIOLYTICS_VALIDATOR_COMPOSE_SOURCE.
// Output: field-name-only reasons, or one PASS line per check group.
// It never prints a value, a hash, a DSN or a fingerprint.

import process from 'node:process';

const SCRYPT = /^scrypt\$N=32768\$r=8\$p=3\$[0-9a-f]{32}\$[0-9a-f]{64}$/;
const SHA40 = /^[0-9a-f]{40}$/;
const RFC3339_UTC = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/;
const CIDR_SINGLE = /^(?:[0-9.]+|\[[0-9a-fA-F:]+\])\/(?:32|128)$/;
const HEX64 = /^[0-9a-f]{64}$/;
const EXPECTED_BINDS = [
  '/docker-entrypoint-initdb.d/00_roles.sh',
  '/docker-entrypoint-initdb.d/01_schema.sql',
  '/etc/postgresql/postgresql.conf',
  '/etc/postgresql/pg_hba.conf',
];

class CheckError extends Error {
  constructor(field, reason) {
    super(reason);
    this.field = field;
  }
}

function fail(field, reason) {
  throw new CheckError(field, reason);
}

function asObject(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function service(config, name) {
  const services = asObject(config.services);
  const found = services[name];
  if (!found) fail(`compose.${name}`, 'service is missing from the resolved file');
  return asObject(found);
}

function env(serviceConfig, key) {
  const values = asObject(serviceConfig.environment);
  return typeof values[key] === 'string' ? values[key] : '';
}

function hasEnv(serviceConfig, key) {
  return Object.prototype.hasOwnProperty.call(asObject(serviceConfig.environment), key);
}

// Rendered config output escapes a literal dollar as two dollars, while the
// container receives one, so resolved values are compared after unescaping.
function unescapeDollars(value) {
  return String(value).replace(/\$\$/g, '$');
}

function networksOf(serviceConfig) {
  return asObject(serviceConfig.networks);
}

function portBindings(serviceConfig) {
  return asArray(serviceConfig.ports).map((entry) => asObject(entry));
}

function mountedTargets(serviceConfig) {
  return asArray(serviceConfig.volumes).map((entry) => asObject(entry));
}

function tmpfsEntries(serviceConfig) {
  const value = serviceConfig.tmpfs;
  if (Array.isArray(value)) return value.map(String);
  if (typeof value === 'string') return [value];
  return [];
}

function requireBool(value, field, reason) {
  if (value !== true) fail(field, reason);
}

function requireLimit(serviceConfig, name) {
  const limits = asObject(serviceConfig.deploy);
  const memory = serviceConfig.mem_limit ?? limits.limits?.memory;
  const cpus = serviceConfig.cpus ?? limits.limits?.cpus;
  const pids = serviceConfig.pids_limit ?? limits.pids_limit;
  if (memory === undefined || memory === null || memory === 0) {
    fail(`compose.${name}.mem_limit`, 'memory limit is not set');
  }
  if (cpus === undefined || cpus === null || cpus === 0) {
    fail(`compose.${name}.cpus`, 'CPU limit is not set');
  }
  if (pids === undefined || pids === null || pids === 0) {
    fail(`compose.${name}.pids_limit`, 'PID limit is not set');
  }
}

function requireLogging(serviceConfig, name) {
  const logging = asObject(serviceConfig.logging);
  if (logging.driver !== 'json-file') fail(`compose.${name}.logging`, 'json-file driver is required');
  const options = asObject(logging.options);
  if (options['max-size'] !== '10m' || String(options['max-file']) !== '3') {
    fail(`compose.${name}.logging`, 'bounded max-size 10m and max-file 3 are required');
  }
}

function checkPorts(serviceConfig, name, target) {
  const bindings = portBindings(serviceConfig);
  if (bindings.length !== 1) fail(`compose.${name}.ports`, 'exactly one published port is required');
  const binding = bindings[0];
  if (binding.host_ip !== '127.0.0.1') fail(`compose.${name}.ports`, 'the port must bind 127.0.0.1');
  if (Number(binding.target) !== target) fail(`compose.${name}.ports`, 'unexpected container port');
}

function checkHardening(serviceConfig, name) {
  requireBool(serviceConfig.read_only, `compose.${name}.read_only`, 'a read-only root filesystem is required');
  const capDrop = asArray(serviceConfig.cap_drop).map(String);
  if (!capDrop.includes('ALL')) fail(`compose.${name}.cap_drop`, 'cap_drop ALL is required');
  const security = asArray(serviceConfig.security_opt).map((entry) => String(entry).toLowerCase());
  if (!security.includes('no-new-privileges:true')) {
    fail(`compose.${name}.security_opt`, 'no-new-privileges is required');
  }
}

function checkNoHostAccess(serviceConfig, name) {
  if (serviceConfig.privileged === true) fail(`compose.${name}.privileged`, 'privileged mode is forbidden');
  if (serviceConfig.network_mode) fail(`compose.${name}.network_mode`, 'host or shared networking is forbidden');
  for (const mount of mountedTargets(serviceConfig)) {
    const source = typeof mount.source === 'string' ? mount.source : '';
    if (source.includes('docker.sock')) fail(`compose.${name}.volumes`, 'the docker socket must not be mounted');
  }
}

function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Compose 2.x drops an explicit create_host_path false from resolved output, so
// the long-form bind declarations are checked in the compose source itself.
function requireLongBinds(source) {
  const blocks = [];
  let current = null;
  for (const line of source.split(/\r?\n/)) {
    if (/^\s*-\s+type:\s*bind\s*(#.*)?$/.test(line)) {
      current = [line];
      blocks.push(current);
      continue;
    }
    if (!current) continue;
    if (/^\s*(#.*)?$/.test(line)) continue;
    const indent = line.length - line.trimStart().length;
    if (indent <= current[0].length - current[0].trimStart().length) current = null;
    else current.push(line);
  }
  const texts = blocks.map((lines) => lines.join('\n'));
  for (const block of texts) {
    const target = /^\s*target:\s*(\S+)\s*$/m.exec(block);
    const label = target ? target[1] : 'a bind mount';
    if (!/^\s*source:\s*\S+\s*$/m.test(block)) {
      fail('compose.bind_mounts', `mount ${label} must name a source`);
    }
    if (!/^\s*read_only:\s*true\s*$/m.test(block)) {
      fail('compose.bind_mounts', `mount ${label} must be read-only`);
    }
    if (!/create_host_path:\s*false\b/.test(block)) {
      fail('compose.bind_mounts', `mount ${label} must set create_host_path false`);
    }
  }
  for (const target of EXPECTED_BINDS) {
    const pattern = new RegExp(`^\\s*target:\\s*${escapeRegExp(target)}\\s*$`, 'm');
    if (!texts.some((block) => pattern.test(block))) {
      fail('compose.bind_mounts', `missing long-form read-only bind mount ${target}`);
    }
  }
}

function checkHashPreservation(resolvedHash) {
  const expected = process.env.HELIOLYTICS_EXPECTED_WEB_PASSWORD_HASH;
  if (!expected) return;
  const fromEnvFile = process.env.HELIOLYTICS_WEB_PASSWORD_HASH ?? '';
  if (!SCRYPT.test(expected)) fail('expected_web_hash_file', 'generator output is not a verifier');
  if (fromEnvFile !== expected) {
    fail('HELIOLYTICS_WEB_PASSWORD_HASH', 'process environment differs from the generator output');
  }
  if (resolvedHash !== expected) {
    fail('HELIOLYTICS_WEB_PASSWORD_HASH', 'resolved compose value differs from the generator output');
  }
  process.stdout.write('web hash literal preserved: PASS\n');
}

function validate(config) {
  const db = service(config, 'db');
  const api = service(config, 'api');
  const web = service(config, 'web');

  // Networks: internal for data and application, bridge for ingress and tools.
  const networks = asObject(config.networks);
  if (asObject(networks.data).internal !== true) fail('compose.networks.data', 'data network must be internal');
  if (asObject(networks.application).internal !== true) {
    fail('compose.networks.application', 'application network must be internal');
  }
  if (asObject(networks.ingress).internal === true) fail('compose.networks.ingress', 'ingress must not be internal');
  if (asObject(networks.tools).internal === true) {
    fail('compose.networks.tools', 'tools must not be internal because it publishes a port');
  }
  for (const [name, network] of Object.entries(networks)) {
    if (asObject(network).external === true) fail(`compose.networks.${name}`, 'external networks are forbidden');
  }

  const apiNetworks = Object.keys(networksOf(api));
  for (const required of ['data', 'application', 'ingress']) {
    if (!apiNetworks.includes(required)) fail('compose.api.networks', `api must join ${required}`);
  }
  const webNetworks = Object.keys(networksOf(web));
  for (const required of ['application', 'ingress']) {
    if (!webNetworks.includes(required)) fail('compose.web.networks', `web must join ${required}`);
  }
  const dbNetworks = Object.keys(networksOf(db));
  if (dbNetworks.length !== 1 || dbNetworks[0] !== 'data') fail('compose.db.networks', 'db must join data only');

  // db: no published port, four read-only binds, one named data volume.
  if (portBindings(db).length !== 0) fail('compose.db.ports', 'the database must not publish a port');
  const source = Buffer.from(process.env.HELIOLYTICS_VALIDATOR_COMPOSE_SOURCE ?? '', 'base64').toString('utf8');
  if (!source.includes('services:')) fail('compose', 'the compose source was not supplied');
  requireLongBinds(source);
  const binds = mountedTargets(db).filter((mount) => mount.type === 'bind');
  for (const target of EXPECTED_BINDS) {
    const mount = binds.find((entry) => entry.target === target);
    if (!mount) fail('compose.db.volumes', `missing required read-only mount ${target}`);
    if (mount.read_only !== true) fail('compose.db.volumes', `mount ${target} must be read-only`);
    if (asObject(mount.bind).create_host_path === true) {
      fail('compose.db.volumes', `mount ${target} must not create the host path`);
    }
  }
  const dataMount = mountedTargets(db).find((mount) => mount.target === '/var/lib/postgresql/data');
  if (!dataMount || dataMount.type !== 'volume') {
    fail('compose.db.volumes', 'the data path must be a named volume');
  }
  if (!String(dataMount.source ?? '').endsWith('pgdata')) {
    fail('compose.db.volumes', 'the data volume name must end with pgdata');
  }
  if (!db.healthcheck) fail('compose.db.healthcheck', 'an authenticated healthcheck is required');

  // api: app role DSN only, no web-only secret, bounded limits and hardening.
  const dsn = env(api, 'DATABASE_URL');
  if (!dsn.startsWith('postgres://heliolytics_app:')) {
    fail('compose.api.DATABASE_URL', 'api must connect as the restricted app role');
  }
  if (!dsn.endsWith('@db:5432/heliolytics?sslmode=disable')) {
    fail('compose.api.DATABASE_URL', 'api must target the db service and heliolytics database');
  }
  for (const forbidden of [
    'HELIOLYTICS_SIGNING_SECRET',
    'HELIOLYTICS_SESSION_SECRET',
    'HELIOLYTICS_WEB_PASSWORD_HASH',
    'HELIOLYTICS_API_KEY',
    'REPARSE_ENABLED',
    'REPARSE_SECRET',
  ]) {
    if (hasEnv(api, forbidden)) fail('compose.api.environment', `${forbidden} must not reach the api`);
  }
  if (env(api, 'TRUST_PROXY') !== 'true') fail('compose.api.TRUST_PROXY', 'production requires trusted proxy mode');
  const cidrs = env(api, 'TRUSTED_PROXY_CIDRS').split(',').filter(Boolean);
  if (cidrs.length === 0) fail('compose.api.TRUSTED_PROXY_CIDRS', 'trusted proxy CIDRs are required');
  for (const cidr of cidrs) {
    if (!CIDR_SINGLE.test(cidr)) fail('compose.api.TRUSTED_PROXY_CIDRS', 'each entry must be a single host CIDR');
  }
  const ring = env(api, 'HELIOLYTICS_API_KEYS').split(',');
  const kids = [];
  for (const entry of ring) {
    const match = /^(phone|web)\.[0-9a-f]{64}$/.exec(entry);
    if (!match) fail('compose.api.HELIOLYTICS_API_KEYS', 'each key must be <phone|web>.<64 lowercase hex>');
    kids.push(match[1]);
  }
  if (ring.length !== 2 || new Set(kids).size !== 2) {
    fail('compose.api.HELIOLYTICS_API_KEYS', 'exactly one phone and one web key are required');
  }
  checkPorts(api, 'api', 8080);
  checkHardening(api, 'api');
  checkNoHostAccess(api, 'api');
  requireLimit(api, 'api');
  requireLogging(api, 'api');
  if (!tmpfsEntries(api).some((entry) => entry.startsWith('/tmp:'))) {
    fail('compose.api.tmpfs', 'a bounded /tmp tmpfs is required');
  }

  // web: nonroot, its own web key, session secret, no database credential.
  if (String(web.user ?? '') !== '1000:1000') fail('compose.web.user', 'web must run as uid and gid 1000');
  if (hasEnv(web, 'DATABASE_URL')) fail('compose.web.environment', 'web must not receive a database URL');
  if (!/^web\.[0-9a-f]{64}$/.test(env(web, 'HELIOLYTICS_API_KEY'))) {
    fail('compose.web.HELIOLYTICS_API_KEY', 'web must receive its own web key');
  }
  if (!HEX64.test(env(web, 'HELIOLYTICS_SESSION_SECRET'))) {
    fail('compose.web.HELIOLYTICS_SESSION_SECRET', 'web session secret must be 64 lowercase hex');
  }
  const resolvedHash = unescapeDollars(env(web, 'HELIOLYTICS_WEB_PASSWORD_HASH'));
  if (!SCRYPT.test(resolvedHash)) {
    fail('compose.web.HELIOLYTICS_WEB_PASSWORD_HASH', 'resolved verifier is not a complete scrypt line');
  }
  if (env(web, 'API_INTERNAL_URL') !== 'http://api:8080') {
    fail('compose.web.API_INTERNAL_URL', 'web must call the api service internally');
  }
  if (env(web, 'COOKIE_SECURE') !== 'true') fail('compose.web.COOKIE_SECURE', 'secure cookies are required');
  checkPorts(web, 'web', 3000);
  checkHardening(web, 'web');
  checkNoHostAccess(web, 'web');
  requireLimit(web, 'web');
  requireLogging(web, 'web');
  const webTmpfs = tmpfsEntries(web);
  if (!webTmpfs.some((entry) => entry.startsWith('/tmp:') && entry.includes('mode=1777'))) {
    fail('compose.web.tmpfs', '/tmp must be mode 1777');
  }
  if (!webTmpfs.some((entry) => entry.startsWith('/app/.next/cache:') && entry.includes('mode=0700'))) {
    fail('compose.web.tmpfs', '.next cache must be mode 0700');
  }

  // Build metadata: both images carry a full SHA and one UTC build time.
  const apiArgs = asObject(asObject(api.build).args);
  const webArgs = asObject(asObject(web.build).args);
  if (!SHA40.test(String(apiArgs.GIT_SHA ?? ''))) fail('compose.api.build.GIT_SHA', 'full lowercase SHA required');
  if (!SHA40.test(String(webArgs.GIT_SHA ?? ''))) fail('compose.web.build.GIT_SHA', 'full lowercase SHA required');
  if (!RFC3339_UTC.test(String(apiArgs.BUILD_TIME ?? ''))) {
    fail('compose.api.build.BUILD_TIME', 'RFC3339 UTC build time required');
  }
  if (String(apiArgs.BUILD_TIME ?? '') !== String(webArgs.BUILD_TIME ?? '')) {
    fail('compose.web.build.BUILD_TIME', 'api and web build times must match');
  }

  // Optional profile services stay out of the default start.
  const allServices = asObject(config.services);
  if (!asArray(asObject(allServices.adminer).profiles).includes('tools')) {
    fail('compose.adminer.profiles', 'adminer must stay behind the tools profile');
  }
  checkPorts(asObject(allServices.adminer), 'adminer', 8080);
  const connector = asObject(allServices.cloudflared);
  if (!asArray(connector.profiles).includes('tunnel')) {
    fail('compose.cloudflared.profiles', 'the connector must stay behind the tunnel profile');
  }
  const connectorIngress = asObject(asObject(connector.networks).ingress);
  if (!String(connectorIngress.ipv4_address ?? '')) {
    fail('compose.cloudflared.networks', 'the connector needs a static ingress address');
  }
  checkNoHostAccess(connector, 'cloudflared');
  requireLimit(connector, 'cloudflared');

  return resolvedHash;
}

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf8');
}

try {
  const raw = await readStdin();
  if (!raw.trim()) fail('compose', 'resolved configuration is empty');
  let config;
  try {
    config = JSON.parse(raw);
  } catch {
    fail('compose', 'resolved configuration is not JSON');
  }
  const resolvedHash = validate(asObject(config));
  checkHashPreservation(resolvedHash);
  process.stdout.write('environment validation: PASS\n');
} catch (error) {
  if (error instanceof CheckError) {
    process.stderr.write(`FAIL ${error.field}: ${error.message}\n`);
  } else {
    process.stderr.write('FAIL compose: validation aborted\n');
  }
  process.exitCode = 1;
}
