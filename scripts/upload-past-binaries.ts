import { execSync } from "node:child_process";

function run(cmd: string) {
    try {
        return execSync(cmd, { encoding: "utf-8", stdio: ["pipe", "pipe", "ignore"] }).trim();
    } catch {
        return "";
    }
}

const tags = run("git tag --sort=v:refname").split("\n").filter(Boolean);

if (tags.length === 0) {
    console.log("No tags found.");
    process.exit(0);
}

for (const tag of tags) {
    console.log(`\n========================================`);
    console.log(`Processing tag ${tag}...`);

    try {
        console.log(`Building binary for ${tag} via Docker...`);
        // Build using the git URL directly so we don't mess up our local git tree
        execSync(`docker build --target builder --build-arg BUILD_VERSION=${tag.replace(/^v/, '')} -t linkdave-builder:${tag} .`, { stdio: "inherit" });
        
        try {
            execSync(`docker rm -f extract-${tag}`, { stdio: "ignore" });
        } catch {} // Ignore error if it doesn't exist
        
        // Extract the binary
        execSync(`docker create --name extract-${tag} linkdave-builder:${tag}`, { stdio: "ignore" });
        execSync(`docker cp extract-${tag}:/app/linkdave ./linkdave-linux-amd64`, { stdio: "ignore" });
        execSync(`docker rm extract-${tag}`, { stdio: "ignore" });
        
        // Make the binary executable
        execSync(`chmod +x ./linkdave-linux-amd64`, { stdio: "ignore" });

        console.log(`Uploading binary for ${tag}...`);
        
        // Upload the binary (clobber overwrites the existing one)
        execSync(`gh release upload ${tag} ./linkdave-linux-amd64 --clobber`, { stdio: "inherit" });
        
        // Cleanup the old unarchived binary from the release
        try {
            execSync(`gh release delete-asset ${tag} linkdave-linux-amd64.tar.gz -y`, { stdio: "ignore" });
        } catch {}

        execSync(`rm -f linkdave-linux-amd64`, { stdio: "ignore" });

        console.log(`Successfully attached bundled binary to ${tag} release.`);
    } catch (error: any) {
        console.error(`Failed to process tag ${tag}. Error: ${error.message}`);
    }
}

console.log("Done!");
