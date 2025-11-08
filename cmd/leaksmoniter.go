package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config and other types remain the same
type Config struct {
	TYPE        string `yaml:"TYPE"`
	SORT        string `yaml:"SORT"`
	DIRECTION   string `yaml:"DIRECTION"`
	PER_PAGE    int    `yaml:"PER_PAGE"`
	PAGE        int    `yaml:"PAGE"`
	Private     string `yaml:"Private"`
	HTMLURL     string `yaml:"HTMLURL"`
	Description string `yaml:"Description"`
	Fork        string `yaml:"Fork"`
	CreatedAt   string `yaml:"CreatedAt"`
	UpdatedAt   string `yaml:"UpdatedAt"`
	PushedAt    string `yaml:"PushedAt"`
	GitURL      string `yaml:"GitURL"`
	SSHURL      string `yaml:"SSHURL"`
	CloneURL    string `yaml:"CloneURL"`
	SVNURL      string `yaml:"SVNURL"`
	Size        string `yaml:"Size"`
	Language    string `yaml:"Language"`
}

// Colors for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
)

var (
	tokenFile   string
	delay       string
	scanType    string
	dateRange   string
	downloadDir string
	parallel    int
	depth       int
	notifyID    string
)

// leaksmoniterCmd represents the leaksmoniter command
var leaksmoniterCmd = &cobra.Command{
	Use:   "leaksmoniter",
	Short: "Monitor GitHub repositories for leaks and vulnerabilities",
	Long: `A comprehensive tool to monitor GitHub organizations, users, and members
for potential leaks and vulnerabilities using trufflehog scanning.

Features:
- Fetch repositories from organizations, users, and their members
- Clone repositories with configurable depth and parallelism
- Extract commits and code changes
- Scan for vulnerabilities using trufflehog
- Send notifications to Discord

Examples:
  # Complete automated workflow including vulnerability scanning
  echo "Shopify" | gitrepoenum leaksmoniter --scan-repo org --date 24h

  # Scan individual user repositories
  echo "rix4uni" | gitrepoenum leaksmoniter --scan-repo user

  # Scan both org and member repositories
  cat orgnames.txt | gitrepoenum leaksmoniter --scan-repo org,member

  # With Discord notifications for vulnerabilities
  cat orgnames.txt | gitrepoenum leaksmoniter --scan-repo org,member --notifyid allvuln

  # With custom base directory
  cat orgnames.txt | gitrepoenum leaksmoniter --scan-repo org --download-dir ~/myrepos

  # High parallelism for faster cloning
  cat orgnames.txt | gitrepoenum leaksmoniter --parallel 20 --depth 10

  # Scan recent repositories only (last 7 days)
  echo "google" | gitrepoenum leaksmoniter --scan-repo org --date 7d

  # Comprehensive scan with all options
  echo "microsoft" | gitrepoenum leaksmoniter --scan-repo org,member,user --date 30d --parallel 15 --notifyid my-webhook`,
	Run: func(cmd *cobra.Command, args []string) {
		executeLeaksMonitor()
	},
}

