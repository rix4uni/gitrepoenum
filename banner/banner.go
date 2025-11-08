package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.3"

func PrintVersion() {
	fmt.Printf("Current gitrepoenum version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
           _  __                                                         
   ____ _ (_)/ /_ _____ ___   ____   ____   ___   ____   __  __ ____ ___ 
  / __  // // __// ___// _ \ / __ \ / __ \ / _ \ / __ \ / / / // __  __ \
 / /_/ // // /_ / /   /  __// /_/ // /_/ //  __// / / // /_/ // / / / / /
 \__, //_/ \__//_/    \___// .___/ \____/ \___//_/ /_/ \__,_//_/ /_/ /_/ 
/____/                    /_/                                            `
	fmt.Printf("%s\n%75s\n\n", banner, "Current gitrepoenum version "+version)
}
