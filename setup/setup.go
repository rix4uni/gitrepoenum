package setup

import (
    "fmt"
    "io"
    "os"
    "os/user"
    "path/filepath"
)

// Function to copy files from the current directory to the config directory
func copyFile(src, dest string) error {
    sourceFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer sourceFile.Close()

    destinationFile, err := os.Create(dest)
    if err != nil {
        return err
    }
    defer destinationFile.Close()

    _, err = io.Copy(destinationFile, sourceFile)
    if err != nil {
        return err
    }

    return nil
}

// Function to initialize the config setup
func InitConfig() {
    usr, err := user.Current()
    if err != nil {
        fmt.Println("Error retrieving user information:", err)
        return
    }

    configDir := filepath.Join(usr.HomeDir, ".config", "gitrepoenum")

    // Check if the directory exists
    if _, err := os.Stat(configDir); os.IsNotExist(err) {
        // fmt.Println("Creating config directory:", configDir)
        err = os.MkdirAll(configDir, 0755)
        if err != nil {
            fmt.Println("Error creating config directory:", err)
            return
        }
    }

    // Paths to your default config and token files in your project
    defaultConfig := "config/config.yaml"
    defaultToken := "config/github-token.txt"

    // Destination paths in the ~/.config/gitrepoenum
    destConfig := filepath.Join(configDir, "config.yaml")
    destToken := filepath.Join(configDir, "github-token.txt")

    // Check if the config files already exist
    if _, err := os.Stat(destConfig); err == nil {
        // fmt.Println("Configuration already set up. Skipping config.yaml setup.")
    } else {
        // Copy the config file to ~/.config/gitrepoenum
        err = copyFile(defaultConfig, destConfig)
        if err != nil {
            fmt.Println("Error copying config.yaml:", err)
            return
        }
    }

    if _, err := os.Stat(destToken); err == nil {
        // fmt.Println("Configuration already set up. Skipping github-token.txt setup.")
    } else {
        // Copy the token file to ~/.config/gitrepoenum
        err = copyFile(defaultToken, destToken)
        if err != nil {
            fmt.Println("Error copying github-token.txt:", err)
            return
        }
    }

    // fmt.Println("Configuration setup completed.")
}
