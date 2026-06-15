#!/usr/bin/env node

import { cpSync, existsSync, mkdirSync, rmSync, chmodSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const rootDir = dirname(scriptDir);
const packageDir = join(rootDir, "npm");
const packageVersion = JSON.parse(readFileSync(join(packageDir, "package.json"), "utf-8")).version || "dev";

const platforms = [
  { key: "darwin-arm64", goos: "darwin", goarch: "arm64", exe: "nova" },
  { key: "darwin-x64", goos: "darwin", goarch: "amd64", exe: "nova" },
  { key: "linux-arm64", goos: "linux", goarch: "arm64", exe: "nova" },
  { key: "linux-x64", goos: "linux", goarch: "amd64", exe: "nova" },
  { key: "win32-x64", goos: "windows", goarch: "amd64", exe: "nova.exe" },
];

main();

function main() {
  console.log("==> 构建前端");
  runPackageManager(["-C", "web", "install", "--frozen-lockfile"]);
  runPackageManager(["-C", "web", "build"]);

  console.log("==> 清理 npm 发布产物");
  for (const name of ["vendor", "web", "skills"]) {
    rmSync(join(packageDir, name), { recursive: true, force: true });
  }
  for (const name of ["README.md", "CHANGELOG.md", "LICENSE", "config.toml"]) {
    rmSync(join(packageDir, name), { force: true });
  }

  console.log("==> 复制运行时资源");
  cpDir(join(rootDir, "web", "dist"), join(packageDir, "web"));
  cpDir(join(rootDir, "skills"), join(packageDir, "skills"));
  cpFileIfExists("config.toml");
  cpFileIfExists("README.md");
  cpFileIfExists("CHANGELOG.md");
  cpFileIfExists("LICENSE");

  console.log("==> 交叉编译 Nova");
  for (const target of platforms) {
    const outDir = join(packageDir, "vendor", target.key);
    mkdirSync(outDir, { recursive: true });
    const out = join(outDir, target.exe);
    run("go", ["build", "-trimpath", "-ldflags", `-s -w -X nova/internal/buildinfo.Version=${packageVersion}`, "-o", out, "./cmd/nova"], {
      ...process.env,
      CGO_ENABLED: "0",
      GOOS: target.goos,
      GOARCH: target.goarch,
    });
    if (target.goos !== "windows") {
      chmodSync(out, 0o755);
    }
  }

  chmodSync(join(packageDir, "bin", "nova.js"), 0o755);
  console.log("==> npm 发布目录已准备好: npm/");
  console.log("    预览: cd npm && npm pack --dry-run");
  console.log("    发布: cd npm && npm publish --access public");
}

function runPackageManager(args) {
  const pnpm = commandName("pnpm");
  const check = spawnSync(pnpm, ["--version"], { cwd: rootDir, stdio: "ignore" });
  if (check.status === 0) {
    run(pnpm, args);
    return;
  }
  run(commandName("npx"), ["pnpm", ...args]);
}

function run(cmd, args, env = process.env) {
  const result = spawnSync(commandName(cmd), args, {
    cwd: rootDir,
    env,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function commandName(name) {
  if (process.platform === "win32" && !name.endsWith(".cmd")) {
    return `${name}.cmd`;
  }
  return name;
}

function cpDir(from, to) {
  if (!existsSync(from)) {
    console.error(`缺少目录: ${from}`);
    process.exit(1);
  }
  cpSync(from, to, { recursive: true });
}

function cpFileIfExists(name) {
  const from = join(rootDir, name);
  if (existsSync(from)) {
    cpSync(from, join(packageDir, name));
  }
}
