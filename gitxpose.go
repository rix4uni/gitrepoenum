package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

var (
	randSource         = rand.New(rand.NewSource(time.Now().UnixNano()))
	randMutex          sync.Mutex
	secretFileMutex    sync.Mutex
	detectedSecretsMap map[string]bool
	secretsMapLoaded   bool
)

func getRandomToken(tokens []string) string {
	randMutex.Lock()
	defer randMutex.Unlock()
	return tokens[randSource.Intn(len(tokens))]
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

// calculateOptimalParallelism calculates optimal parallelism based on system resources
func calculateOptimalParallelism(maxParallel int, autoScale bool) int {
	if !autoScale && maxParallel > 0 {
		return maxParallel
	}

	cpuCores := runtime.NumCPU()
	if cpuCores < 1 {
		cpuCores = 1
	}

	// Default to CPU cores * 2 for I/O bound operations
	optimal := cpuCores * 2

	// If maxParallel is set, use the minimum
	if maxParallel > 0 && optimal > maxParallel {
		optimal = maxParallel
	}

	// Minimum of 1, maximum reasonable limit
	if optimal < 1 {
		optimal = 1
	}
	if optimal > 100 {
		optimal = 100
	}

	return optimal
}

// fetchReposParallel fetches GitHub API pages in parallel
func fetchReposParallel(scanType, username string, tokens []string, apiParallel int, createdFilter, updatedFilter, pushedFilter string, noFork bool, client *http.Client) ([]byte, error) {
	type pageResult struct {
		page  int
		repos []map[string]interface{}
		err   error
	}

	// First, fetch page 1 to determine total pages
	token := getRandomToken(tokens)
	apiURL := buildAPIURL(scanType, username, 1)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Check rate limit headers
	rateLimitRemaining := resp.Header.Get("X-RateLimit-Remaining")
	rateLimitReset := resp.Header.Get("X-RateLimit-Reset")

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("%sfailed to fetch repos: %s%s", ColorRed, resp.Status, ColorReset)
	}

	var firstPageRepos []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&firstPageRepos); err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()

	if len(firstPageRepos) == 0 {
		// No repos found
		createdDuration, _ := parseDuration(createdFilter)
		updatedDuration, _ := parseDuration(updatedFilter)
		pushedDuration, _ := parseDuration(pushedFilter)
		filteredRepos := filterRepos([]map[string]interface{}{}, createdDuration, updatedDuration, pushedDuration, noFork)
		return printCleanOutput(username, filteredRepos)
	}

	// Determine if there are more pages (GitHub returns 30 per page by default)
	hasMorePages := len(firstPageRepos) == 30
	totalPages := 1
	if hasMorePages {
		// Estimate total pages - we'll fetch until we get an empty page
		totalPages = 10 // Start with reasonable estimate, will adjust
	}

	allRepos := make([]map[string]interface{}, 0, len(firstPageRepos)*totalPages)
	allRepos = append(allRepos, firstPageRepos...)

	if !hasMorePages {
		// Only one page, process and return
		createdDuration, _ := parseDuration(createdFilter)
		updatedDuration, _ := parseDuration(updatedFilter)
		pushedDuration, _ := parseDuration(pushedFilter)
		filteredRepos := filterRepos(allRepos, createdDuration, updatedDuration, pushedDuration, noFork)
		return printCleanOutput(username, filteredRepos)
	}

	// Fetch remaining pages in parallel
	pageChan := make(chan int, apiParallel*2)
	resultChan := make(chan pageResult, apiParallel*2)
	var wg sync.WaitGroup
	var reposMutex sync.Mutex

	// Worker pool for fetching pages
	for i := 0; i < apiParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range pageChan {
				token := getRandomToken(tokens)
				apiURL := buildAPIURL(scanType, username, page)
				req, err := http.NewRequest("GET", apiURL, nil)
				if err != nil {
					resultChan <- pageResult{page: page, err: err}
					continue
				}
				req.Header.Set("Authorization", "token "+token)

				resp, err := client.Do(req)
				if err != nil {
					resultChan <- pageResult{page: page, err: err}
					continue
				}

				if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					resultChan <- pageResult{page: page, err: fmt.Errorf("status: %s", resp.Status)}
					continue
				}

				var repos []map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
					resp.Body.Close()
					resultChan <- pageResult{page: page, err: err}
					continue
				}
				resp.Body.Close()

				resultChan <- pageResult{page: page, repos: repos}
			}
		}()
	}

	// Start fetching pages
	page := 2
	activePages := 0
	pageResults := make(map[int][]map[string]interface{})

	// Send initial batch of pages
	for i := 0; i < apiParallel && page <= totalPages; i++ {
		pageChan <- page
		activePages++
		page++
	}

	// Collect results and send more pages as needed
	done := false
	for !done || activePages > 0 {
		result := <-resultChan
		activePages--
		if result.err != nil {
			// On error, continue with other pages
			continue
		}

		if len(result.repos) == 0 {
			// Empty page means we're done
			done = true
			continue
		}

		reposMutex.Lock()
		pageResults[result.page] = result.repos
		reposMutex.Unlock()

		// If we got a full page, there might be more
		if len(result.repos) == 30 && !done {
			if page <= totalPages+5 { // Allow some buffer
				pageChan <- page
				activePages++
				page++
			}
		}
	}

	close(pageChan)
	wg.Wait()
	close(resultChan)

	// Collect all repos in order
	for p := 2; p < page; p++ {
		if repos, ok := pageResults[p]; ok {
			allRepos = append(allRepos, repos...)
		}
	}

	// Parse duration filters with proper error handling
	createdDuration, err := parseDuration(createdFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing created filter: %v%s", ColorRed, err, ColorReset)
	}
	updatedDuration, err := parseDuration(updatedFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing updated filter: %v%s", ColorRed, err, ColorReset)
	}
	pushedDuration, err := parseDuration(pushedFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing pushed filter: %v%s", ColorRed, err, ColorReset)
	}

	filteredRepos := filterRepos(allRepos, createdDuration, updatedDuration, pushedDuration, noFork)

	// Use rate limit info if available (for future adaptive rate limiting)
	_ = rateLimitRemaining
	_ = rateLimitReset

	return printCleanOutput(username, filteredRepos)
}

