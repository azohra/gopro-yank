import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { test } from "node:test";

const supported = ["darwin", "linux"].includes(process.platform)
  && ["arm64", "x64"].includes(process.arch)
  && !(process.platform === "darwin" && process.arch === "x64");

for (const corrupt of [false, true]) {
  test(corrupt ? "rejects a corrupt download without replacing an installed binary" : "installs a release and runs its executable", { skip: !supported }, (t) => {
    const root = mkdtempSync(join(tmpdir(), "gopro-install-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const payload = join(root, "payload");
    const release = join(root, "release");
    const destination = join(root, "bin");
    for (const directory of [payload, release, destination]) mkdirSync(directory);
    const binary = "#!/bin/sh\nprintf 'installed\\n'\n";
    writeFileSync(join(payload, "gopro-yank"), binary, { mode: 0o755 });
    const arch = process.arch === "x64" ? "amd64" : "arm64";
    const asset = `gopro-yank_${process.platform}_${arch}.tar.gz`;
    execFileSync("tar", ["-czf", join(release, asset), "-C", payload, "gopro-yank"]);
    const digest = createHash("sha256").update(readFileSync(join(release, asset))).digest("hex");
    writeFileSync(join(release, "checksums.txt"), `${corrupt ? "0".repeat(64) : digest}  ${asset}\n`);
    const installed = join(destination, "gopro-yank");
    if (corrupt) writeFileSync(installed, "previous installation");
    const result = spawnSync("sh", [resolve("public/install.sh")], {
      env: { ...process.env, GOPRO_YANK_RELEASE_URL: `file://${release}`, GOPRO_YANK_INSTALL_DIR: destination, GOPRO_YANK_NO_PATH_UPDATE: "1" },
      encoding: "utf8",
    });
    if (corrupt) {
      assert.notEqual(result.status, 0);
      assert.equal(readFileSync(installed, "utf8"), "previous installation");
    } else {
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.equal(execFileSync(installed, { encoding: "utf8" }), "installed\n");
    }
  });
}
