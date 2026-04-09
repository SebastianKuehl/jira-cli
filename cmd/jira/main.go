package main

import (
	"log"
	"os"

	"github.com/sebastian/jira-cli/internal/app"
)

func main() {
	if err := app.New().Run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