func fetchRepos(scanType, username string, tokens []string, delay time.Duration, createdFilter, updatedFilter, pushedFilter string, noFork bool, apiParallel int, client *http.Client) ([]byte, error) {
	// Use parallel fetching if apiParallel > 1, otherwise use sequential
	if apiParallel > 1 {
		return fetchReposParallel(scanType, username, tokens, apiParallel, createdFilter, updatedFilter, pushedFilter, noFork, client)
	}

	// Fallback to sequential for backward compatibility
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

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("%sfailed to fetch repos: %s%s", ColorRed, resp.Status, ColorReset)
		}

		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(repos) == 0 {
			break
		}

		allRepos = append(allRepos, repos...)
		page++
		if delay > 0 {
			time.Sleep(delay)
		}
	}

	createdDuration, err := parseDuration(createdFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing created filter: %v%s", ColorRed, err, ColorReset)
	}
	updatedDuration, err := parseDuration(updatedFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing updated filter: %v%s", ColorRed, err, ColorReset)
	}
	pushedDuration, err := parseDuration(pushedFilter)
	if err != nil {
		return nil, fmt.Errorf("%serror parsing pushed filter: %v%s", ColorRed, err, ColorReset)
	}

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

	err = os.WriteFile(outputFilePath, output, 0644)
	if err != nil {
		return fmt.Errorf("%serror writing commit to file: %v%s", ColorRed, err, ColorReset)
	}

	return nil
}

