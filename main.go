package main

import (
	"github.com/rix4uni/gitrepoenum/cmd"
	"github.com/rix4uni/gitrepoenum/setup"  // Import the init package
)

func main() {
	setup.InitConfig()  // Call the config setup function before running the commands
	cmd.Execute()
}
