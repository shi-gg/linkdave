import { execSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const CONVENTIONAL_COMMIT_REGEX = /^(feat|perf|fix|refactor|docs|build|types|chore|examples|test|style|ci)(?:\(([^()]+)\))?!?: (.+)$/i;
const STRIP_GIT_REGEX = /\.git$/;
const PR_REGEX = /\(#(\d+)\)$/;
const SPACE_PR_REGEX = /\s*\(#\d+\)$/;

const PACKAGE_PATH = join(import.meta.dirname, "..", "client", "package.json");
const packageJson = JSON.parse(readFileSync(PACKAGE_PATH, "utf-8"));
const VERSION = packageJson.version;
const TAG = `v${VERSION}`;

function run(cmd: string) {
    try {
        return execSync(cmd, { encoding: "utf-8", stdio: ["pipe", "pipe", "ignore"] }).trim();
    } catch {
        return "";
    }
}

const REMOTE_URL = run("git config --get remote.origin.url").replace(STRIP_GIT_REGEX, "");
const PREV_TAG = run("git describe --tags --abbrev=0");

const log = run(
    PREV_TAG
        ? `git log ${PREV_TAG}..HEAD --pretty=format:"%H|%h|%s"`
        : "git log --pretty=format:\"%H|%h|%s\""
);

const commits = log
    .split("\n")
    .filter(Boolean);

const categories: Record<string, { title: string; items: string[]; }> = {
    feat: { title: "🚀 Features", items: [] },
    perf: { title: "🔥 Performance", items: [] },
    fix: { title: "🩹 Fixes", items: [] },
    refactor: { title: "💅 Refactors", items: [] },
    docs: { title: "📖 Documentation", items: [] },
    build: { title: "📦 Build", items: [] },
    chore: { title: "🏡 Chore", items: [] },
    test: { title: "✅ Tests", items: [] },
    style: { title: "🎨 Styles", items: [] },
    ci: { title: "🤖 CI", items: [] },
    other: { title: "🛸 Other", items: [] }
};

for (const line of commits) {
    const [fullHash, shortHash, ...subjectParts] = line.split("|");
    const subject = subjectParts.join("|");

    const prNumber = subject.match(PR_REGEX)?.[1];
    const msg = prNumber ? subject.replace(SPACE_PR_REGEX, "") : subject;
    const prLink = prNumber
        ? ` ([#${prNumber}](${REMOTE_URL}/pull/${prNumber}))`
        : ` ([${shortHash}](${REMOTE_URL}/commit/${fullHash}))`;

    const parts = msg.match(CONVENTIONAL_COMMIT_REGEX);

    if (!parts) {
        const description = msg.charAt(0).toUpperCase() + msg.slice(1);
        categories.other.items.push(`- ${description}${prLink}`);

        continue;
    }

    const type = parts[1].toLowerCase();
    const scope = parts[2];
    const description = parts[3]
        .charAt(0)
        .toUpperCase()
        + parts[3].slice(1);

    const formattedMsg = scope
        ? `- **${categories[type] ? "" : `${type}: `}${scope}:** ${description}${prLink}`
        : `- ${categories[type] ? "" : `${type}: `}${description}${prLink}`;

    if (categories[type]) categories[type].items.push(formattedMsg);
    else categories.other.items.push(formattedMsg);

}

let changelog = "";
if (PREV_TAG) changelog += `[compare changes](${REMOTE_URL}/compare/${PREV_TAG}...${TAG})\n\n`;
else changelog += `[compare changes](${REMOTE_URL}/commits/${TAG})\n\n`;

for (const key of Object.keys(categories)) {
    const category = categories[key];
    if (category.items.length > 0) {
        changelog += `### ${category.title}\n\n`;
        changelog += category.items.join("\n") + "\n\n";
    }
}

const finalChangelog = changelog.trim() + "\n";
writeFileSync("CHANGELOG.md", finalChangelog);

console.log(finalChangelog);