func processRepoCommits(repoPath string, commitParallel int) error {
	commitsFile := filepath.Join(repoPath, "commits.txt")

	if _, err := os.Stat(commitsFile); os.IsNotExist(err) {
		return fmt.Errorf("%scommits.txt not found in %s%s", ColorRed, repoPath, ColorReset)
	}

	content, err := os.ReadFile(commitsFile)
	if err != nil {
		return fmt.Errorf("error reading commits file: %v", err)
	}

	commits := strings.Split(string(content), "\n")
	codeDir := filepath.Join(repoPath, "code")
	if err := os.MkdirAll(codeDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating code directory: %v", err)
	}

	// Filter out empty commits
	validCommits := make([]string, 0, len(commits))
	for _, commit := range commits {
		if strings.TrimSpace(commit) != "" {
			validCommits = append(validCommits, strings.TrimSpace(commit))
		}
	}

	if len(validCommits) == 0 {
		return nil
	}

	// Process commits in parallel if commitParallel > 1
	if commitParallel > 1 && len(validCommits) > 1 {
		return processCommitsParallel(repoPath, validCommits, commitParallel, codeDir)
	}

	// Sequential processing
	for _, commit := range validCommits {
		outputFilePath := filepath.Join(codeDir, commit+".txt")
		if err := fetchCommitContent(repoPath, commit, outputFilePath); err != nil {
			return err
		}
	}

	return nil
}

func processCommitsParallel(repoPath string, commits []string, commitParallel int, codeDir string) error {
	type commitResult struct {
		commit string
		err    error
	}

	commitChan := make(chan string, len(commits))
	resultChan := make(chan commitResult, len(commits))
	var wg sync.WaitGroup

	// Worker pool
	for i := 0; i < commitParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for commit := range commitChan {
				outputFilePath := filepath.Join(codeDir, commit+".txt")
				err := fetchCommitContent(repoPath, commit, outputFilePath)
				resultChan <- commitResult{commit: commit, err: err}
			}
		}()
	}

	// Send commits to workers
	for _, commit := range commits {
		commitChan <- commit
	}
	close(commitChan)

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Check for errors
	for result := range resultChan {
		if result.err != nil {
			return result.err
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

	// Drain response body
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%sdiscord webhook returned status: %d%s", ColorRed, resp.StatusCode, ColorReset)
	}

	return nil
}

// loadDetectedSecrets loads detected secrets from file into a map for O(1) lookup
func loadDetectedSecrets(filePath string) (map[string]bool, error) {
	secretsMap := make(map[string]bool)

	// If file doesn't exist, return empty map (no secrets detected yet)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return secretsMap, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		// If read fails, return empty map (assume no secrets detected yet)
		return secretsMap, nil
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		secret := strings.TrimSpace(line)
		if secret != "" {
			secretsMap[secret] = true
		}
	}

	return secretsMap, nil
}

// saveDetectedSecret appends a new secret to the detected secrets file (thread-safe)
func saveDetectedSecret(filePath string, secret string) error {
	secretFileMutex.Lock()
	defer secretFileMutex.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create secrets directory: %v", err)
	}

	// Open file in append mode, create if it doesn't exist
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open secrets file: %v", err)
	}
	defer file.Close()

	// Write secret with newline
	_, err = file.WriteString(secret + "\n")
	if err != nil {
		return fmt.Errorf("failed to write secret to file: %v", err)
	}

	return nil
}

// isSecretAlreadyDetected checks if a secret already exists in the map
func isSecretAlreadyDetected(secret string, secretsMap map[string]bool) bool {
	return secretsMap[secret]
}

// getDetectedSecretsFilePath returns the path to the detected secrets file
func getDetectedSecretsFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}
	return filepath.Join(homeDir, ".config", "gitxpose", "detected-secrets.txt"), nil
}

// ensureSecretsMapLoaded loads the secrets map if not already loaded (thread-safe)
func ensureSecretsMapLoaded() error {
	secretFileMutex.Lock()
	defer secretFileMutex.Unlock()

	if secretsMapLoaded {
		return nil
	}

	secretsFilePath, err := getDetectedSecretsFilePath()
	if err != nil {
		// If we can't get the path, start with empty map
		detectedSecretsMap = make(map[string]bool)
		secretsMapLoaded = true
		return nil
	}

	detectedSecretsMap, err = loadDetectedSecrets(secretsFilePath)
	if err != nil {
		// If load fails, start with empty map
		detectedSecretsMap = make(map[string]bool)
	}
	secretsMapLoaded = true

	return nil
}

