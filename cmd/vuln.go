package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var inputDir string
var outputDir string

// vulnCmd represents the vuln command
var vulnCmd = &cobra.Command{
	Use:   "vuln",
	Short: "Scan repositories for vulnerabilities using TruffleHog",
	Long: `This command scans multiple repositories for vulnerabilities using TruffleHog 
and saves the results in the specified output directory.

Examples:
$ gitrepoenum vuln
$ gitrepoenum vuln -i ~/allgithubrepo/commits -o ~/allgithubrepo/commits`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate input and output directories
		if inputDir == "" || outputDir == "" {
			fmt.Println("Input and output directories must be specified.")
			return
		}

		// Scan the input directory for repositories
		err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check if the path is a directory and if it contains a 'code' folder
			if info.IsDir() && filepath.Base(path) == "code" {
				repoPath := filepath.Dir(path)                                              // Get the repository path
				vulnOutputPath := filepath.Join(outputDir, filepath.Base(repoPath), "vuln") // Output directory for vuln

				// Create the vuln output directory if it doesn't exist
				err = os.MkdirAll(vulnOutputPath, os.ModePerm)
				if err != nil {
					return err
				}

				// Run TruffleHog on the current repository
				trufflehogCmd := exec.Command("trufflehog", "filesystem", repoPath)
				trufflehogOutputFile := filepath.Join(vulnOutputPath, "trufflehog.txt")

				// Redirect output to the output file
				outputFile, err := os.Create(trufflehogOutputFile)
				if err != nil {
					return err
				}
				defer outputFile.Close()

				trufflehogCmd.Stdout = outputFile
				trufflehogCmd.Stderr = outputFile // Capture stderr as well

				if err := trufflehogCmd.Run(); err != nil {
					return fmt.Errorf("failed to run trufflehog on %s: %v", repoPath, err)
				}

				fmt.Printf("[Scanned] %s and saved results to %s\n", repoPath, trufflehogOutputFile)
			}
			return nil
		})

		if err != nil {
			fmt.Printf("Error scanning directories: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(vulnCmd)

	// Define flags for input and output directories
	vulnCmd.Flags().StringVarP(&inputDir, "input", "i", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "commits"), "Input directory containing repositories code")
	vulnCmd.Flags().StringVarP(&outputDir, "output", "o", filepath.Join(os.Getenv("HOME"), "allgithubrepo", "commits"), "Output directory for vulnerability reports")
}
