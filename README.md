## gitxpose

🔍 Discover GitHub repositories and hunt for leaked credentials with style

## Features

✨ **Comprehensive GitHub Scanning**
- 📦 Download all repositories from organizations, users, or members
- 🔐 Automatically scan for leaked credentials using TruffleHog
- 🎯 Filter repositories by creation, update, or push dates
- 🚫 Exclude forked repositories
- 🔔 Send verified secret alerts to Discord
- 🔄 Secret deduplication (prevents duplicate notifications for the same secret)
- 💾 Track detected secrets in `~/.config/gitxpose/detected-secrets.txt`

🎨 **Beautiful Terminal Output**
- Colorized and formatted output
- Progress tracking with counters
- Clean visual separators
- Easy-to-read repository listings

⚡ **Performance**
- **Parallel repository cloning** with auto-scaling based on system resources
- **Parallel API page fetching** for faster repository discovery
- **Parallel repository analysis** (commits, code extraction, vulnerability scanning)
- **Parallel commit processing** within each repository
- **Auto-detection of system resources** (CPU cores, RAM) for optimal performance
- **Configurable parallelism** at multiple levels (API, analysis, commits)
- Configurable request delays
- Efficient credential detection
- Secret deduplication to prevent duplicate notifications
- **Expected speedup:** 4-8x faster for large organizations (200+ repos)

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
wget https://github.com/rix4uni/gitxpose/releases/download/v0.0.5/gitxpose-linux-amd64-0.0.5.tgz
tar -xvzf gitxpose-linux-amd64-0.0.5.tgz
rm -rf gitxpose-linux-amd64-0.0.5.tgz
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

**Note:** Secrets are automatically deduplicated. If the same secret is detected multiple times, only the first detection will trigger a Discord notification. All detected secrets are tracked in `~/.config/gitxpose/detected-secrets.txt`.

## Usage

```yaml
Usage of gitxpose:
  -analysis-parallel int
        Parallelism for repository analysis (0 = auto-detect based on system resources)
  -api-parallel int
        Parallelism for API requests (default: 1, 0 = auto-detect / 2)
  -auto-scale
        Enable automatic scaling based on system resources (default: true)
  -commit-parallel int
        Parallelism for commit processing (0 = auto-detect / 2)
  -created string
        Filter repos created within duration (e.g., 1h, 7d, 1m, 1y)
  -date string
        Fetch commits from repositories (e.g., 50s, 40m, 5h, 1d, 2w, 3M, 1y, all) (default "all")
  -delay string
        Delay duration between requests (default "-1ns")
  -id string
        Send verified vulnerabilities to Discord
  -max-parallel int
        Maximum parallelism (0 = auto-detect based on system resources)
  -no-fork
        Exclude forked repositories
  -output string
        Directory or file to save the output (default: "~/.gitxpose/")
        If directory doesn't exist, it will be created automatically
  -parallel int
        Number of repositories to clone in parallel (default: 10, 0 = auto-detect)
  -pushed string
        Filter repos pushed within duration
  -scan-repo string
        Type of scan: org, member, or user (required)
  -silent
        Silent mode (suppress banner)
  -token string
        Path to the file containing GitHub tokens (default: "~/.config/gitxpose/github-token.txt")
  -updated string
        Filter repos updated within duration
  -version
        Print the version of the tool and exit
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

**Custom output directory:**
```yaml
echo "username" | gitxpose --scan-repo user --output my-results/
```

**Auto-scaling performance (uses all CPU cores):**
```yaml
echo "username" | gitxpose --scan-repo user --auto-scale
```

**Manual parallelism control:**
```yaml
echo "username" | gitxpose --scan-repo user --max-parallel 16 --api-parallel 4 --analysis-parallel 8 --commit-parallel 4
```

**Disable auto-scaling and use fixed parallelism:**
```yaml
echo "username" | gitxpose --scan-repo user --auto-scale=false --parallel 5
```

**Silent mode (no banner):**
```yaml
echo "username" | gitxpose --scan-repo user --silent
```

**Combine multiple filters:**
```yaml
echo "username" | gitxpose --scan-repo user --no-fork --updated 30d --date 7d --id allvuln --output results/
```

### Performance Tuning

**Auto-scaling (Recommended):**
By default, gitxpose automatically detects your system's CPU cores and scales parallelism accordingly. This is optimal for most use cases:

```yaml
echo "username" | gitxpose --scan-repo user --auto-scale
```

**Manual Control:**
For fine-grained control, you can set parallelism at different levels:

```yaml
# Limit maximum parallelism
echo "username" | gitxpose --scan-repo user --max-parallel 8

# Control specific operations
echo "username" | gitxpose --scan-repo user \
  --api-parallel 2 \
  --analysis-parallel 4 \
  --commit-parallel 2 \
  --parallel 4
```

**Disable Auto-scaling:**
To use fixed parallelism values:

```yaml
echo "username" | gitxpose --scan-repo user --auto-scale=false --parallel 5
```

**Performance Tips:**
- For large organizations (100+ repos), enable auto-scaling for best performance
- Use multiple GitHub tokens for better rate limiting
- Increase `--analysis-parallel` for CPU-bound systems
- Increase `--api-parallel` for faster repository discovery (be mindful of rate limits)

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

### Default Output (no -output flag)
```yaml
~/.gitxpose/
└── username/
    ├── username_repo.json      # Repository metadata
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

### Custom Output Directory (with -output flag)
```yaml
your-output-dir/
└── username/
    ├── username_repo.json      # Repository metadata
    ├── repo1/
    │   ├── commits.txt
    │   ├── code/
    │   └── vuln/
    └── repo2/
        └── ...
```

**Note:** When using `-output`, all files (JSON, cloned repos, code, commits, vuln scans) are saved to the specified directory. If the directory doesn't exist, it will be created automatically.

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
