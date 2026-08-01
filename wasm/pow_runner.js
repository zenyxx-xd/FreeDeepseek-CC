'use strict';
const fs = require('fs');
const path = require('path');

const WASM_PATH = path.join(__dirname, 'sha3_wasm_bg.wasm');

let compiledModule = null;

async function getModule() {
  if (!compiledModule) {
    const bytes = fs.readFileSync(WASM_PATH);
    compiledModule = await WebAssembly.compile(bytes);
  }
  return compiledModule;
}

async function solvePOW(challenge) {
  const module = await getModule();
  const instance = await WebAssembly.instantiate(module, { wbg: {} });
  const e = instance.exports;
  const encoder = new TextEncoder();
  const prefix = challenge.salt + '_' + challenge.expire_at + '_';
  const cBytes = encoder.encode(challenge.challenge);
  const pBytes = encoder.encode(prefix);
  const cP = e.__wbindgen_export_0(cBytes.length, 1) >>> 0;
  const pP = e.__wbindgen_export_0(pBytes.length, 1) >>> 0;
  new Uint8Array(e.memory.buffer, cP, cBytes.length).set(cBytes);
  new Uint8Array(e.memory.buffer, pP, pBytes.length).set(pBytes);
  const sp = e.__wbindgen_add_to_stack_pointer(-16);
  e.wasm_solve(sp, cP, cBytes.length, pP, pBytes.length, challenge.difficulty);
  const dv = new DataView(e.memory.buffer);
  const code = dv.getInt32(sp, true);
  const ans = dv.getFloat64(sp + 8, true);
  e.__wbindgen_add_to_stack_pointer(16);
  if (code === 0 || !Number.isFinite(ans) || ans <= 0) throw new Error('POW failed');
  return Math.floor(ans);
}

if (process.argv[2]) {
  try {
    const challenge = JSON.parse(process.argv[2]);
    solvePOW(challenge).then(answer => {
      process.stdout.write(JSON.stringify({ answer }));
    }).catch(err => {
      process.stderr.write(err.message || String(err));
      process.exit(1);
    });
  } catch (e) {
    process.stderr.write(e.message);
    process.exit(1);
  }
}

module.exports = { solvePOW };
