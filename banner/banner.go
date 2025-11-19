package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.5"

func PrintVersion() {
	fmt.Printf("Current gitxpose version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
           _  __                                
   ____ _ (_)/ /_ _  __ ____   ____   _____ ___ 
  / __  // // __/| |/_// __ \ / __ \ / ___// _ \
 / /_/ // // /_ _>  < / /_/ // /_/ /(__  )/  __/
 \__, //_/ \__//_/|_|/ .___/ \____//____/ \___/ 
/____/              /_/
`
	fmt.Printf("%s\n%55s\n\n", banner, "Current gitxpose version "+version)
}
