// Compares schema/schema.json with the hand-written frontend definitions.
//
// The site has no build step, so this is the drift guard: run it in CI (and
// locally after touching a field) and fix whichever side is wrong.
//
// Usage: node schema/check-frontend.mjs
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import vm from 'node:vm'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const schema = JSON.parse(readFileSync(join(root, 'schema/schema.json'), 'utf8'))
const modulesSource = readFileSync(join(root, 'frontend/js/modules.js'), 'utf8')
const tomlSource = readFileSync(join(root, 'frontend/js/toml.js'), 'utf8')
const zipSource = readFileSync(join(root, 'frontend/js/zip.js'), 'utf8')

const sandbox = {}
vm.createContext(sandbox)
vm.runInContext(`${modulesSource}\n;globalThis.MODULES = MODULES;`, sandbox)
const modules = sandbox.MODULES

const problems = []
const fail = (message) => problems.push(message)

const moduleSchema = schema.properties?.modules?.properties
if (!moduleSchema) fail('schema.json has no modules.properties')

const compareKeys = (label, actual, expected) => {
  for (const key of expected) {
    if (!actual.includes(key)) fail(`${label}: schema is missing ${key}`)
  }
  for (const key of actual) {
    if (!expected.includes(key)) fail(`${label}: schema has ${key}, frontend does not`)
  }
}

if (moduleSchema) {
  for (const def of modules) {
    const node = moduleSchema[def.id]
    if (!node) {
      fail(`${def.id}: missing from schema.json`)
      continue
    }
    const expectedModuleKeys = [
      'enabled',
      ...def.sections.map((section) => section.key),
      ...(def.lists ?? []).map((list) => list.key),
    ]
    compareKeys(`modules.${def.id}`, Object.keys(node.properties ?? {}), expectedModuleKeys)

    for (const section of def.sections) {
      const sectionNode = node.properties?.[section.key]
      if (!sectionNode || sectionNode.type !== 'object') {
        fail(`modules.${def.id}.${section.key}: expected an object in schema.json`)
        continue
      }
      const expectedSectionKeys = section.fields.map((field) => field.key)
      if (section.toggle) expectedSectionKeys.push('enabled')
      compareKeys(
        `modules.${def.id}.${section.key}`,
        Object.keys(sectionNode.properties ?? {}),
        expectedSectionKeys,
      )
    }

    for (const list of def.lists ?? []) {
      const listNode = node.properties?.[list.key]
      if (!listNode || listNode.type !== 'array') {
        fail(`modules.${def.id}.${list.key}: expected an array in schema.json`)
        continue
      }
      if (list.scalar) {
        if (listNode.items?.type !== 'string') {
          fail(`modules.${def.id}.${list.key}: scalar lists must be arrays of strings`)
        }
        continue
      }
      const expectedItemKeys = (list.fields ?? []).map((field) => field.key)
      compareKeys(
        `modules.${def.id}.${list.key}[]`,
        Object.keys(listNode.items?.properties ?? {}),
        expectedItemKeys,
      )
    }
  }
  for (const name of Object.keys(moduleSchema)) {
    if (!modules.some((def) => def.id === name)) {
      fail(`modules.${name}: present in schema.json, missing from frontend/js/modules.js`)
    }
  }
}

const versionMatch = tomlSource.match(/schema_version\s*=\s*(\d+)/)
if (!versionMatch) {
  fail('frontend/js/toml.js does not write schema_version')
} else if (Number(versionMatch[1]) !== schema.schema_version) {
  fail(`frontend writes schema_version ${versionMatch[1]}, schema.json says ${schema.schema_version}`)
}

const supportedMatch = zipSource.match(/SUPPORTED_SCHEMA_VERSION\s*=\s*(\d+)/)
if (!supportedMatch) {
  fail('frontend/js/zip.js has no SUPPORTED_SCHEMA_VERSION')
} else if (Number(supportedMatch[1]) !== schema.schema_version) {
  fail(`zip.js supports schema ${supportedMatch[1]}, schema.json says ${schema.schema_version}`)
}

if (problems.length > 0) {
  console.error(`frontend/schema drift:\n- ${problems.join('\n- ')}`)
  process.exit(1)
}
console.log(`frontend matches schema.json (${modules.length} modules, schema ${schema.schema_version})`)
