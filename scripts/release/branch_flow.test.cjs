"use strict";

const { test } = require("node:test");
const assert = require("node:assert/strict");
const { evaluate } = require("./branch_flow.cjs");

test("Work branches can target only preview", () => {
  for (const head of ["feat/order", "fix/socket", "docs/release"]) {
    for (const base of ["preview", "rc", "main", "feat/other"]) {
      assert.equal(evaluate({ head, base, sameRepository: true }).allowed, base === "preview");
    }
  }
});

test("Internal channel promotion advances one stage at a time", () => {
  const expected = { preview: "rc", rc: "main", main: null };
  for (const [head, target] of Object.entries(expected)) {
    for (const base of ["preview", "rc", "main", "feat/other"]) {
      assert.equal(evaluate({ head, base, sameRepository: true }).allowed, base === target);
    }
  }
});

test("Fork channel names do not grant promotion privileges", () => {
  for (const head of ["preview", "rc", "main", "feat/order"]) {
    for (const base of ["preview", "rc", "main"]) {
      assert.equal(evaluate({ head, base, sameRepository: false }).allowed, base === "preview");
    }
  }
});

test("Required statuses are isolated by target branch", () => {
  const contexts = ["preview", "rc", "main"].map(base => evaluate({ head: "feat/order", base, sameRepository: true }).context);
  assert.equal(new Set(contexts).size, 3);
});

test("Dependency metadata promotion branches target only their channel", () => {
  for (const [head, target] of [["promote/rc/v0.7.0-rc.1", "rc"], ["promote/stable/v0.7.0", "main"]]) {
    for (const base of ["preview", "rc", "main"]) {
      assert.equal(evaluate({ head, base, sameRepository: true }).allowed, base === target);
      assert.equal(evaluate({ head, base, sameRepository: false }).allowed, base === "preview");
    }
  }
  for (const head of ["promote/rc/v0.7.0", "promote/stable/v0.7.0-rc.1"]) {
    assert.equal(evaluate({ head, base: "main", sameRepository: true }).allowed, false);
    assert.equal(evaluate({ head, base: "rc", sameRepository: true }).allowed, false);
  }
});
