import { execSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const PACKAGE_PATH = join(import.meta.dirname, "..", "client", "package.json");
const packageJson = JSON.parse(readFileSync(PACKAGE_PATH, "utf-8"));

const VERSION = packageJson.version;
const PACKAGE_NAME = packageJson.name;

let publishedVersion = "0.0.0";
try {
    publishedVersion = execSync(`npm show ${PACKAGE_NAME} version`, {
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "ignore"]
    }).trim();
} catch { }

console.log(`Current version:  ${VERSION}`);
console.log(`Published version: ${publishedVersion}`);

const shouldPublish = VERSION !== publishedVersion;

if (shouldPublish) {
    console.log("New version available: " + VERSION);
} else {
    console.log("No new version.");
}

const githubEnv = process.env.GITHUB_ENV;
if (githubEnv) {
    writeFileSync(githubEnv, `version=${VERSION}\npublish=${shouldPublish}\n`, { flag: "a" });
}