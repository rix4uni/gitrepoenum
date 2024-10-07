package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
)

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

func runGitLog(repoPath string, gitTime string, timeDirection string, outputFile string) error {
	// Prepare the command based on whether gitTime is empty
	args := []string{"-C", repoPath, "--no-pager", "log", "--pretty=format:%H"}

	if gitTime != "" {
		args = append(args, timeDirection+gitTime)
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error executing git log: %v", err)
	}

	return ioutil.WriteFile(outputFile, output, 0644) // Save output to file
}

func scanRepos(inputDir string, dateFlag string, timeDirection string, outputDir string) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			repoPath := filepath.Join(inputDir, entry.Name())
			// Check if the directory is a Git repository
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				// Prepare output path
				outputRepoDir := filepath.Join(outputDir, entry.Name())
				if err := os.MkdirAll(outputRepoDir, os.ModePerm); err != nil {
					return fmt.Errorf("error creating output directory: %v", err)
				}
				outputFile := filepath.Join(outputRepoDir, "commits.txt")

				// Fetch commits
				fmt.Printf("[FETCHING COMMITS] %s into %s\n", repoPath, outputFile)
				if err := runGitLog(repoPath, dateFlag, timeDirection, outputFile); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Show commit logs",
	Long: `This command retrieves git commit logs based on date and time parameters.

Examples:
$ gitrepoenum commit
$ gitrepoenum commit -i ~/allgithubrepo/download -d 50s -t before -o ~/allgithubrepo/commits
$ gitrepoenum commit -i ~/allgithubrepo/download -d 5h -t before -o ~/allgithubrepo/commits
$ gitrepoenum commit -i ~/allgithubrepo/download -d 1d -t after -o ~/allgithubrepo/commits
$ gitrepoenum commit -i ~/allgithubrepo/download -d all -o ~/allgithubrepo/commits

Date Options:
50s	# 50 seconds
40m     # 40 minutes
5h      # 5 hours
1d      # 1 day
2w      # 2 weeks
3M      # 3 months
1y      # 1 year
all     # All commits`,
	Run: func(cmd *cobra.Command, args []string) {
		inputDir, _ := cmd.Flags().GetString("input")
		outputDir, _ := cmd.Flags().GetString("output")
		dateFlag, _ := cmd.Flags().GetString("date")
		timeDirection, _ := cmd.Flags().GetString("time")

		// Handle 'all' case
		if dateFlag == "all" {
			// Scan all repositories
			if err := scanRepos(inputDir, "", "", outputDir); err != nil {
				fmt.Println("Error:", err)
			}
			return
		}

		// Extract the number and unit from the date argument
		re := regexp.MustCompile(`([0-9]+)([smhdwMy])`) // Updated regex to include short forms
		matches := re.FindStringSubmatch(dateFlag)

		if len(matches) != 3 {
			fmt.Println("Invalid date format. Use <number><unit>.")
			return
		}

		dateNum := matches[1]
		dateUnit := matches[2]

		// Convert to git time format
		gitTime, err := convertToGitTime(dateNum, dateUnit)
		if err != nil {
			fmt.Println(err)
			return
		}

		// Determine time direction
		var direction string
		if timeDirection == "before" {
			direction = "--before="
		} else if timeDirection == "after" {
			direction = "--after="
		} else {
			fmt.Println("Invalid time direction. Use 'after' or 'before'.")
			return
		}

		// Scan all repositories and fetch commits
		if err := scanRepos(inputDir, gitTime, direction, outputDir); err != nil {
			fmt.Println("Error:", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(commitCmd)

	commitCmd.Flags().StringP("input", "i", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "download"), "Specify the input directory containing Git repositories")
	commitCmd.Flags().StringP("date", "d", "all", "Specify the date range for the commits (e.g., 50s, 40m, 5h, 1d, 2w, 3M, 1y, all)")
	commitCmd.Flags().StringP("time", "t", "", "Specify 'before' or 'after' the given date")
	commitCmd.Flags().StringP("output", "o", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "commits"), "Specify the output directory for commit logs")
}
