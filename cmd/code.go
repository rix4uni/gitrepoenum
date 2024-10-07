package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func fetchCommitContent(repoPath string, commitHash string, outputFilePath string) error {
	// Run git show command to get the commit details
	cmd := exec.Command("git", "-C", repoPath, "--no-pager", "show", commitHash)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error fetching commit %s: %v", commitHash, err)
	}

	// Write the commit details to the output file
	err = ioutil.WriteFile(outputFilePath, output, 0644)
	if err != nil {
		return fmt.Errorf("error writing commit to file: %v", err)
	}

	return nil
}

func processRepo(repoPath string, commitsFile string, outputDir string) error {
	// Read the commits.txt file to get all commit hashes
	content, err := ioutil.ReadFile(commitsFile)
	if err != nil {
		return fmt.Errorf("error reading commits file: %v", err)
	}

	commits := strings.Split(string(content), "\n")
	codeDir := filepath.Join(outputDir, "code")
	if err := os.MkdirAll(codeDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating code directory: %v", err)
	}

	// Iterate over each commit and fetch its content
	for _, commit := range commits {
		if strings.TrimSpace(commit) == "" {
			continue // Skip empty lines
		}

		outputFilePath := filepath.Join(codeDir, commit+".txt")
		// fmt.Printf("[FETCHING CODE] %s into %s\n", repoPath, outputFilePath)
		if err := fetchCommitContent(repoPath, commit, outputFilePath); err != nil {
			return err
		}
	}

	return nil
}

func processAllRepos(inputDir string, outputDir string) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			repoPath := filepath.Join(inputDir, entry.Name())
			// Check if the directory is a Git repository
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				// Path to the commits.txt file for this repo
				commitsFile := filepath.Join(outputDir, entry.Name(), "commits.txt")
				outputRepoDir := filepath.Join(outputDir, entry.Name())

				fmt.Printf("[FETCHING COMMITS] %s into %s\n", repoPath, outputRepoDir+"/code")
				if err := processRepo(repoPath, commitsFile, outputRepoDir); err != nil {
					fmt.Printf("Error processing repo %s: %v\n", entry.Name(), err)
				}
			}
		}
	}

	return nil
}

var codeCmd = &cobra.Command{
	Use:   "code",
	Short: "Fetch code from multiple commits",
	Long: `This command fetches code from multiple commits based on a list in commits.txt for each repository.

Examples:
$ gitrepoenum code
$ gitrepoenum code -i ~/allgithubrepo/download -o ~/allgithubrepo/commits`,
	Run: func(cmd *cobra.Command, args []string) {
		inputDir, _ := cmd.Flags().GetString("input")
		outputDir, _ := cmd.Flags().GetString("output")

		if err := processAllRepos(inputDir, outputDir); err != nil {
			fmt.Println("Error:", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(codeCmd)
	codeCmd.Flags().StringP("input", "i", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "download"), "Specify the input directory containing Git repositories")
	codeCmd.Flags().StringP("output", "o", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "commits"), "Specify the output directory for storing commit code")
}
