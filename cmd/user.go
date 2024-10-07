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
type userConfig struct {
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

func userloadConfig(filePath string) (*userConfig, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config userConfig
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// Read tokens from the token file
func userloadTokens(filePath string) ([]string, error) {
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

type userCleanRepo struct {
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
func usergetRandomToken(tokens []string) string {
	rand.Seed(time.Now().UnixNano())
	return tokens[rand.Intn(len(tokens))]
}

func userfetchRepos(username string, config *userConfig, tokens []string, cleanOutput bool, delay time.Duration) ([]byte, error) {
	var allRepos []map[string]interface{}
	page := config.PAGE

	for {
		// Randomly pick a token for each request
		token := usergetRandomToken(tokens)

		// Build the API URL using the loaded config values
		apiURL := fmt.Sprintf("https://api.github.com/users/%s/repos?type=%s&sort=%s&direction=%s&per_page=%d&page=%d", username, config.TYPE, config.SORT, config.DIRECTION, config.PER_PAGE, page)

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
		return userprintCleanOutput(username, allRepos, config)
	}

	// Pretty-print the JSON response
	output, err := json.MarshalIndent(allRepos, "", "  ")
	if err != nil {
		return nil, err
	}

	return output, nil
}

func userprintCleanOutput(username string, repos []map[string]interface{}, config *userConfig) ([]byte, error) {
	// Prepare the clean output structure
	output := map[string]interface{}{
		"user":  fmt.Sprintf("https://github.com/%s", username),
		"repos": make([]userCleanRepo, len(repos)),
	}

	for i, repo := range repos {
		// Create a clean repo structure based on config settings
		cleanRepo := userCleanRepo{}

		if config.Private == "YES" {
			cleanRepo.Private = usergetBool(repo, "private")
		}
		if config.HTMLURL == "YES" {
			cleanRepo.HTMLURL = usergetString(repo, "html_url")
		}
		if config.Description == "YES" {
			cleanRepo.Description = usergetString(repo, "description")
		}
		if config.Fork == "YES" {
			cleanRepo.Fork = usergetBool(repo, "fork")
		}
		if config.CreatedAt == "YES" {
			cleanRepo.CreatedAt = usergetString(repo, "created_at")
		}
		if config.UpdatedAt == "YES" {
			cleanRepo.UpdatedAt = usergetString(repo, "updated_at")
		}
		if config.PushedAt == "YES" {
			cleanRepo.PushedAt = usergetString(repo, "pushed_at")
		}
		if config.GitURL == "YES" {
			cleanRepo.GitURL = usergetString(repo, "git_url")
		}
		if config.SSHURL == "YES" {
			cleanRepo.SSHURL = usergetString(repo, "ssh_url")
		}
		if config.CloneURL == "YES" {
			cleanRepo.CloneURL = usergetString(repo, "clone_url")
		}
		if config.SVNURL == "YES" {
			cleanRepo.SVNURL = usergetString(repo, "svn_url")
		}
		if config.Size == "YES" {
			cleanRepo.Size = usergetInt(repo, "size") // size is float64 in the decoded JSON
		}
		if config.Language == "YES" {
			cleanRepo.Language = usergetString(repo, "language")
		}

		output["repos"].([]userCleanRepo)[i] = cleanRepo
	}

	// Type assertion for repos
	reposList := output["repos"].([]userCleanRepo)

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
func usergetString(repo map[string]interface{}, key string) string {
	if value, ok := repo[key]; ok && value != nil {
		return value.(string)
	}
	return "" // Return empty string if key does not exist or is nil
}

// Helper function to safely get a bool value from the map
func usergetBool(repo map[string]interface{}, key string) bool {
	if value, ok := repo[key]; ok && value != nil {
		return value.(bool)
	}
	return false // Return false if key does not exist or is nil
}

// Helper function to safely get an int value from the map
func usergetInt(repo map[string]interface{}, key string) int {
	if value, ok := repo[key]; ok && value != nil {
		return int(value.(float64)) // size is typically a float64 in JSON
	}
	return 0 // Return 0 if key does not exist or is nil
}

// userCmd represents the user command
var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Fetch GitHub repositories of a single USER or multiple USERS using a list of usernames",
	Long: `Fetch GitHub repositories of a single USER or multiple USERS using a list of usernames

Examples:
$ echo "rix4uni" | gitrepoenum user -c -o output.json
$ cat usernames.txt | gitrepoenum user -c
$ cat usernames.txt | gitrepoenum user --info
$ cat usernames.txt | gitrepoenum user --delay 1ns
$ cat usernames.txt | gitrepoenum user --config custompath/config.yaml -t custompath/github-token.txt`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		configPathExpanded := os.ExpandEnv(userconfigPath) // Expand the environment variable here
		config, err := userloadConfig(configPathExpanded)  // Use the expanded path
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}

		// Load tokens from the file
		tokenFileExpanded := os.ExpandEnv(usertokenFile) // Expand the environment variable here
		tokens, err := userloadTokens(tokenFileExpanded) // Use the expanded path
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
		delay, err := time.ParseDuration(userdelayFlag)
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
			output, err := userfetchRepos(username, config, tokens, usercleanOutput, delay)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching repos for %s: %v\n", username, err)
				continue
			}

			// Print output to terminal unless --info is used
			if !userinfoFlag {
				fmt.Println(string(output)) // Print output in terminal
			}

			// If no output file specified, save each username in its own file
			if useroutputFile == "" {
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
				outputPath := filepath.Join(outputDir, fmt.Sprintf("%s-user.json", username))
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
		if useroutputFile != "" {
			if err := os.WriteFile(useroutputFile, allReposOutput, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", useroutputFile, err)
			} else {
				fmt.Printf("Output saved to %s\n", useroutputFile)
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
		}
	},
}

var (
	userconfigPath  string
	usertokenFile   string
	userdelayFlag   string
	usercleanOutput bool
	useroutputFile  string
	userinfoFlag    bool
)

func init() {
	rootCmd.AddCommand(userCmd)

	userCmd.Flags().StringVar(&userconfigPath, "config", "$HOME/.config/gitrepoenum/config.yaml", "path to the config.yaml file")
	userCmd.Flags().StringVarP(&usertokenFile, "token", "t", "$HOME/.config/gitrepoenum/github-token.txt", "Path to the file containing GitHub tokens, 1 token per line")
	userCmd.Flags().StringVarP(&userdelayFlag, "delay", "d", "-1ns", "Delay duration between requests (e.g., 1ns, 1us, 1ms, 1s, 1m)")
	userCmd.Flags().BoolVarP(&usercleanOutput, "custom-field", "c", false, "Custom Fields JSON output")
	userCmd.Flags().StringVarP(&useroutputFile, "output", "o", "", "File to save the output.")
	userCmd.Flags().BoolVar(&userinfoFlag, "info", false, "Disable terminal output and only save to file")
}
