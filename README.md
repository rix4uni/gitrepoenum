## gitxpose

🔍 Discover GitHub repositories and hunt for leaked credentials with style

## Features

✨ **Comprehensive GitHub Scanning**
- 📦 Download all repositories from organizations, users, or members
- 🔐 Automatically scan for leaked credentials using TruffleHog
- 🎯 Filter repositories by creation, update, or push dates
- 🚫 Exclude forked repositories
- 🔔 Send verified secret alerts to Discord

🎨 **Beautiful Terminal Output**
- Colorized and formatted output
- Progress tracking with counters
- Clean visual separators
- Easy-to-read repository listings

⚡ **Performance**
- Parallel repository cloning
- Configurable request delays
- Efficient credential detection

## Prerequisites

Before installing gitxpose, ensure you have **TruffleHog** installed:

```yaml
git clone https://github.com/trufflesecurity/trufflehog.git
cd trufflehog
go install
```

## Installation

### Option 1: Install using Go
```
go install github.com/rix4uni/gitxpose@latest
```

### Option 2: Download prebuilt binaries
```
wget https://github.com/rix4uni/gitxpose/releases/download/v0.0.4/gitxpose-linux-amd64-0.0.4.tgz
tar -xvzf gitxpose-linux-amd64-0.0.4.tgz
rm -rf gitxpose-linux-amd64-0.0.4.tgz
mv gitxpose ~/go/bin/gitxpose
```

Or download [binary release](https://github.com/rix4uni/gitxpose/releases) for your platform.

### Option 3: Compile from source
```
git clone --depth 1 https://github.com/rix4uni/gitxpose.git
cd gitxpose; go install
```

## Configuration

### GitHub Token Setup

Create a configuration directory and add your GitHub tokens:

```yaml
mkdir -p ~/.config/gitxpose
echo "your_github_token_here" > ~/.config/gitxpose/github-token.txt
```

You can add multiple tokens (one per line) for better rate limiting:

```yaml
echo "token1" >> ~/.config/gitxpose/github-token.txt
echo "token2" >> ~/.config/gitxpose/github-token.txt
```

### Discord Notifications (Optional)

To receive verified secret alerts via Discord, configure notify:

```yaml
mkdir -p ~/.config/notify
```

Create `~/.config/notify/provider-config.yaml`:

```yaml
discord:
  - id: "allvuln"
    discord_webhook_url: "https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"
```

## Usage

```yaml
Usage of gitxpose:
  -created string
        Filter repos created within duration (e.g., 1h, 7d, 1m, 1y)
  -date string
        Fetch commits from repositories (e.g., 50s, 40m, 5h, 1d, 2w, 3M, 1y, all) (default "all")
  -delay string
        Delay duration between requests (default "-1ns")
  -id string
        Send verified vulnerabilities to Discord
  -no-fork
        Exclude forked repositories
  -output string
        Directory to save the output (default "/root/.gitxpose/")
  -parallel int
        Number of repositories to clone in parallel (default 10)
  -pushed string
        Filter repos pushed within duration
  -scan-repo string
        Type of scan: org, member, or user (required)
  -token string
        Path to the file containing GitHub tokens (default "/root/.config/gitxpose/github-token.txt")
  -updated string
        Filter repos updated within duration
```

## Examples

### Basic Usage

**Scan a user's repositories:**
```yaml
echo "username" | gitxpose --scan-repo user
```

**Scan an organization:**
```yaml
echo "orgname" | gitxpose --scan-repo org
```

**Get organization members:**
```yaml
echo "orgname" | gitxpose --scan-repo member
```

### Advanced Usage

**Exclude forked repositories:**
```yaml
echo "username" | gitxpose --scan-repo user --no-fork
```

**Filter by update date (repos updated in last 30 days):**
```yaml
echo "username" | gitxpose --scan-repo user --updated 30d
```

**Scan with Discord notifications:**
```yaml
echo "username" | gitxpose --scan-repo user --id allvuln
```

**Scan specific time period commits:**
```yaml
echo "username" | gitxpose --scan-repo user --date 7d
```

**Custom parallel downloads:**
```yaml
echo "username" | gitxpose --scan-repo user --parallel 20
```

**Combine multiple filters:**
```yaml
echo "username" | gitxpose --scan-repo user --no-fork --updated 30d --date 7d --id allvuln
```

### Time Duration Formats

- **Seconds:** `50s`
- **Minutes:** `40m`
- **Hours:** `5h`
- **Days:** `7d`
- **Weeks:** `2w`
- **Months:** `3M`
- **Years:** `1y`
- **All:** `all` (default)

## Output Structure

```yaml
~/.gitxpose/
└── username/
    ├── fetchrepo.json          # Repository metadata
    ├── repo1/
    │   ├── commits.txt         # List of commit hashes
    │   ├── code/              # Commit contents
    │   │   ├── hash1.txt
    │   │   └── hash2.txt
    │   └── vuln/
    │       └── trufflehog.json # Vulnerability scan results
    └── repo2/
        └── ...
```

## Output Example

```yaml
╭──────────────────────────────────────────────────────────────────────────────╮
│ REPOSITORIES FOR USERNAME                                                    │
╰──────────────────────────────────────────────────────────────────────────────╯

👤 User: https://github.com/username

📦 Found 6 repositories:

  1. gitxpose
     🔗 https://github.com/username/gitxpose.git

  2. project2
     🔗 https://github.com/username/project2.git

────────────────────────────────────────────────────────────────────────────────

╭──────────────────────────────────────────────────────────────────────────────╮
│ CLONING REPOSITORIES                                                         │
╰──────────────────────────────────────────────────────────────────────────────╯

✓ Cloned [1/6]: gitxpose
✓ Cloned [2/6]: project2

└─ Successfully cloned 6 repositories

╭──────────────────────────────────────────────────────────────────────────────╮
│ ANALYZING REPOSITORIES                                                       │
╰──────────────────────────────────────────────────────────────────────────────╯

[1/6] Processing: gitxpose
────────────────────────────────────────────────────────────────────────────────
  📝 Fetching commits: gitxpose
  📦 Fetching code: gitxpose
  🔍 Scanning: gitxpose
  🔔 Notified: Verified secret sent to Discord
✓ Completed: gitxpose

🎉 All operations completed successfully!
```
