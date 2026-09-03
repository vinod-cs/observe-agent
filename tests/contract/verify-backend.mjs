// AGENTV1 FILE START: execute existing backend normalizer read-only; no API or DB writes.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';

const [platformRoot, fixturePath] = process.argv.slice(2);
assert(platformRoot && fixturePath, 'usage: verify-backend.mjs <read-only-platform-root> <fixture>');
const { normalizeOtlpMetricRequest } = await import(pathToFileURL(resolve(platformRoot, 'backend/api/src/shared/normalization/otlp-metric-normalization.ts')));
const batches = JSON.parse(await readFile(fixturePath, 'utf8'));
let total = 0;
for (const batch of batches) {
 const result = normalizeOtlpMetricRequest(batch);
 assert.equal(result.rejectedDataPoints, 0);
 assert(result.points.length > 0 && result.points.length <= 1000);
 for (const point of result.points) {
  assert.equal(point.resourceIdentity.hostId, 'i-0345d461c99a6da2f');
  assert.equal(point.resourceIdentity.cloudAccountId, '127696279140');
  assert.equal(point.resourceIdentity.cloudRegion, 'us-east-2');
  assert.equal(point.resourceIdentity.telemetryDistroName, 'agent-i');
  assert.equal(point.isHostMetric, true);
  assert.equal(point.metricId, 'app.system_disk_io');
  assert.equal(point.unit, 'By');
 }
 total += result.points.length;
}
assert.equal(total, 2001);
console.log(JSON.stringify({normalizer: 'PASS', points: total, rejected: 0, identity: 'exact EC2 resource identity preserved', databasePersistence: 'NOT TESTED'}));
// AGENTV1 FILE END