func loadConfig(filePath string) (*Config, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func loadTokens(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tokens []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		token := scanner.Text()
		if token != "" {
			tokens = append(tokens, token)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

func getRandomToken(tokens []string) string {
	rand.Seed(time.Now().UnixNano())
	return tokens[rand.Intn(len(tokens))]
}

func parseDuration(durationStr string) (time.Duration, error) {
	if durationStr == "all" {
		return 0, nil
	}

	re := regexp.MustCompile(`^(\d+)([smhdwMy])$`)
	matches := re.FindStringSubmatch(durationStr)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration format: %s", durationStr)
	}

	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	var duration time.Duration

	switch unit {
	case "s":
		duration = time.Duration(num) * time.Second
	case "m":
		duration = time.Duration(num) * time.Minute
	case "h":
		duration = time.Duration(num) * time.Hour
	case "d":
		duration = time.Duration(num) * 24 * time.Hour
	case "w":
		duration = time.Duration(num) * 7 * 24 * time.Hour
	case "M":
		duration = time.Duration(num) * 30 * 24 * time.Hour
	case "y":
		duration = time.Duration(num) * 365 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown time unit: %s", unit)
	}

	return duration, nil
}

func filterReposByDate(repos []map[string]interface{}, dateRange string) ([]map[string]interface{}, error) {
	if dateRange == "all" {
		return repos, nil
	}

	duration, err := parseDuration(dateRange)
	if err != nil {
		return nil, err
	}

	cutoffTime := time.Now().Add(-duration)
	var filteredRepos []map[string]interface{}

	for _, repo := range repos {
		updatedAtStr, ok := repo["updated_at"].(string)
		if !ok || updatedAtStr == "" {
			continue
		}

		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			updatedAt, err = time.Parse("2006-01-02T15:04:05Z", updatedAtStr)
			if err != nil {
				continue
			}
		}

		if updatedAt.After(cutoffTime) {
			filteredRepos = append(filteredRepos, repo)
		}
	}

	return filteredRepos, nil
}

func cloneRepositories(repoURLs []string, downloadDir string, parallelCount int, depth int) {
	sem := make(chan struct{}, parallelCount)
	var wg sync.WaitGroup

	for _, url := range repoURLs {
		wg.Add(1)

		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			parts := strings.Split(url, "/")
			if len(parts) < 2 {
				return
			}

			username := parts[len(parts)-2]
			reponame := strings.TrimSuffix(parts[len(parts)-1], ".git")
			dirName := username + "-" + reponame

			if downloadDir != "" {
				dirName = filepath.Join(downloadDir, dirName)
			}

			if _, err := os.Stat(dirName); !os.IsNotExist(err) {
				os.RemoveAll(dirName)
			}

			cloneArgs := []string{"clone", url, dirName}
			if depth > 0 {
				cloneArgs = append(cloneArgs, "--depth", fmt.Sprintf("%d", depth))
			}
			cloneCmd := exec.Command("git", cloneArgs...)

			var stderr bytes.Buffer
			cloneCmd.Stderr = &stderr

			if err := cloneCmd.Run(); err != nil {
				fmt.Printf("%s✗%s Failed: %s\n", ColorRed, ColorReset, url)
			} else {
				fmt.Printf("%s✓%s Cloned: %s\n", ColorGreen, ColorReset, url)
			}
		}(url)
	}

	wg.Wait()
}

func fetchUserRepos(username string, config *Config, tokens []string, delay time.Duration, dateRange string) ([]string, error) {
	var allCloneURLs []string
	page := 1

	for {
		token := getRandomToken(tokens)
		apiURL := fmt.Sprintf("https://api.github.com/users/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
			username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "token "+token)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch user repos: %s", resp.Status)
		}

		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, err
		}

		if len(repos) == 0 {
			break
		}

		if dateRange != "all" {
			filteredRepos, err := filterReposByDate(repos, dateRange)
			if err != nil {
				return nil, fmt.Errorf("error filtering user repos by date: %v", err)
			}
			repos = filteredRepos
		}

		for _, repo := range repos {
			if cloneURL, ok := repo["clone_url"].(string); ok && cloneURL != "" {
				allCloneURLs = append(allCloneURLs, cloneURL)
			}
		}

		page++
		time.Sleep(delay)
	}

	return allCloneURLs, nil
}