func getDiscordWebhookURL(notifyID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%sfailed to get home directory: %v%s", ColorRed, err, ColorReset)
	}
	configPath := filepath.Join(homeDir, ".config", "notify", "provider-config.yaml")
	content, err := os.ReadFile(configPath)
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
		// Load detected secrets map if not already loaded
		if err := ensureSecretsMapLoaded(); err != nil {
			// Log error but continue - don't block scanning
			fmt.Printf("  %s⚠ Warning:%s Failed to load detected secrets: %v\n", ColorYellow, ColorReset, err)
		}

		webhookURL, err := getDiscordWebhookURL(notifyID)
		if err != nil {
			fmt.Printf("  %s✗ Error:%s %v\n", ColorRed, ColorReset, err)
			return nil
		}

		repoName := filepath.Base(repoPath)
		username := filepath.Base(filepath.Dir(repoPath))

		content, err := os.ReadFile(trufflehogOutputFile)
		if err != nil {
			return fmt.Errorf("%serror reading trufflehog output: %v%s", ColorRed, err, ColorReset)
		}

		secretsFilePath, err := getDetectedSecretsFilePath()
		if err != nil {
			// Log error but continue - don't block scanning
			fmt.Printf("  %s⚠ Warning:%s Failed to get secrets file path: %v\n", ColorYellow, ColorReset, err)
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
				secret := strings.TrimSpace(result.Raw)
				if secret == "" {
					continue
				}

				// Check if secret already detected and mark as detected atomically (thread-safe)
				secretFileMutex.Lock()
				alreadyDetected := isSecretAlreadyDetected(secret, detectedSecretsMap)
				if !alreadyDetected {
					// Mark as detected immediately to prevent duplicate processing
					detectedSecretsMap[secret] = true
				}
				secretFileMutex.Unlock()

				if alreadyDetected {
					// Secret already detected, skip notification
					continue
				}

				// New secret detected - send notification and save
				message := fmt.Sprintf("**[VERIFIED SECRET FOUND]**\n"+
					"🔍 **Repo:** %s/%s\n"+
					"🔑 **Detector:** %s\n"+
					"📝 **Description:** %s\n"+
					"📄 **File:** %s\n"+
					"📍 **Line:** %d\n"+
					"🔐 **Secret:** `%s\n`",
					username, repoName,
					result.DetectorName,
					result.DetectorDescription,
					result.SourceMetadata.Data.Filesystem.File,
					result.SourceMetadata.Data.Filesystem.Line,
					secret)

				if err := sendDiscordNotification(webhookURL, message); err != nil {
					fmt.Printf("  %s✗ Notification failed:%s %v\n", ColorRed, ColorReset, err)
					// Remove from map if notification failed (so it can be retried later)
					secretFileMutex.Lock()
					delete(detectedSecretsMap, secret)
					secretFileMutex.Unlock()
				} else {
					fmt.Printf("  %s🔔 Notified:%s Verified secret sent to Discord\n", ColorGreen, ColorReset)

					// Save secret to file (thread-safe, map already updated)
					if secretsFilePath != "" {
						if err := saveDetectedSecret(secretsFilePath, secret); err != nil {
							fmt.Printf("  %s⚠ Warning:%s Failed to save secret: %v\n", ColorYellow, ColorReset, err)
							// Note: We keep it in the map even if file save fails to avoid duplicate notifications
						}
					}
				}

				// Reduced delay for notifications - can be parallelized
				time.Sleep(500 * time.Millisecond)
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

	return os.WriteFile(outputFile, output, 0644)
}

func fetchCommitsForRepo(repoPath string, dateFlag string, vulnNotifyID string, commitParallel int) error {
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
	if err := processRepoCommits(repoPath, commitParallel); err != nil {
		return fmt.Errorf("%serror fetching commit content: %v%s", ColorRed, err, ColorReset)
	}

	if err := scanRepoForVulnerabilities(repoPath, vulnNotifyID); err != nil {
		return fmt.Errorf("%serror scanning vulnerabilities: %v%s", ColorRed, err, ColorReset)
	}

	return nil
}

