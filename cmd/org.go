package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Define the configuration structure
type orgConfig struct {
	// LIST REPOSITORIES FOR A USER
	TYPE      string `yaml:"TYPE"`
	SORT      string `yaml:"SORT"`
	DIRECTION string `yaml:"DIRECTION"`
	PER_PAGE  int    `yaml:"PER_PAGE"` // Change to int for correct usage
	PAGE      int    `yaml:"PAGE"`     // Change to int for correct usage

	// CleanOutput
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

func orgloadConfig(filePath string) (*orgConfig, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config orgConfig
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// Read tokens from the token file
func orgloadTokens(filePath string) ([]string, error) {
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

type orgCleanRepo struct {
	Private     bool   `json:"private,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
	Description string `json:"description,omitempty"`
	Fork        bool   `json:"fork,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	PushedAt    string `json:"pushed_at,omitempty"`
	GitURL      string `json:"git_url,omitempty"`
	SSHURL      string `json:"ssh_url,omitempty"`
	CloneURL    string `json:"clone_url,omitempty"`
	SVNURL      string `json:"svn_url,omitempty"`
	Size        int    `json:"size,omitempty"`
	Language    string `json:"language,omitempty"`
}

// Helper function to randomly pick a token
func orggetRandomToken(tokens []string) string {
	rand.Seed(time.Now().UnixNano())
	return tokens[rand.Intn(len(tokens))]
}

func orgfetchRepos(username string, config *orgConfig, tokens []string, cleanOutput bool, delay time.Duration) ([]byte, error) {
	var allRepos []map[string]interface{}
	page := config.PAGE

	for {
		// Randomly pick a token for each request
		token := orggetRandomToken(tokens)

		// Build the API URL using the loaded config values
		apiURL := fmt.Sprintf("https://api.github.com/orgs/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d", username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)

		// Create the request for the current page
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		// Set the authorization header with the random token
		req.Header.Set("Authorization", "token "+token)

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// Check for successful response
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to fetch repos: %s", resp.Status)
		}

		// Decode the response body into a slice
		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, err
		}

		// Break the loop if there are no more repositories
		if len(repos) == 0 {
			break
		}

		// Append the current page's repositories to the allRepos slice
		allRepos = append(allRepos, repos...)

		// Increment the page number for the next iteration
		page++

		// Apply the delay between API requests
		time.Sleep(delay)
	}

	// Output based on the -clean-output flag
	if cleanOutput {
		return orgprintCleanOutput(username, allRepos, config)
	}

	// Pretty-print the JSON response
	output, err := json.MarshalIndent(allRepos, "", "  ")
	if err != nil {
		return nil, err
	}

	return output, nil
}

func orgprintCleanOutput(username string, repos []map[string]interface{}, config *orgConfig) ([]byte, error) {
	// Prepare the clean output structure
	output := map[string]interface{}{
		"user":  fmt.Sprintf("https://github.com/%s", username),
		"repos": make([]orgCleanRepo, len(repos)),
	}

	for i, repo := range repos {
		// Create a clean repo structure based on config settings
		cleanRepo := orgCleanRepo{}

		if config.Private == "YES" {
			cleanRepo.Private = orggetBool(repo, "private")
		}
		if config.HTMLURL == "YES" {
			cleanRepo.HTMLURL = orggetString(repo, "html_url")
		}
		if config.Description == "YES" {
			cleanRepo.Description = orggetString(repo, "description")
		}
		if config.Fork == "YES" {
			cleanRepo.Fork = orggetBool(repo, "fork")
		}
		if config.CreatedAt == "YES" {
			cleanRepo.CreatedAt = orggetString(repo, "created_at")
		}
		if config.UpdatedAt == "YES" {
			cleanRepo.UpdatedAt = orggetString(repo, "updated_at")
		}
		if config.PushedAt == "YES" {
			cleanRepo.PushedAt = orggetString(repo, "pushed_at")
		}
		if config.GitURL == "YES" {
			cleanRepo.GitURL = orggetString(repo, "git_url")
		}
		if config.SSHURL == "YES" {
			cleanRepo.SSHURL = orggetString(repo, "ssh_url")
		}
		if config.CloneURL == "YES" {
			cleanRepo.CloneURL = orggetString(repo, "clone_url")
		}
		if config.SVNURL == "YES" {
			cleanRepo.SVNURL = orggetString(repo, "svn_url")
		}
		if config.Size == "YES" {
			cleanRepo.Size = orggetInt(repo, "size") // size is float64 in the decoded JSON
		}
		if config.Language == "YES" {
			cleanRepo.Language = orggetString(repo, "language")
		}

		output["repos"].([]orgCleanRepo)[i] = cleanRepo
	}

	// Type assertion for repos
	reposList := output["repos"].([]orgCleanRepo)

	// Build JSON manually
	jsonOutput, err := json.MarshalIndent(map[string]interface{}{
		"user":  output["user"],
		"repos": reposList,
	}, "", "  ")
	if err != nil {
		return nil, err
	}

	return jsonOutput, nil
}

// Helper function to safely get a string value from the map
func orggetString(repo map[string]interface{}, key string) string {
	if value, ok := repo[key]; ok && value != nil {
		return value.(string)
	}
	return "" // Return empty string if key does not exist or is nil
}

// Helper function to safely get a bool value from the map
func orggetBool(repo map[string]interface{}, key string) bool {
	if value, ok := repo[key]; ok && value != nil {
		return value.(bool)
	}
	return false // Return false if key does not exist or is nil
}

// Helper function to safely get an int value from the map
func orggetInt(repo map[string]interface{}, key string) int {
	if value, ok := repo[key]; ok && value != nil {
		return int(value.(float64)) // size is typically a float64 in JSON
	}
	return 0 // Return 0 if key does not exist or is nil
}

// orgCmd represents the org command
var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "Fetch GitHub repositories of a single ORG or multiple ORGS using a list of orgnames",
	Long: `Fetch GitHub repositories of a single ORG or multiple ORGS using a list of orgnames

Examples:
$ echo "IBM" | gitrepoenum org -c -o output.json
$ cat orgnames.txt | gitrepoenum org -c
$ cat orgnames.txt | gitrepoenum org --info
$ cat orgnames.txt | gitrepoenum org --delay 1ns
$ cat orgnames.txt | gitrepoenum org --config custompath/config.yaml -t custompath/github-token.txt`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		configPathExpanded := os.ExpandEnv(orgconfigPath) // Expand the environment variable here
		config, err := orgloadConfig(configPathExpanded)  // Use the expanded path
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}

		// Load tokens from the file
		tokenFileExpanded := os.ExpandEnv(orgtokenFile) // Expand the environment variable here
		tokens, err := orgloadTokens(tokenFileExpanded) // Use the expanded path
		if err != nil {
			fmt.Printf("Error loading tokens: %v\n", err)
			return
		}

		// Ensure tokens are loaded
		if len(tokens) == 0 {
			fmt.Println("No tokens found in the file")
			return
		}

		// Parse the delay value
		delay, err := time.ParseDuration(orgdelayFlag)
		if err != nil {
			fmt.Printf("Invalid delay duration: %v\n", err)
			return
		}

		// Read input for usernames
		scanner := bufio.NewScanner(os.Stdin)
		var allReposOutput []byte

		for scanner.Scan() {
			username := scanner.Text()

			// Fetch repositories for each username
			output, err := orgfetchRepos(username, config, tokens, orgcleanOutput, delay)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching repos for %s: %v\n", username, err)
				continue
			}

			// Print output to terminal unless --info is used
			if !orginfoFlag {
				fmt.Println(string(output)) // Print output in terminal
			}

			// If no output file specified, save each username in its own file
			if orgoutputFile == "" {
				// Set the default output path
				homeDir, err := os.UserHomeDir()
				if err != nil {
					fmt.Printf("Error fetching home directory: %v\n", err)
					return
				}

				// Ensure the directory ~/allgithubrepo exists
				outputDir := filepath.Join(homeDir, "allgithubrepo")
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					fmt.Printf("Error creating directory: %v\n", err)
					return
				}

				// Set the default output file path for each username
				outputPath := filepath.Join(outputDir, fmt.Sprintf("%s-org.json", username))
				if err := os.WriteFile(outputPath, output, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", outputPath, err)
				} else {
					fmt.Printf("Output saved to %s\n", outputPath)
				}
			} else {
				// Append output to the allReposOutput if -o flag is used
				allReposOutput = append(allReposOutput, output...)
			}
		}

		// Save all repos in one file if -o flag is specified
		if orgoutputFile != "" {
			if err := os.WriteFile(orgoutputFile, allReposOutput, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", orgoutputFile, err)
			} else {
				fmt.Printf("Output saved to %s\n", orgoutputFile)
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
		}
	},
}

var (
	orgconfigPath  string
	orgtokenFile   string
	orgdelayFlag   string
	orgcleanOutput bool
	orgoutputFile  string
	orginfoFlag    bool
)

func init() {
	rootCmd.AddCommand(orgCmd)

	orgCmd.Flags().StringVar(&orgconfigPath, "config", "$HOME/.config/gitrepoenum/config.yaml", "path to the config.yaml file")
	orgCmd.Flags().StringVarP(&orgtokenFile, "token", "t", "$HOME/.config/gitrepoenum/github-token.txt", "Path to the file containing GitHub tokens, 1 token per line")
	orgCmd.Flags().StringVarP(&orgdelayFlag, "delay", "d", "-1ns", "Delay duration between requests (e.g., 1ns, 1us, 1ms, 1s, 1m)")
	orgCmd.Flags().BoolVarP(&orgcleanOutput, "custom-field", "c", false, "Custom Fields JSON output")
	orgCmd.Flags().StringVarP(&orgoutputFile, "output", "o", "", "File to save the output.")
	orgCmd.Flags().BoolVar(&orginfoFlag, "info", false, "Disable terminal output and only save to file")
}