func extractCommits(baseDir string, dateRange string) {
	downloadDir := filepath.Join(baseDir, "download")
	commitsDir := filepath.Join(baseDir, "commits")

	fmt.Printf("\n%s═══════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s                  STEP 3: EXTRACTING COMMITS%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s═══════════════════════════════════════════════════%s\n\n", ColorCyan, ColorReset)

	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		return
	}

	totalRepos := 0
	for _, entry := range entries {
		if entry.IsDir() {
			repoPath := filepath.Join(downloadDir, entry.Name())
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				totalRepos++
			}
		}
	}

	if totalRepos == 0 {
		fmt.Printf("%s!%s No Git repositories found\n", ColorYellow, ColorReset)
		return
	}

	currentRepo := 0
	for _, entry := range entries {
		if entry.IsDir() {
			repoPath := filepath.Join(downloadDir, entry.Name())
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				currentRepo++

				outputRepoDir := filepath.Join(commitsDir, entry.Name())
				os.MkdirAll(outputRepoDir, os.ModePerm)
				outputFile := filepath.Join(outputRepoDir, "commits.txt")

				args := []string{"-C", repoPath, "--no-pager", "log", "--pretty=format:%H"}

				if dateRange != "all" {
					re := regexp.MustCompile(`([0-9]+)([smhdwMy])`)
					matches := re.FindStringSubmatch(dateRange)
					if len(matches) == 3 {
						dateNum := matches[1]
						dateUnit := matches[2]
						var gitTime string
						switch dateUnit {
						case "s":
							gitTime = fmt.Sprintf("%s seconds", dateNum)
						case "m":
							gitTime = fmt.Sprintf("%s minutes", dateNum)
						case "h":
							gitTime = fmt.Sprintf("%s hours", dateNum)
						case "d":
							gitTime = fmt.Sprintf("%s days", dateNum)
						case "w":
							gitTime = fmt.Sprintf("%s weeks", dateNum)
						case "M":
							gitTime = fmt.Sprintf("%s months", dateNum)
						case "y":
							gitTime = fmt.Sprintf("%s years", dateNum)
						}
						if gitTime != "" {
							args = append(args, "--before="+gitTime)
						}
					}
				}

				cmd := exec.Command("git", args...)
				output, err := cmd.Output()
				if err != nil {
					continue
				}

				os.WriteFile(outputFile, output, 0644)

				commits := strings.Split(strings.TrimSpace(string(output)), "\n")
				commitCount := len(commits)
				if commitCount == 1 && commits[0] == "" {
					commitCount = 0
				}

				fmt.Printf("%s[%d/%d]%s %s → %d commits\n", ColorBlue, currentRepo, totalRepos, ColorReset, entry.Name(), commitCount)
			}
		}
	}

	fmt.Printf("\n%s✓%s Commit extraction completed\n", ColorGreen, ColorReset)
}

func extractCode(baseDir string) {
	downloadDir := filepath.Join(baseDir, "download")
	commitsDir := filepath.Join(baseDir, "commits")
	codeDir := filepath.Join(baseDir, "code")

	fmt.Printf("\n%s═══════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s               STEP 4: EXTRACTING CODE%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s═══════════════════════════════════════════════════%s\n\n", ColorCyan, ColorReset)

	entries, err := os.ReadDir(commitsDir)
	if err != nil {
		return
	}

	totalRepos := 0
	for _, entry := range entries {
		if entry.IsDir() {
			commitsFile := filepath.Join(commitsDir, entry.Name(), "commits.txt")
			if _, err := os.Stat(commitsFile); err == nil {
				totalRepos++
			}
		}
	}

	if totalRepos == 0 {
		fmt.Printf("%s!%s No commit files found\n", ColorYellow, ColorReset)
		return
	}

	currentRepo := 0
	for _, entry := range entries {
		if entry.IsDir() {
			repoName := entry.Name()
			commitsFile := filepath.Join(commitsDir, repoName, "commits.txt")

			if _, err := os.Stat(commitsFile); os.IsNotExist(err) {
				continue
			}

			currentRepo++

			content, err := os.ReadFile(commitsFile)
			if err != nil {
				continue
			}

			commits := strings.Split(string(content), "\n")
			repoCodeDir := filepath.Join(codeDir, repoName, "code")
			os.MkdirAll(repoCodeDir, os.ModePerm)

			validCommits := 0
			for _, commit := range commits {
				if strings.TrimSpace(commit) != "" {
					validCommits++
				}
			}

			processedCommits := 0
			for _, commit := range commits {
				if strings.TrimSpace(commit) == "" {
					continue
				}

				processedCommits++
				outputFilePath := filepath.Join(repoCodeDir, commit+".txt")
				repoPath := filepath.Join(downloadDir, repoName)

				cmd := exec.Command("git", "-C", repoPath, "--no-pager", "show", commit)
				output, err := cmd.Output()
				if err != nil {
					continue
				}

				os.WriteFile(outputFilePath, output, 0644)
			}

			fmt.Printf("%s[%d/%d]%s %s → %d files\n", ColorBlue, currentRepo, totalRepos, ColorReset, repoName, processedCommits)
		}
	}

	fmt.Printf("\n%s✓%s Code extraction completed\n", ColorGreen, ColorReset)
}

