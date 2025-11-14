package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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

	"github.com/rix4uni/gitxpose/banner"
)

// Color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
)

type cleanRepo struct {
	HTMLURL  string `json:"html_url,omitempty"`
	CloneURL string `json:"clone_url,omitempty"`
}

type TruffleHogResult struct {
	SourceMetadata struct {
		Data struct {
			Filesystem struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"Filesystem"`
		} `json:"Data"`
	} `json:"SourceMetadata"`
	DetectorName        string `json:"DetectorName"`
	DetectorDescription string `json:"DetectorDescription"`
	Verified            bool   `json:"Verified"`
	Raw                 string `json:"Raw"`
}

type DiscordWebhook struct {
	Content string `json:"content"`
}

func getRandomToken(tokens []string) string {
	rand.Seed(time.Now().UnixNano())
	return tokens[rand.Intn(len(tokens))]
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

func buildAPIURL(scanType, username string, page int) string {
	repoType := "owner"
	sort := "full_name"
	direction := "asc"
	perPage := 30

	switch scanType {
	case "org":
		return fmt.Sprintf("https://api.github.com/orgs/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
			username, repoType, sort, direction, perPage, page)
	case "member":
		return fmt.Sprintf("https://api.github.com/orgs/%s/members?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
			username, repoType, sort, direction, perPage, page)
	case "user":
		return fmt.Sprintf("https://api.github.com/users/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d",
			username, repoType, sort, direction, perPage, page)
	default:
		return ""
	}
}

func parseDuration(duration string) (time.Duration, error) {
	if duration == "" {
		return 0, nil
	}

	duration = strings.TrimSpace(duration)
	if len(duration) < 2 {
		return 0, fmt.Errorf("invalid duration format")
	}

	value, err := strconv.Atoi(duration[:len(duration)-1])
	if err != nil {
		return 0, err
	}

	unit := duration[len(duration)-1:]
	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "m":
		return time.Duration(value) * 30 * 24 * time.Hour, nil
	case "y":
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit: %s (use h, d, m, or y)", unit)
	}
}

func filterRepos(repos []map[string]interface{}, createdDuration, updatedDuration, pushedDuration time.Duration, noFork bool) []map[string]interface{} {
	filtered := []map[string]interface{}{}
	now := time.Now()

	for _, repo := range repos {
		if noFork {
			if fork, ok := repo["fork"].(bool); ok && fork {
				continue
			}
		}

		if createdDuration > 0 {
			if createdAt, ok := repo["created_at"].(string); ok {
				t, err := time.Parse(time.RFC3339, createdAt)
				if err == nil && now.Sub(t) > createdDuration {
					continue
				}
			}
		}

		if updatedDuration > 0 {
			if updatedAt, ok := repo["updated_at"].(string); ok {
				t, err := time.Parse(time.RFC3339, updatedAt)
				if err == nil && now.Sub(t) > updatedDuration {
					continue
				}
			}
		}

		if pushedDuration > 0 {
			if pushedAt, ok := repo["pushed_at"].(string); ok {
				t, err := time.Parse(time.RFC3339, pushedAt)
				if err == nil && now.Sub(t) > pushedDuration {
					continue
				}
			}
		}

		filtered = append(filtered, repo)
	}

	return filtered
}

func fetchRepos(scanType, username string, tokens []string, delay time.Duration, createdFilter, updatedFilter, pushedFilter string, noFork bool) ([]byte, error) {
	var allRepos []map[string]interface{}
	page := 1

	for {
		token := getRandomToken(tokens)
		apiURL := buildAPIURL(scanType, username, page)

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
			return nil, fmt.Errorf("%sfailed to fetch repos: %s%s", ColorRed, resp.Status, ColorReset)
		}

		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, err
		}

		if len(repos) == 0 {
			break
		}

		allRepos = append(allRepos, repos...)
		page++
		time.Sleep(delay)
	}

	createdDuration, _ := parseDuration(createdFilter)
	updatedDuration, _ := parseDuration(updatedFilter)
	pushedDuration, _ := parseDuration(pushedFilter)

	filteredRepos := filterRepos(allRepos, createdDuration, updatedDuration, pushedDuration, noFork)

	return printCleanOutput(username, filteredRepos)
}

func printCleanOutput(username string, repos []map[string]interface{}) ([]byte, error) {
	output := map[string]interface{}{
		"user":  fmt.Sprintf("https://github.com/%s", username),
		"repos": make([]cleanRepo, len(repos)),
	}

	for i, repo := range repos {
		repoItem := cleanRepo{
			HTMLURL:  getString(repo, "html_url"),
			CloneURL: getString(repo, "clone_url"),
		}
		output["repos"].([]cleanRepo)[i] = repoItem
	}

	reposList := output["repos"].([]cleanRepo)

	jsonOutput, err := json.MarshalIndent(map[string]interface{}{
		"user":  output["user"],
		"repos": reposList,
	}, "", "  ")
	if err != nil {
		return nil, err
	}

	return jsonOutput, nil
}

func printBeautifulRepoOutput(username string, repos []map[string]interface{}) {
	userURL := fmt.Sprintf("https://github.com/%s", username)

	printHeader(fmt.Sprintf("REPOSITORIES FOR %s", strings.ToUpper(username)))

	fmt.Printf("%s👤 User:%s %s%s%s\n\n", ColorCyan, ColorReset, ColorBold, userURL, ColorReset)

	if len(repos) == 0 {
		fmt.Printf("%s⚠ No repositories found%s\n", ColorYellow, ColorReset)
		return
	}

	fmt.Printf("%s📦 Found %d repositories:%s\n\n", ColorGreen, len(repos), ColorReset)

	for i, repo := range repos {
		cloneURL := getString(repo, "clone_url")
		repoName := getString(repo, "name")
		if repoName == "" && cloneURL != "" {
			parts := strings.Split(cloneURL, "/")
			if len(parts) > 0 {
				repoName = strings.TrimSuffix(parts[len(parts)-1], ".git")
			}
		}

		fmt.Printf("  %s%d.%s %s%s%s\n", ColorDim, i+1, ColorReset, ColorBold, repoName, ColorReset)
		fmt.Printf("     %s🔗%s %s\n", ColorBlue, ColorReset, cloneURL)

		if i < len(repos)-1 {
			fmt.Println()
		}
	}

	printSeparator()
}

func getString(repo map[string]interface{}, key string) string {
	if value, ok := repo[key]; ok && value != nil {
		return value.(string)
	}
	return ""
}

func fetchCommitContent(repoPath string, commitHash string, outputFilePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "--no-pager", "show", commitHash)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%serror fetching commit %s: %v%s", ColorRed, commitHash, err, ColorReset)
	}

	err = ioutil.WriteFile(outputFilePath, output, 0644)
	if err != nil {
		return fmt.Errorf("%serror writing commit to file: %v%s", ColorRed, err, ColorReset)
	}

	return nil
}

func processRepoCommits(repoPath string) error {
	commitsFile := filepath.Join(repoPath, "commits.txt")

	if _, err := os.Stat(commitsFile); os.IsNotExist(err) {
		return fmt.Errorf("%scommits.txt not found in %s%s", ColorRed, repoPath, ColorReset)
	}

	content, err := ioutil.ReadFile(commitsFile)
	if err != nil {
		return fmt.Errorf("error reading commits file: %v", err)
	}

	commits := strings.Split(string(content), "\n")
	codeDir := filepath.Join(repoPath, "code")
	if err := os.MkdirAll(codeDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating code directory: %v", err)
	}

	for _, commit := range commits {
		if strings.TrimSpace(commit) == "" {
			continue
		}

		outputFilePath := filepath.Join(codeDir, commit+".txt")
		if err := fetchCommitContent(repoPath, commit, outputFilePath); err != nil {
			return err
		}
	}

	return nil
}

func sendDiscordNotification(webhookURL, message string) error {
	webhook := DiscordWebhook{
		Content: message,
	}

	jsonData, err := json.Marshal(webhook)
	if err != nil {
		return err
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%sdiscord webhook returned status: %d%s", ColorRed, resp.StatusCode, ColorReset)
	}

	return nil
}

func getDiscordWebhookURL(notifyID string) (string, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "notify", "provider-config.yaml")
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("%sfailed to read notify config: %v%s", ColorRed, err, ColorReset)
	}

	lines := strings.Split(string(content), "\n")
	var foundID bool

	for i, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "- id:") {
			idValue := strings.TrimSpace(strings.TrimPrefix(line, "- id:"))
			idValue = strings.Trim(idValue, "\"'")

			if idValue == notifyID {
				foundID = true
				for j := i + 1; j < len(lines) && j < i+10; j++ {
					nextLine := strings.TrimSpace(lines[j])

					if strings.HasPrefix(nextLine, "- id:") {
						break
					}

					if strings.HasPrefix(nextLine, "discord_webhook_url:") {
						webhookURL := strings.TrimSpace(strings.TrimPrefix(nextLine, "discord_webhook_url:"))
						webhookURL = strings.Trim(webhookURL, "\"'")
						return webhookURL, nil
					}
				}
			}
		}
	}

	if foundID {
		return "", fmt.Errorf("%swebhook URL not found for ID: %s (ID found but webhook_url missing)%s", ColorRed, notifyID, ColorReset)
	}
	return "", fmt.Errorf("%sID not found in config: %s%s", ColorRed, notifyID, ColorReset)
}

func printSeparator() {
	fmt.Printf("%s%s%s\n", ColorDim, strings.Repeat("─", 80), ColorReset)
}

func printHeader(title string) {
	fmt.Printf("\n%s╭%s╮%s\n", ColorCyan, strings.Repeat("─", 78), ColorReset)
	fmt.Printf("%s│%s %s%-76s%s %s│%s\n", ColorCyan, ColorReset, ColorBold, title, ColorReset, ColorCyan, ColorReset)
	fmt.Printf("%s╰%s╯%s\n\n", ColorCyan, strings.Repeat("─", 78), ColorReset)
}

func printSubHeader(title string) {
	fmt.Printf("\n%s┌─ %s%s%s\n", ColorBlue, ColorBold, title, ColorReset)
}

func printFooter(message string) {
	fmt.Printf("\n%s└─ %s%s%s\n\n", ColorGreen, ColorBold, message, ColorReset)
}

func scanRepoForVulnerabilities(repoPath string, notifyID string) error {
	vulnDir := filepath.Join(repoPath, "vuln")
	if err := os.MkdirAll(vulnDir, os.ModePerm); err != nil {
		return fmt.Errorf("%serror creating vuln directory: %v%s", ColorRed, err, ColorReset)
	}

	trufflehogOutputFile := filepath.Join(vulnDir, "trufflehog.json")
	repoName := filepath.Base(repoPath)

	fmt.Printf("  %s🔍 Scanning:%s %s%-50s%s\n", ColorYellow, ColorReset, ColorDim, repoName, ColorReset)

	trufflehogCmd := exec.Command("trufflehog", "filesystem", "--json", repoPath)

	outputFile, err := os.Create(trufflehogOutputFile)
	if err != nil {
		return fmt.Errorf("error creating trufflehog output file: %v", err)
	}
	defer outputFile.Close()

	trufflehogCmd.Stdout = outputFile
	trufflehogCmd.Stderr = outputFile

	if err := trufflehogCmd.Run(); err != nil {
		return fmt.Errorf("%sfailed to run trufflehog: %v%s", ColorRed, err, ColorReset)
	}

	if notifyID != "" {
		webhookURL, err := getDiscordWebhookURL(notifyID)
		if err != nil {
			fmt.Printf("  %s✗ Error:%s %v\n", ColorRed, ColorReset, err)
			return nil
		}

		repoName := filepath.Base(repoPath)
		username := filepath.Base(filepath.Dir(repoPath))

		content, err := ioutil.ReadFile(trufflehogOutputFile)
		if err != nil {
			return fmt.Errorf("%serror reading trufflehog output: %v%s", ColorRed, err, ColorReset)
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			var result TruffleHogResult
			if err := json.Unmarshal([]byte(line), &result); err != nil {
				continue
			}

			if result.Verified {
				message := fmt.Sprintf("**[VERIFIED SECRET FOUND]**\n"+
					"🔍 **Repo:** %s/%s\n"+
					"🔑 **Detector:** %s\n"+
					"📝 **Description:** %s\n"+
					"📄 **File:** %s\n"+
					"📍 **Line:** %d\n"+
					"🔐 **Secret:** `%s\n\n\n`",
					username, repoName,
					result.DetectorName,
					result.DetectorDescription,
					result.SourceMetadata.Data.Filesystem.File,
					result.SourceMetadata.Data.Filesystem.Line,
					result.Raw)

				if err := sendDiscordNotification(webhookURL, message); err != nil {
					fmt.Printf("  %s✗ Notification failed:%s %v\n", ColorRed, ColorReset, err)
				} else {
					fmt.Printf("  %s🔔 Notified:%s Verified secret sent to Discord\n", ColorGreen, ColorReset)
				}

				time.Sleep(1 * time.Second)
			}
		}
	}

	return nil
}

func convertToGitTime(dateValue string, timeUnit string) (string, error) {
	switch timeUnit {
	case "s", "second", "seconds":
		return fmt.Sprintf("%s seconds", dateValue), nil
	case "m", "minute", "minutes":
		return fmt.Sprintf("%s minutes", dateValue), nil
	case "h", "hour", "hours":
		return fmt.Sprintf("%s hours", dateValue), nil
	case "d", "day", "days":
		return fmt.Sprintf("%s days", dateValue), nil
	case "w", "week", "weeks":
		return fmt.Sprintf("%s weeks", dateValue), nil
	case "M", "month", "months":
		return fmt.Sprintf("%s months", dateValue), nil
	case "y", "year", "years":
		return fmt.Sprintf("%s years", dateValue), nil
	default:
		return "", fmt.Errorf("invalid time unit")
	}
}

func runGitLog(repoPath string, gitTime string, outputFile string) error {
	args := []string{"-C", repoPath, "--no-pager", "log", "--pretty=format:%H"}

	if gitTime != "" {
		args = append(args, "--before="+gitTime)
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%serror executing git log: %v%s", ColorRed, err, ColorReset)
	}

	return ioutil.WriteFile(outputFile, output, 0644)
}

func fetchCommitsForRepo(repoPath string, dateFlag string, vulnNotifyID string) error {
	outputFile := filepath.Join(repoPath, "commits.txt")
	repoName := filepath.Base(repoPath)

	var gitTime string
	if dateFlag != "all" && dateFlag != "" {
		re := regexp.MustCompile(`([0-9]+)([smhdwMy])`)
		matches := re.FindStringSubmatch(dateFlag)

		if len(matches) != 3 {
			return fmt.Errorf("%sinvalid date format: %s%s", ColorRed, dateFlag, ColorReset)
		}

		dateNum := matches[1]
		dateUnit := matches[2]

		var err error
		gitTime, err = convertToGitTime(dateNum, dateUnit)
		if err != nil {
			return err
		}
	}

	fmt.Printf("  %s📝 Fetching commits:%s %s\n", ColorCyan, ColorReset, repoName)
	if err := runGitLog(repoPath, gitTime, outputFile); err != nil {
		return fmt.Errorf("%serror fetching commits: %v%s", ColorRed, err, ColorReset)
	}

	fmt.Printf("  %s📦 Fetching code:%s %s\n", ColorCyan, ColorReset, repoName)
	if err := processRepoCommits(repoPath); err != nil {
		return fmt.Errorf("%serror fetching commit content: %v%s", ColorRed, err, ColorReset)
	}

	if err := scanRepoForVulnerabilities(repoPath, vulnNotifyID); err != nil {
		return fmt.Errorf("%serror scanning vulnerabilities: %v%s", ColorRed, err, ColorReset)
	}

	return nil
}

func cloneRepositories(repoURLs []string, parallel int, dateFlag string, username string, vulnNotifyID string) {
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var clonedRepos []string
	var reposMutex sync.Mutex
	var cloneCount int

	printHeader("CLONING REPOSITORIES")

	for _, url := range repoURLs {
		wg.Add(1)

		go func(url string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			parts := strings.Split(url, "/")
			if len(parts) < 2 {
				fmt.Printf("%s✗ Invalid URL:%s %s\n", ColorRed, ColorReset, url)
				return
			}

			reponame := strings.TrimSuffix(parts[len(parts)-1], ".git")
			dirName := filepath.Join(os.Getenv("HOME"), ".gitxpose", username, reponame)

			if _, err := os.Stat(dirName); !os.IsNotExist(err) {
				fmt.Printf("%s⚠ Removing existing:%s %s\n", ColorYellow, ColorReset, reponame)
				err := os.RemoveAll(dirName)
				if err != nil {
					fmt.Printf("%s✗ Failed to remove:%s %s\n", ColorRed, ColorReset, err)
					return
				}
			}

			cloneCmd := exec.Command("git", "clone", url, dirName)

			var stderr bytes.Buffer
			cloneCmd.Stderr = &stderr

			if err := cloneCmd.Run(); err != nil {
				fmt.Printf("%s✗ Clone failed:%s %s\n", ColorRed, ColorReset, reponame)
			} else {
				reposMutex.Lock()
				cloneCount++
				fmt.Printf("%s✓ Cloned [%d/%d]:%s %s\n", ColorGreen, cloneCount, len(repoURLs), ColorReset, reponame)
				clonedRepos = append(clonedRepos, dirName)
				reposMutex.Unlock()
			}
		}(url)
	}

	wg.Wait()

	if len(clonedRepos) > 0 {
		printFooter(fmt.Sprintf("Successfully cloned %d repositories", len(clonedRepos)))

		printHeader("ANALYZING REPOSITORIES")

		for i, repoPath := range clonedRepos {
			fmt.Printf("\n%s[%d/%d]%s Processing: %s%s%s\n", ColorBlue, i+1, len(clonedRepos), ColorReset, ColorBold, filepath.Base(repoPath), ColorReset)
			printSeparator()

			if err := fetchCommitsForRepo(repoPath, dateFlag, vulnNotifyID); err != nil {
				fmt.Printf("%s✗ Analysis failed:%s %v\n", ColorRed, ColorReset, err)
			} else {
				fmt.Printf("%s✓ Completed:%s %s\n", ColorGreen, ColorReset, filepath.Base(repoPath))
			}
		}

		printFooter("Analysis complete!")
	}
}

func main() {
	scanRepoType := flag.String("scan-repo", "", "Type of scan: org, member, or user (required)")
	tokenFile := flag.String("token", filepath.Join(os.Getenv("HOME"), ".config", "gitxpose", "github-token.txt"), "Path to the file containing GitHub tokens")
	delayFlag := flag.String("delay", "-1ns", "Delay duration between requests")
	outputFile := flag.String("output", filepath.Join(os.Getenv("HOME"), ".gitxpose")+"/", "Directory to save the output")
	createdFilter := flag.String("created", "", "Filter repos created within duration (e.g., 1h, 7d, 1m, 1y)")
	updatedFilter := flag.String("updated", "", "Filter repos updated within duration")
	pushedFilter := flag.String("pushed", "", "Filter repos pushed within duration")
	noForkFlag := flag.Bool("no-fork", false, "Exclude forked repositories")
	downloadParallel := flag.Int("parallel", 10, "Number of repositories to clone in parallel")
	dateFilter := flag.String("date", "all", "Fetch commits from repositories (e.g., 50s, 40m, 5h, 1d, 2w, 3M, 1y, all)")
	vulnNotifyID := flag.String("id", "", "Send verified vulnerabilities to Discord")
	silent := flag.Bool("silent", false, "Silent mode.")
	version := flag.Bool("version", false, "Print the version of the tool and exit.")
	flag.Parse()

	// Print version and exit if -version flag is provided
	if *version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	// Don't Print banner if -silnet flag is provided
	if !*silent {
		banner.PrintBanner()
	}

	if *scanRepoType == "" {
		fmt.Fprintf(os.Stderr, "%sError: --scan-repo is required%s\n", ColorRed, ColorReset)
		flag.Usage()
		os.Exit(1)
	}

	if *scanRepoType != "org" && *scanRepoType != "member" && *scanRepoType != "user" {
		fmt.Fprintf(os.Stderr, "%sInvalid --scan-repo value. Must be 'org', 'member', or 'user'%s\n", ColorRed, ColorReset)
		os.Exit(1)
	}

	tokenFileExpanded := os.ExpandEnv(*tokenFile)
	tokens, err := loadTokens(tokenFileExpanded)
	if err != nil {
		fmt.Printf("%sError loading tokens: %v%s\n", ColorRed, err, ColorReset)
		os.Exit(1)
	}

	if len(tokens) == 0 {
		fmt.Printf("%sNo tokens found in the file%s\n", ColorRed, ColorReset)
		os.Exit(1)
	}

	delay, err := time.ParseDuration(*delayFlag)
	if err != nil {
		fmt.Printf("%sInvalid delay duration: %v%s\n", ColorRed, err, ColorReset)
		os.Exit(1)
	}

	outputFileExpanded := os.ExpandEnv(*outputFile)

	outputDir := outputFileExpanded
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("%sError creating directory: %v%s\n", ColorRed, err, ColorReset)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)

	isDirectory := strings.HasSuffix(outputFileExpanded, "/") || outputFileExpanded == filepath.Join(os.Getenv("HOME"), ".gitxpose")+"/"

	var allReposOutput []byte
	var allCloneURLs []string
	var currentUsername string

	for scanner.Scan() {
		username := scanner.Text()
		currentUsername = username

		output, err := fetchRepos(*scanRepoType, username, tokens, delay, *createdFilter, *updatedFilter, *pushedFilter, *noForkFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s✗ Error fetching repos for %s: %v%s\n", ColorRed, username, err, ColorReset)
			continue
		}

		var jsonData map[string]interface{}
		if err := json.Unmarshal(output, &jsonData); err == nil {
			// Print beautiful output to terminal
			if repos, ok := jsonData["repos"].([]interface{}); ok {
				reposMap := make([]map[string]interface{}, 0)
				for _, repo := range repos {
					if repoMap, ok := repo.(map[string]interface{}); ok {
						reposMap = append(reposMap, repoMap)
						if cloneURL, ok := repoMap["clone_url"].(string); ok && cloneURL != "" {
							allCloneURLs = append(allCloneURLs, cloneURL)
						}
					}
				}
				printBeautifulRepoOutput(username, reposMap)
			}
		}

		if isDirectory {
			userDir := filepath.Join(outputDir, username)
			if err := os.MkdirAll(userDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "%s✗ Error creating directory %s: %v%s\n", ColorRed, userDir, err, ColorReset)
				continue
			}

			outputPath := filepath.Join(userDir, "fetchrepo.json")
			if err := os.WriteFile(outputPath, output, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "%s✗ Error writing to file %s: %v%s\n", ColorRed, outputPath, err, ColorReset)
			} else {
				fmt.Printf("\n%s✓ Saved:%s %s\n", ColorGreen, ColorReset, outputPath)
			}
		} else {
			allReposOutput = append(allReposOutput, output...)
		}
	}

	if !isDirectory {
		outputPath := outputFileExpanded
		if !filepath.IsAbs(outputFileExpanded) {
			homeDir, _ := os.UserHomeDir()
			outputPath = filepath.Join(homeDir, ".gitxpose", outputFileExpanded)
		}

		if err := os.WriteFile(outputPath, allReposOutput, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "%s✗ Error writing to file %s: %v%s\n", ColorRed, outputPath, err, ColorReset)
		} else {
			fmt.Printf("%s✓ Saved:%s %s\n", ColorGreen, ColorReset, outputPath)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, ColorRed+"✗ Error reading input:"+ColorReset, err)
	}

	if len(allCloneURLs) > 0 {
		cloneRepositories(allCloneURLs, *downloadParallel, *dateFilter, currentUsername, *vulnNotifyID)

		printSeparator()
		fmt.Printf("\n%s🎉 All operations completed successfully!%s\n\n", ColorGreen+ColorBold, ColorReset)
	}
}