func cloneRepositories(repoURLs []string, parallel int, dateFlag string, username string, vulnNotifyID string, analysisParallel int, commitParallel int, outputDir string) {
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
			dirName := filepath.Join(outputDir, username, reponame)

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

		// Parallelize repository analysis
		if analysisParallel > 1 {
			analyzeReposParallel(clonedRepos, dateFlag, vulnNotifyID, analysisParallel, commitParallel)
		} else {
			// Sequential analysis for backward compatibility
			for i, repoPath := range clonedRepos {
				fmt.Printf("\n%s[%d/%d]%s Processing: %s%s%s\n", ColorBlue, i+1, len(clonedRepos), ColorReset, ColorBold, filepath.Base(repoPath), ColorReset)
				printSeparator()

				if err := fetchCommitsForRepo(repoPath, dateFlag, vulnNotifyID, commitParallel); err != nil {
					fmt.Printf("%s✗ Analysis failed:%s %v\n", ColorRed, ColorReset, err)
				} else {
					fmt.Printf("%s✓ Completed:%s %s\n", ColorGreen, ColorReset, filepath.Base(repoPath))
				}
			}
		}

		printFooter("Analysis complete!")
	}
}

func analyzeReposParallel(clonedRepos []string, dateFlag string, vulnNotifyID string, analysisParallel int, commitParallel int) {
	type repoResult struct {
		index int
		path  string
		err   error
	}

	repoChan := make(chan repoResult, len(clonedRepos))
	var wg sync.WaitGroup
	var printMutex sync.Mutex

	// Worker pool for analyzing repositories
	for i := 0; i < analysisParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for result := range repoChan {
				repoPath := result.path
				repoName := filepath.Base(repoPath)

				printMutex.Lock()
				fmt.Printf("\n%s[%d/%d]%s Processing: %s%s%s\n", ColorBlue, result.index+1, len(clonedRepos), ColorReset, ColorBold, repoName, ColorReset)
				printSeparator()
				printMutex.Unlock()

				err := fetchCommitsForRepo(repoPath, dateFlag, vulnNotifyID, commitParallel)

				printMutex.Lock()
				if err != nil {
					fmt.Printf("%s✗ Analysis failed:%s %v\n", ColorRed, ColorReset, err)
				} else {
					fmt.Printf("%s✓ Completed:%s %s\n", ColorGreen, ColorReset, repoName)
				}
				printMutex.Unlock()
			}
		}()
	}

	// Send repos to workers
	for i, repoPath := range clonedRepos {
		repoChan <- repoResult{index: i, path: repoPath}
	}
	close(repoChan)

	wg.Wait()
}