func scanVulnerabilities(baseDir string, notifyID string) {
	codeDir := filepath.Join(baseDir, "code")
	vulnDir := filepath.Join(baseDir, "vuln")

	fmt.Printf("\n%s═══════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s            STEP 5: SCANNING VULNERABILITIES%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s═══════════════════════════════════════════════════%s\n\n", ColorCyan, ColorReset)

	entries, err := os.ReadDir(codeDir)
	if err != nil {
		return
	}

	totalRepos := 0
	for _, entry := range entries {
		if entry.IsDir() {
			codeSubDir := filepath.Join(codeDir, entry.Name(), "code")
			if _, err := os.Stat(codeSubDir); err == nil {
				totalRepos++
			}
		}
	}

	if totalRepos == 0 {
		fmt.Printf("%s!%s No code directories found\n", ColorYellow, ColorReset)
		return
	}

	currentRepo := 0
	for _, entry := range entries {
		if entry.IsDir() {
			repoName := entry.Name()
			codePath := filepath.Join(codeDir, repoName, "code")

			if _, err := os.Stat(codePath); os.IsNotExist(err) {
				continue
			}

			currentRepo++

			vulnOutputPath := filepath.Join(vulnDir, repoName, "vuln")
			os.MkdirAll(vulnOutputPath, os.ModePerm)

			trufflehogOutputFile := filepath.Join(vulnOutputPath, "trufflehog.json")
			fmt.Printf("%s[%d/%d]%s Scanning: %s\n", ColorBlue, currentRepo, totalRepos, ColorReset, repoName)

			trufflehogCmd := exec.Command("trufflehog", "filesystem", "--json", codePath)

			outputFile, err := os.Create(trufflehogOutputFile)
			if err != nil {
				continue
			}

			trufflehogCmd.Stdout = outputFile
			trufflehogCmd.Stderr = outputFile
			trufflehogCmd.Run()
			outputFile.Close()

			// Send formatted vulnerabilities to Discord
			if notifyID != "" {
				// Extract repo URL from repoName
				parts := strings.Split(repoName, "-")
				var repoURL string
				if len(parts) >= 2 {
					username := parts[0]
					reponame := strings.Join(parts[1:], "-")
					repoURL = fmt.Sprintf("https://github.com/%s/%s", username, reponame)
				} else {
					repoURL = repoName
				}

				// Create properly formatted vulnerability output
				cmd := exec.Command(
					"bash", "-c",
					fmt.Sprintf(`cat %s | jq -r --arg repo "%s" --arg vulnFile "%s" 'select(.Verified==true) | "Repo: \($repo), VulnDir: \($vulnFile), Leaks: \(.DetectorName): \(.Raw)"'`, trufflehogOutputFile, repoURL, trufflehogOutputFile),
				)

				output, err := cmd.Output()
				if err == nil && len(output) > 0 {
					// Group vulnerabilities by unique combinations to avoid duplicates
					uniqueVulns := make(map[string]bool)
					var formattedOutput []string

					lines := strings.Split(string(output), "\n")
					for _, line := range lines {
						if line != "" && !uniqueVulns[line] {
							uniqueVulns[line] = true
							formattedOutput = append(formattedOutput, line)
						}
					}

					if len(formattedOutput) > 0 {
						// Send each vulnerability line directly to notify without temp file
						for _, vulnLine := range formattedOutput {
							notifyCmd := exec.Command(
								"bash", "-c",
								fmt.Sprintf(`echo "%s" | notify -duc -silent -id %s &>/dev/null`, vulnLine, notifyID),
							)
							notifyCmd.Run()
						}

						fmt.Printf("%s✓%s Sent %d vulnerabilities to Discord\n", ColorGreen, ColorReset, len(formattedOutput))
					} else {
						fmt.Printf("%s!%s No verified vulnerabilities\n", ColorYellow, ColorReset)
					}
				} else {
					fmt.Printf("%s!%s No verified vulnerabilities\n", ColorYellow, ColorReset)
				}
			}
		}
	}

	fmt.Printf("\n%s✓%s Vulnerability scanning completed\n", ColorGreen, ColorReset)
}

