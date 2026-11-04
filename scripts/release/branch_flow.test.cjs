"use strict";

const { test } = require("node:test");
const assert = require("node:assert/strict");
const { evaluate } = require("./branch_flow.cjs");

test("작업 브랜치의 허용 대상은 preview뿐입니다", () => {
  for (const head of ["feat/order", "fix/socket", "docs/release"]) {
    for (const base of ["preview", "rc", "main", "feat/other"]) {
      assert.equal(evaluate({ head, base, sameRepository: true }).allowed, base === "preview");
    }
  }
});

test("저장소 내부 채널 승격은 한 단계씩만 가능합니다", () => {
  const expected = { preview: "rc", rc: "main", main: null };
  for (const [head, target] of Object.entries(expected)) {
    for (const base of ["preview", "rc", "main", "feat/other"]) {
      assert.equal(evaluate({ head, base, sameRepository: true }).allowed, base === target);
    }
  }
});

test("fork의 채널 이름으로 승격 권한을 가장할 수 없습니다", () => {
  for (const head of ["preview", "rc", "main", "feat/order"]) {
    for (const base of ["preview", "rc", "main"]) {
      assert.equal(evaluate({ head, base, sameRepository: false }).allowed, base === "preview");
    }
  }
});

test("서로 다른 대상 브랜치의 필수 상태를 공유하지 않습니다", () => {
  const contexts = ["preview", "rc", "main"].map(base => evaluate({ head: "feat/order", base, sameRepository: true }).context);
  assert.equal(new Set(contexts).size, 3);
});

test("의존성 메타데이터 승격 브랜치는 해당 채널만 대상으로 합니다", () => {
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
