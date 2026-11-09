"use strict";

// fork의 main 등은 작업 브랜치로 취급하고, 저장소 내부 채널 이름만 예약합니다.
function evaluate({ head, base, sameRepository }) {
  let allowed = false;
  const promotion = /^promote\/(rc|stable)\/v\d+\.\d+\.\d+(-rc\.[1-9]\d*)?$/.exec(head);
  if (sameRepository && promotion) allowed = (promotion[1] === "rc") === Boolean(promotion[2]) && base === (promotion[1] === "stable" ? "main" : "rc");
  else if (sameRepository && head === "preview") allowed = base === "rc";
  else if (sameRepository && head === "rc") allowed = base === "main";
  else if (!(sameRepository && head === "main")) allowed = base === "preview";
  return {
    allowed,
    context: `Branch Flow / ${base}`,
    description: allowed
      ? `${head} → ${base} is allowed`
      : "Only work → preview → rc → main transitions are allowed",
  };
}

module.exports = { evaluate };

if (require.main === module) {
  if (process.env.GITHUB_EVENT_NAME === "pull_request") {
    const event = JSON.parse(require("node:fs").readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"));
    const pr = event.pull_request;
    const result = evaluate({
      head: pr.head.ref,
      base: pr.base.ref,
      sameRepository: pr.head.repo?.full_name === pr.base.repo.full_name,
    });
    console.log(result.description);
    if (!result.allowed) process.exitCode = 1;
  } else {
    console.log("Validating a non-PR CI event");
  }
}