func fetchData(username string, config *Config, tokens []string, delay time.Duration, scanType string, dateRange string) ([]byte, []string, error) {
	var allData []map[string]interface{}
	var cloneURLs []string
	page := config.PAGE

	for {
		token := getRandomToken(tokens)
		var apiURL string
		if scanType == "member" {
			apiURL = fmt.Sprintf("https://api.github.com/orgs/%s/members?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
				username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)
		} else if scanType == "user" {
			apiURL = fmt.Sprintf("https://api.github.com/users/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
				username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)
		} else {
			apiURL = fmt.Sprintf("https://api.github.com/orgs/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
				username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)
		}

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, nil, err
		}

		req.Header.Set("Authorization", "token "+token)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, nil, fmt.Errorf("failed to fetch data: %s", resp.Status)
		}

		var data []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, nil, err
		}

		if len(data) == 0 {
			break
		}

		if (scanType == "org" || scanType == "user") && dateRange != "all" {
			filteredData, err := filterReposByDate(data, dateRange)
			if err != nil {
				return nil, nil, fmt.Errorf("error filtering repos by date: %v", err)
			}
			data = filteredData
		}

		allData = append(allData, data...)

		if scanType == "org" || scanType == "user" {
			for _, repo := range data {
				if cloneURL, ok := repo["clone_url"].(string); ok && cloneURL != "" {
					cloneURLs = append(cloneURLs, cloneURL)
				}
			}
		}

		page++
		time.Sleep(delay)
	}

	output, err := json.MarshalIndent(allData, "", "  ")
	if err != nil {
		return nil, nil, err
	}

	return output, cloneURLs, nil
}

