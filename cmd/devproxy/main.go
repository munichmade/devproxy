package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	fmt.Printf("devproxy %s\n", version)
	os.Exit(0)
}
