#!/usr/bin/env node

import { accessSync, constants } from "node:fs";
import { spawn } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const binaryName = process.platform === "win32" ? "nova.exe" : "nova";
const binaryPath = join(packageRoot, "vendor", platformKey(), binaryName);

try {
  accessSync(binaryPath, constants.F_OK);
} catch {
  console.error(`当前平台暂不支持或二进制缺失: ${platformKey()}`);
  console.error(`缺失文件: ${binaryPath}`);
  process.exit(1);
}

const args = process.argv.slice(2);
if (process.env.NOVA_BACKEND_PORT && !hasFlag(args, "port")) {
  args.push("--port", process.env.NOVA_BACKEND_PORT);
}

const child = spawn(binaryPath, args, {
  stdio: "inherit",
  env: {
    ...process.env,
    NOVA_DIR: process.env.NOVA_DIR || resolve(process.cwd(), ".nova"),
    NOVA_WEB_DIR: process.env.NOVA_WEB_DIR || join(packageRoot, "web"),
    NOVA_SKILLS_DIR: process.env.NOVA_SKILLS_DIR || join(packageRoot, "skills"),
  },
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});

child.on("error", (err) => {
  console.error(`启动 Nova 失败: ${err.message}`);
  process.exit(1);
});

function platformKey() {
  const arch = process.arch === "x64" ? "x64" : process.arch;
  return `${process.platform}-${arch}`;
}

function hasFlag(args, name) {
  return args.some((arg) => arg === `--${name}` || arg.startsWith(`--${name}=`));
}
