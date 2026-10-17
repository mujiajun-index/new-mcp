import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";
import vm from "node:vm";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const ts = require("typescript");
const module = { exports: {} };
vm.runInNewContext(ts.transpileModule(
  readFileSync(new URL("../web/src/lib/task-history.ts", import.meta.url), "utf8"),
  { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } },
).outputText, { module, exports: module.exports });
const { deleteTaskHistory, historyPageCount } = module.exports;

test("batch deletion bounds concurrent requests and deduplicates task IDs", async () => {
  let active = 0;
  let peak = 0;
  const calls = [];
  const ids = Array.from({ length: 10 }, (_, i) => `task-${i}`);
  const result = await deleteTaskHistory([...ids, ids[0]], async (id) => {
    calls.push(id);
    peak = Math.max(peak, ++active);
    await new Promise((resolve) => setTimeout(resolve, 5));
    active--;
  });
  assert.equal(peak, 4);
  assert.deepEqual(calls, ids);
  assert.deepEqual(Array.from(result.deleted), ids);
  assert.equal(result.failed.length, 0);
});

test("partial failures keep exact failed IDs and do not stop later deletions", async () => {
  const result = await deleteTaskHistory(["saved", "running", "busy", "other", "last"], (id) => {
    if (id === "busy") throw new Error("lease held");
    if (id === "running") return Promise.reject(new Error("task is running"));
    return Promise.resolve();
  });
  assert.deepEqual(Array.from(result.deleted), ["saved", "other", "last"]);
  assert.deepEqual(Array.from(result.failed, (failure) => failure.id), ["running", "busy"]);
  assert.equal(result.failed[0].error.message, "task is running");
});

test("empty selection makes no deletion calls", async () => {
  let calls = 0;
  const result = await deleteTaskHistory([], async () => { calls++; });
  assert.equal(calls, 0);
  assert.equal(result.deleted.length, 0);
  assert.equal(result.failed.length, 0);
});

test("pagination remains valid when the last page or all records are deleted", () => {
  assert.equal(historyPageCount(0, 20), 1);
  assert.equal(historyPageCount(20, 20), 1);
  assert.equal(historyPageCount(21, 20), 2);
  assert.equal(Math.min(2, historyPageCount(21 - 1, 20)), 1);
  assert.equal(Math.min(2, historyPageCount(40 - 20, 20)), 1);
  assert.equal(historyPageCount(-1, 20), 1);
});