func executeLeaksMonitor() {
	scanTypes := strings.Split(scanType, ",")
	validScanTypes := make([]string, 0)

	for _, st := range scanTypes {
		st = strings.TrimSpace(st)
		if st == "org" || st == "member" || st == "user" {
			validScanTypes = append(validScanTypes, st)
		}
	}

	if len(validScanTypes) == 0 {
		fmt.Printf("%s✗%s Invalid scan type: %s\n", ColorRed, ColorReset, scanType)
		return
	}

	// Load config
	configPath := "$HOME/.config/gitrepoenum/config.yaml"
	configPathExpanded := os.ExpandEnv(configPath)
	config, err := loadConfig(configPathExpanded)
	if err != nil {
		fmt.Printf("%s✗%s Config error: %v\n", ColorRed, ColorReset, err)
		return
	}

	// Load tokens
	tokenFileExpanded := os.ExpandEnv(tokenFile)
	tokens, err := loadTokens(tokenFileExpanded)
	if err != nil || len(tokens) == 0 {
		fmt.Printf("%s✗%s Token error\n", ColorRed, ColorReset)
		return
	}

	delayDuration, _ := time.ParseDuration(delay)

	scanner := bufio.NewScanner(os.Stdin)
	var allCloneURLs []string

	for scanner.Scan() {
		username := scanner.Text()

		for _, currentScanType := range validScanTypes {
			fmt.Printf("\n%s═══════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
			fmt.Printf("%s                  STEP 1: FETCHING DATA%s\n", ColorCyan, ColorReset)
			fmt.Printf("%s═══════════════════════════════════════════════════%s\n\n", ColorCyan, ColorReset)

			output, cloneURLs, err := fetchData(username, config, tokens, delayDuration, currentScanType, dateRange)
			if err != nil {
				fmt.Printf("%s✗%s Fetch error: %v\n", ColorRed, ColorReset, err)
				continue
			}

			if currentScanType == "org" || currentScanType == "user" {
				allCloneURLs = append(allCloneURLs, cloneURLs...)
				fmt.Printf("%s✓%s Found %d repositories\n", ColorGreen, ColorReset, len(cloneURLs))
			}

			if currentScanType == "member" {
				var members []map[string]interface{}
				if err := json.Unmarshal(output, &members); err == nil {
					memberReposCount := 0
					for _, member := range members {
						if memberLogin, ok := member["login"].(string); ok {
							memberCloneURLs, err := fetchUserRepos(memberLogin, config, tokens, delayDuration, dateRange)
							if err == nil {
								allCloneURLs = append(allCloneURLs, memberCloneURLs...)
								memberReposCount += len(memberCloneURLs)
							}
						}
					}
					fmt.Printf("%s✓%s Found %d member repositories\n", ColorGreen, ColorReset, memberReposCount)
				}
			}

			// Save output
			var outputDir string
			if downloadDir != "" {
				outputDir = downloadDir
			} else {
				homeDir, _ := os.UserHomeDir()
				outputDir = filepath.Join(homeDir, ".gitrepoenum")
			}
			os.MkdirAll(outputDir, 0755)

			var outputPath string
			if dateRange != "all" {
				outputPath = filepath.Join(outputDir, fmt.Sprintf("%s-%s-%s.json", username, currentScanType, dateRange))
			} else {
				outputPath = filepath.Join(outputDir, fmt.Sprintf("%s-%s.json", username, currentScanType))
			}

			if err := os.WriteFile(outputPath, output, 0644); err == nil {
				fmt.Printf("%s✓%s Output saved to %s\n", ColorGreen, ColorReset, outputPath)
			}
		}
	}

	if len(allCloneURLs) == 0 {
		fmt.Printf("\n%s!%s No repositories to clone\n", ColorYellow, ColorReset)
		return
	}

	uniqueURLs := make(map[string]bool)
	var uniqueCloneURLs []string
	for _, url := range allCloneURLs {
		if !uniqueURLs[url] {
			uniqueURLs[url] = true
			uniqueCloneURLs = append(uniqueCloneURLs, url)
		}
	}

	fmt.Printf("\n%s═══════════════════════════════════════════════════%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s                 STEP 2: CLONING%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s═══════════════════════════════════════════════════%s\n\n", ColorCyan, ColorReset)

	fmt.Printf("%s→%s Cloning %d repositories\n\n", ColorBlue, ColorReset, len(uniqueCloneURLs))

	if downloadDir == "" {
		homeDir, _ := os.UserHomeDir()
		downloadDir = filepath.Join(homeDir, ".gitrepoenum")
	}

	finalDownloadDir := filepath.Join(downloadDir, "download")
	os.MkdirAll(finalDownloadDir, 0755)

	cloneRepositories(uniqueCloneURLs, finalDownloadDir, parallel, depth)

	extractCommits(downloadDir, dateRange)
	extractCode(downloadDir)
	scanVulnerabilities(downloadDir, notifyID)

	if err := scanner.Err(); err != nil {
		fmt.Printf("%s✗%s Input error: %v\n", ColorRed, ColorReset, err)
	}
}

func init() {
	rootCmd.AddCommand(leaksmoniterCmd)

	// Define flags
	leaksmoniterCmd.Flags().StringVarP(&tokenFile, "token", "t", "$HOME/.config/gitrepoenum/github-token.txt", "GitHub tokens file, 1 token per line")
	leaksmoniterCmd.Flags().StringVarP(&delay, "delay", "d", "-1ns", "Delay between requests (e.g., 1ns, 1us, 1ms, 1s, 1m)")
	leaksmoniterCmd.Flags().StringVarP(&scanType, "scan-repo", "s", "org,member", "Scan type: org, member, user")
	leaksmoniterCmd.Flags().StringVarP(&dateRange, "date", "D", "24h", "Specify the date range for repositories (e.g., 50s, 40m, 5h, 1d, 2w, 3M, 1y, all)")
	leaksmoniterCmd.Flags().StringVarP(&downloadDir, "download-dir", "o", "", "Base directory for downloads, commits, code, and vulnerabilities")
	leaksmoniterCmd.Flags().IntVarP(&parallel, "parallel", "p", 10, "Repositories to clone in parallel")
	leaksmoniterCmd.Flags().IntVarP(&depth, "depth", "z", 5, "Git clone depth")
	leaksmoniterCmd.Flags().StringVarP(&notifyID, "notifyid", "n", "allvuln", "Send verified vulnerabilities to Discord")
}