func main() {
	scanRepoType := flag.String("scan-repo", "", "Type of scan: org, member, or user (required)")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
		if homeDir == "" {
			homeDir = "."
		}
	}

	tokenFile := flag.String("token", filepath.Join(homeDir, ".config", "gitxpose", "github-token.txt"), "Path to the file containing GitHub tokens")
	delayFlag := flag.String("delay", "-1ns", "Delay duration between requests")
	outputFile := flag.String("output", filepath.Join(homeDir, ".gitxpose")+"/", "Directory to save the output")
	createdFilter := flag.String("created", "", "Filter repos created within duration (e.g., 1h, 7d, 1m, 1y)")
	updatedFilter := flag.String("updated", "", "Filter repos updated within duration")
	pushedFilter := flag.String("pushed", "", "Filter repos pushed within duration")
	noForkFlag := flag.Bool("no-fork", false, "Exclude forked repositories")
	downloadParallel := flag.Int("parallel", 10, "Number of repositories to clone in parallel")
	maxParallel := flag.Int("max-parallel", 0, "Maximum parallelism (0 = auto-detect based on system resources)")
	autoScale := flag.Bool("auto-scale", true, "Enable automatic scaling based on system resources")
	apiParallel := flag.Int("api-parallel", 1, "Parallelism for API requests (0 = auto-detect / 2)")
	analysisParallel := flag.Int("analysis-parallel", 0, "Parallelism for repository analysis (0 = auto-detect)")
	commitParallel := flag.Int("commit-parallel", 0, "Parallelism for commit processing (0 = auto-detect / 2)")
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

	// Calculate optimal parallelism
	optimalParallel := calculateOptimalParallelism(*maxParallel, *autoScale)
	if *downloadParallel <= 0 {
		*downloadParallel = optimalParallel
	}
	if *apiParallel <= 0 {
		*apiParallel = optimalParallel / 2
		if *apiParallel < 1 {
			*apiParallel = 1
		}
	}
	if *analysisParallel <= 0 {
		*analysisParallel = optimalParallel
	}
	if *commitParallel <= 0 {
		*commitParallel = optimalParallel / 2
		if *commitParallel < 1 {
			*commitParallel = 1
		}
	}

	// Create reusable HTTP client with connection pooling
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *apiParallel * 2,
			MaxIdleConnsPerHost: *apiParallel,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	outputFileExpanded := os.ExpandEnv(*outputFile)

	// Get current working directory for relative paths
	currentDir, err := os.Getwd()
	if err != nil {
		// If we can't get current directory, fall back to home directory
		currentDir = homeDir
	}

	// Determine if output is a directory (has trailing slash) or a file
	// Also check if it's the default .gitxpose directory
	defaultOutputDir := filepath.Join(homeDir, ".gitxpose") + "/"
	hasTrailingSlash := strings.HasSuffix(outputFileExpanded, "/")

	// Build the full path to check if it exists
	var fullPathToCheck string
	if filepath.IsAbs(outputFileExpanded) {
		fullPathToCheck = outputFileExpanded
	} else {
		fullPathToCheck = filepath.Join(currentDir, outputFileExpanded)
	}

	// Check if the path exists and is a directory
	var isDirectory bool
	if hasTrailingSlash || outputFileExpanded == defaultOutputDir {
		isDirectory = true
	} else {
		// Check if path exists and is a directory
		if info, err := os.Stat(fullPathToCheck); err == nil {
			isDirectory = info.IsDir()
		} else {
			// Path doesn't exist - create it as a directory since user wants all output there
			// This allows users to specify a directory name without trailing slash
			isDirectory = true
		}
	}

	var outputDir string
	if isDirectory {
		// It's a directory
		if filepath.IsAbs(outputFileExpanded) {
			// Absolute path - use as is (remove trailing slash if present)
			outputDir = strings.TrimSuffix(outputFileExpanded, "/")
		} else {
			// Relative path - use current working directory (remove trailing slash if present)
			outputDir = filepath.Join(currentDir, strings.TrimSuffix(outputFileExpanded, "/"))
		}
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Printf("%sError creating directory: %v%s\n", ColorRed, err, ColorReset)
			os.Exit(1)
		}
	} else {
		// It's a file, determine the output directory for repos
		// Use a directory with the same base name as the file (without extension)
		if filepath.IsAbs(outputFileExpanded) {
			// Absolute path - use parent directory with base name
			baseName := strings.TrimSuffix(filepath.Base(outputFileExpanded), filepath.Ext(outputFileExpanded))
			outputDir = filepath.Join(filepath.Dir(outputFileExpanded), baseName)
		} else {
			// Relative path - use current directory with base name
			baseName := strings.TrimSuffix(filepath.Base(outputFileExpanded), filepath.Ext(outputFileExpanded))
			outputDir = filepath.Join(currentDir, baseName)
		}
		// Ensure the directory exists
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Printf("%sError creating directory: %v%s\n", ColorRed, err, ColorReset)
			os.Exit(1)
		}
	}

	scanner := bufio.NewScanner(os.Stdin)

	var allReposOutput []byte
	var allCloneURLs []string
	var currentUsername string

	for scanner.Scan() {
		username := scanner.Text()
		currentUsername = username

		output, err := fetchRepos(*scanRepoType, username, tokens, delay, *createdFilter, *updatedFilter, *pushedFilter, *noForkFlag, *apiParallel, httpClient)
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

			outputPath := filepath.Join(userDir, fmt.Sprintf("%s_repo.json", username))
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
			// Relative path - use current working directory
			outputPath = filepath.Join(currentDir, outputFileExpanded)
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
		// Use outputDir for cloning repositories (already set correctly for both file and directory cases)
		cloneRepositories(allCloneURLs, *downloadParallel, *dateFilter, currentUsername, *vulnNotifyID, *analysisParallel, *commitParallel, outputDir)

		printSeparator()
		fmt.Printf("\n%s🎉 All operations completed successfully!%s\n\n", ColorGreen+ColorBold, ColorReset)
	}
}
