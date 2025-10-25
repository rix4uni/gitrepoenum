package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// Define variables for directory, parallel, and depth flags
var directory string
var downloadparallel int
var downloaddepth int

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Clone Git repositories with a custom directory name and parallel option",
	Long: `Clone Git repositories and customize the directory name to username-repositoryname with an option to clone in parallel.

Examples:
$ echo "https://github.com/rix4uni/gitrepoenum.git" | gitrepoenum download
$ cat reponames.txt | gitrepoenum download
$ cat ~/allgithubrepo/rix4uni-user.json | jq -r '.repos[].clone_url' | gitrepoenum download
$ cat reponames.txt | gitrepoenum download -o ~/allgithubrepo/download
$ cat reponames.txt | gitrepoenum download -p 100
$ cat reponames.txt | gitrepoenum download -d 1`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create a scanner to read from stdin
		scanner := bufio.NewScanner(os.Stdin)

		// Collect all repository URLs from stdin
		var repoURLs []string
		for scanner.Scan() {
			repoURLs = append(repoURLs, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			fmt.Println("Error reading input:", err)
			return
		}

		// Create a semaphore channel to limit concurrent clones
		sem := make(chan struct{}, downloadparallel)
		var wg sync.WaitGroup

		for _, url := range repoURLs {
			wg.Add(1)

			go func(url string) {
				defer wg.Done()

				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }() // Release semaphore when done

				// Extract the repository details from the URL
				parts := strings.Split(url, "/")
				if len(parts) < 2 {
					fmt.Println("Invalid repository URL:", url)
					return
				}

				username := parts[len(parts)-2]
				reponame := strings.TrimSuffix(parts[len(parts)-1], ".git")
				dirName := username + "-" + reponame

				// If the directory flag is set, clone into that directory
				if directory != "" {
					dirName = filepath.Join(directory, dirName)
				}

				// Check if the directory already exists
				if _, err := os.Stat(dirName); !os.IsNotExist(err) {
					// If it exists and is not empty, remove it
					fmt.Printf("Directory %s already exists, removing it...\n", dirName)
					err := os.RemoveAll(dirName)
					if err != nil {
						fmt.Printf("Failed to remove existing directory: %s\n", err)
						return
					}
				}

				// Execute the git clone command with the custom directory name and depth option
				cloneArgs := []string{"clone", url, dirName}
				if downloaddepth > 0 {
					cloneArgs = append(cloneArgs, "--depth", fmt.Sprintf("%d", downloaddepth))
				}
				cloneCmd := exec.Command("git", cloneArgs...)

				// Suppress non-error output, but capture stderr for error handling
				var stderr bytes.Buffer
				cloneCmd.Stderr = &stderr

				if err := cloneCmd.Run(); err != nil {
					// Print the error message from Git if cloning fails
					fmt.Printf("Error cloning repository %s: %s\n", url, stderr.String())
				} else {
					// Only show this message upon successful clone
					fmt.Printf("[CLONED] %s into %s\n", url, dirName)
				}
			}(url)
		}

		// Wait for all cloning goroutines to complete
		wg.Wait()
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().StringVarP(&directory, "output-directory", "o", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "download"), "Directory to clone the repositories into")
	downloadCmd.Flags().IntVarP(&downloadparallel, "parallel", "p", 10, "Number of repositories to clone in parallel")
	downloadCmd.Flags().IntVarP(&downloaddepth, "depth", "d", 0, "Create a shallow clone with a history truncated to the specified number of commits, use -d 1 if you want only latest commits")
}
