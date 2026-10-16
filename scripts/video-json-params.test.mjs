import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";
import vm from "node:vm";

// Use the installed frontend compiler to exercise the production parser without
// loading Axios, localStorage or React in Node. No additional test dependency.
const require = createRequire(new URL("../web/package.json", import.meta.url));
const ts = require("typescript");
const compiled = ts.transpileModule(
  readFileSync(
    new URL("../web/src/features/videogen/api.ts", import.meta.url),
    "utf8",
  ),
  {
    compilerOptions: { module: ts.ModuleKind.CommonJS, esModuleInterop: true },
  },
).outputText;

for (const language of ["zh", "en"]) {
  const strings = JSON.parse(
    readFileSync(
      new URL(`../web/src/i18n/locales/${language}.json`, import.meta.url),
      "utf8",
    ),
  ).videogen;
  const module = { exports: {} };
  vm.runInNewContext(compiled, {
    exports: module.exports,
    module,
    require: (name) => {
      if (name === "@/lib/api") return { api: {} };
      if (name === "@/i18n/config") {
        return {
          __esModule: true,
          default: {
            t: (key, { field }) => {
              const template = strings[key.replace("videogen.", "")];
              assert.equal(
                typeof template,
                "string",
                `Missing ${language} translation: ${key}`,
              );
              return template.replace("{{field}}", field);
            },
          },
        };
      }
      throw new Error(`Unexpected dependency: ${name}`);
    },
  });
  const { jsonObject } = module.exports;
  const fields = [
    strings.defaultParams,
    strings.testParams,
    strings.retryParams,
  ];

  test(`${language}: optional blank fields do not block a valid mode parameter`, () => {
    const mode = '{\n  "extra_body": {\n    "mode": "text"\n  }\n}';
    for (const field of fields) {
      for (const input of ["", " \n\t ", "{}"]) {
        assert.equal(JSON.stringify(jsonObject(input, field)), "{}");
      }
      assert.equal(jsonObject(mode, field).extra_body.mode, "text");
    }
  });

  test(`${language}: malformed JSON identifies the field and blocks submission`, () => {
    for (const field of fields) {
      for (const input of [
        '{"extra_body":',
        '{"extra_body":{"mode":"text"}',
        '{"seconds":4,}',
        "{'size':'720P'}",
      ]) {
        assert.throws(
          () => jsonObject(input, field),
          (error) =>
            error.message ===
            strings.invalidParamsJson.replace("{{field}}", field),
        );
      }
    }
  });

  test(`${language}: arrays and scalar values cannot become tool parameters`, () => {
    for (const field of fields) {
      for (const input of ["[]", "null", "true", "0", '"text"']) {
        assert.throws(
          () => jsonObject(input, field),
          (error) =>
            error.message ===
            strings.invalidParamsObject.replace("{{field}}", field),
        );
      }
    }
  });

  test(`${language}: nested gateway parameters retain their values and types`, () => {
    const input = {
      seconds: 4,
      metadata: { seed: 0, audio: false },
      extra_body: { mode: "text", tags: ["a", "b"] },
    };
    assert.equal(
      JSON.stringify(jsonObject(JSON.stringify(input), fields[1])),
      JSON.stringify(input),
    );
  });
}
