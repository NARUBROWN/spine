const assert = require('node:assert/strict');
const { test } = require('node:test');
const ui = require('../../pkg/openapi/ui/swaggerui/assets/swagger-ui-bundle.js');

// Exercise the shipped plugins rather than a second implementation of their logic.
global.self = globalThis;
const samples = ui.plugins.JSONSchema202012Samples({ getSystem: () => system });
const system = { fn: samples.fn };
const sample = schema => samples.fn.jsonSchema202012.getSampleSchema(structuredClone(schema), 'application/json');
const plugin = ui.plugins.JSONSchema202012({ getSystem: () => ({ fn: {}, getComponent: () => {}, getConfigs: () => ({}) }), fn: {} });
const context = plugin.components.withJSONSchema202012Context(() => null)({}).props.value;
const constraints = context.fn.stringifyConstraints;
const resolver = ui.plugins.SwaggerClient({ getSystem: () => resolverSystem, getConfigs: () => ({}), configs: {} });
const resolverSystem = { fn: resolver.fn };

async function resolveField(base, extra) {
  const { spec, errors } = await resolver.fn.resolve({
    baseDoc: 'http://localhost/openapi.json',
    spec: {
      openapi: '3.1.2', info: { title: 'Regression', version: '1' }, paths: {},
      components: { schemas: { Input: { type: 'object', properties: { value: { allOf: [base, extra] } } } } },
    },
  });
  assert.deepEqual(errors, []);
  return spec.components.schemas.Input.properties.value;
}

test('base64url examples use the browser Buffer supported encoding and URL-safe alphabet', async () => {
  const schema = await resolveField({ type: 'string' }, { contentEncoding: 'base64url' });
  assert.equal(JSON.parse(sample(schema)), Buffer.from('string').toString('base64url'));
  const encode = samples.fn.jsonSchema202012.sampleEncoderAPI('base64url');
  for (const bytes of [Buffer.from([0xfb, 0xff]), Buffer.from('f'), Buffer.from('fo'), Buffer.alloc(0)]) {
    assert.equal(encode(bytes), bytes.toString('base64url'));
  }
  assert.equal(JSON.parse(sample({ type: 'string', contentEncoding: 'base64' })), Buffer.from('string').toString('base64'));
});

test('multipleOf formatting handles scientific notation as well as decimal fractions', async () => {
  const schema = await resolveField({ type: 'number' }, { multipleOf: 1e-7 });
  assert.deepEqual(constraints(schema), [{ scope: 'number', value: 'multiple of 1e-7' }]);
  for (const value of [1.25e-7, 1e-308, 2]) {
    assert.deepEqual(constraints({ type: 'number', multipleOf: value }), [{ scope: 'number', value: `multiple of ${value}` }]);
  }
  assert.deepEqual(constraints({ multipleOf: 0.5 }), [{ scope: 'number', value: 'multiple of 5/10' }]);
});

for (const combination of ['anyOf', 'oneOf']) {
  test(`contains.${combination} samples preserve contains constraints without requiring items`, async () => {
    const contains = { [combination]: [{ type: 'string' }], minLength: 5 };
    const schema = await resolveField({}, { type: 'array', contains });
    assert.equal(schema.items, undefined);
    const value = JSON.parse(sample(schema));
    assert.ok(value.some(entry => typeof entry === 'string' && entry.length >= 5));
    assert.doesNotThrow(() => sample({ type: 'array', items: { type: 'number' }, contains }));
  });
}
